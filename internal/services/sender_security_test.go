package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"logtheater/internal/domain"
	"logtheater/internal/repositories"
	settingsstore "logtheater/internal/settings"
)

func newSenderSecurityService(t *testing.T) (*Service, *fakeClock, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 7, 31, 15, 0, 0, 0, time.UTC)}
	settings, err := settingsstore.Open(dataDir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	return New(repo, testConfig(dataDir), clock, settings), clock, dataDir
}

func TestSenderIDNormalizationHasNoGeneratedSuffix(t *testing.T) {
	cases := map[string]string{
		"  Automação   Financeira  ": "automacao-financeira",
		"Consulta PJe - TRF3":        "consulta-pje-trf3",
		"Cobrança / Santander":       "cobranca-santander",
		"---Robô___de@@Consulta---":  "robo-de-consulta",
	}
	for input, expected := range cases {
		actual, err := NormalizeName(input)
		if err != nil || actual != expected {
			t.Fatalf("NormalizeName(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
	svc, _, _ := newSenderSecurityService(t)
	item, _, err := svc.CreateSender(context.Background(), "Financial Automation", "")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "financial-automation" || strings.Contains(item.ID, "-") && strings.HasSuffix(item.ID, "-2") {
		t.Fatalf("unexpected generated ID: %q", item.ID)
	}
}

func TestSenderCreationIsAtomicAndRejectsDuplicateID(t *testing.T) {
	svc, _, _ := newSenderSecurityService(t)
	var created atomic.Int32
	var conflicts atomic.Int32
	var group sync.WaitGroup
	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, err := svc.CreateSender(context.Background(), "Robô Financeiro", "")
			switch {
			case err == nil:
				created.Add(1)
			case errors.Is(err, domain.ErrSenderAlreadyExists):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected create error: %v", err)
			}
		}()
	}
	group.Wait()
	if created.Load() != 1 || conflicts.Load() != 11 {
		t.Fatalf("created=%d conflicts=%d", created.Load(), conflicts.Load())
	}
}

func TestSenderKeyIsOnlyReturnedOnceAndNeverPersistedInPlaintext(t *testing.T) {
	svc, _, dataDir := newSenderSecurityService(t)
	item, credentials, err := svc.CreateSender(context.Background(), "Worker Seguro", "Description")
	if err != nil {
		t.Fatal(err)
	}
	if !credentials.DisplayedOnce || !strings.HasPrefix(credentials.SenderKey, "snd_") || len(strings.TrimPrefix(credentials.SenderKey, "snd_")) < 24 {
		t.Fatalf("unsafe credentials: %+v", credentials)
	}
	persisted, err := os.ReadFile(filepath.Join(dataDir, "senders", item.ID, "sender.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), credentials.SenderKey) || !strings.Contains(string(persisted), `"key_hash"`) {
		t.Fatalf("unexpected sender.json: %s", persisted)
	}
	page, err := svc.Senders(context.Background(), domain.SenderFilters{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := json.Marshal(page)
	if strings.Contains(string(response), credentials.SenderKey) || strings.Contains(string(response), "key_hash") {
		t.Fatalf("sender listing exposed key material: %s", response)
	}
}

func TestSenderAuthenticationRotationRevocationAndReactivation(t *testing.T) {
	svc, _, _ := newSenderSecurityService(t)
	ctx := context.Background()
	first, firstCredentials, err := svc.CreateSender(ctx, "Primeiro Sender", "")
	if err != nil {
		t.Fatal(err)
	}
	second, secondCredentials, err := svc.CreateSender(ctx, "Segundo Sender", "")
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]string{"missing": "", "wrong": "snd_wrong", "other_sender": secondCredentials.SenderKey} {
		if _, _, err = svc.ReceiveLog(ctx, first.ID, key, "INFO", name, nil, nil); !errors.Is(err, domain.ErrInvalidSenderKey) {
			t.Fatalf("%s key should fail generically, got %v", name, err)
		}
	}
	if _, _, err = svc.Health(ctx, first.ID, firstCredentials.SenderKey); err != nil {
		t.Fatal(err)
	}
	connected, _ := svc.Get(ctx, first.ID)
	if connected.Status != domain.StatusOnline || connected.LastActivityAt == nil {
		t.Fatalf("first contact did not connect sender: %+v", connected)
	}
	_, rotatedCredentials, _, err := svc.RotateSenderKey(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Health(ctx, first.ID, firstCredentials.SenderKey); !errors.Is(err, domain.ErrInvalidSenderKey) {
		t.Fatalf("old key remained valid: %v", err)
	}
	if _, _, err = svc.ReceiveLog(ctx, first.ID, rotatedCredentials.SenderKey, "ERROR", "new key", nil, nil); err != nil {
		t.Fatal(err)
	}
	revoked, err := svc.RevokeSender(ctx, first.ID)
	if err != nil || revoked.Status != domain.StatusRevoked {
		t.Fatalf("revoke failed: %+v, %v", revoked, err)
	}
	if _, _, err = svc.Health(ctx, first.ID, rotatedCredentials.SenderKey); !errors.Is(err, domain.ErrInvalidSenderKey) {
		t.Fatalf("revoked key remained valid: %v", err)
	}
	reactivated, reactivatedCredentials, err := svc.ReactivateSender(ctx, first.ID)
	if err != nil || reactivated.Status != domain.StatusNeverConnected || reactivated.LastActivityAt != nil {
		t.Fatalf("reactivation failed: %+v, %v", reactivated, err)
	}
	if reactivatedCredentials.SenderKey == rotatedCredentials.SenderKey {
		t.Fatal("reactivation reused the revoked key")
	}
	if _, _, err = svc.Health(ctx, second.ID, reactivatedCredentials.SenderKey); !errors.Is(err, domain.ErrInvalidSenderKey) {
		t.Fatalf("reactivated key authenticated another sender: %v", err)
	}
}

func TestSenderIDRemainsImmutableAndNeverConnectedDoesNotExpire(t *testing.T) {
	svc, clock, _ := newSenderSecurityService(t)
	ctx := context.Background()
	item, _, err := svc.CreateSender(ctx, "Sender Original", "antes")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.UpdateSender(ctx, item.ID, "Sender Renomeado", "depois")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != item.ID || updated.Name != "Sender Renomeado" || updated.Description != "depois" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	clock.now = clock.now.Add(30 * 24 * time.Hour)
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	updated, _ = svc.Get(ctx, item.ID)
	if updated.Status != domain.StatusNeverConnected {
		t.Fatalf("never-connected sender changed state: %+v", updated)
	}
}
