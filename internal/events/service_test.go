package events

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/repositories"
)

func TestStoreNormalizesLegacyNullCollections(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":1,"events":[{"id":"evt_123456789abc","name":"Evento legado","key":"evento_legado","sender_ids":null,"action_type":"none","recipients":null,"enabled":false,"created_at":"2026-08-01T00:00:00Z","updated_at":"2026-08-01T00:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, "events.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	items := store.All()
	if len(items) != 1 || items[0].SenderIDs == nil || items[0].Recipients == nil {
		t.Fatalf("legacy collections were not normalized: %+v", items)
	}
	response, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), `"sender_ids":[]`) || !strings.Contains(string(response), `"recipients":[]`) {
		t.Fatalf("normalized event still serializes null collections: %s", response)
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func eventFixture(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC)
	for _, sender := range []domain.Sender{{ID: "worker-a", Name: "Worker A", Status: domain.StatusOnline, CreatedAt: now, UpdatedAt: now}, {ID: "worker-b", Name: "Worker B", Status: domain.StatusInactive, CreatedAt: now, UpdatedAt: now}, {ID: "revoked", Name: "Revoked", Status: domain.StatusRevoked, CreatedAt: now, UpdatedAt: now}} {
		if err := repo.Create(context.Background(), sender); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill"}
	emailSettings, err := emailconfig.Open(dir, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(store, repo, emailSettings, fixedClock{now}), dir, "worker-a", "worker-b"
}

func validInput(key string, senders ...string) domain.EventInput {
	return domain.EventInput{Name: "Processing completed", Key: key, SenderIDs: senders, ActionType: domain.EventActionEmail, Recipients: []string{"dev@example.com"}, SubjectTemplate: "Finalizado — {{sender.name}}", MessageTemplate: "Protocol: {{metadata.protocol}}\n{{log.message}}", Enabled: true}
}

func TestEventWithoutEmailCanBeActive(t *testing.T) {
	service, _, sender, _ := eventFixture(t)
	event, err := service.Create(context.Background(), domain.EventInput{Name: "Somente monitoramento", Key: "somente_monitoramento", SenderIDs: []string{sender}, ActionType: domain.EventActionNone, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if event.ActionType != domain.EventActionNone || len(event.Recipients) != 0 || event.SubjectTemplate != "" || !event.Enabled {
		t.Fatalf("invalid event without email: %#v", event)
	}
}

func TestEventCRUDMatchingAndKeyRules(t *testing.T) {
	service, _, senderA, senderB := eventFixture(t)
	ctx := context.Background()
	event, err := service.Create(ctx, validInput("processing_completed", senderA, senderB))
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == "" || !service.Matching(senderA, event.Key)[0].Enabled {
		t.Fatal("event was not indexed")
	}
	if len(service.Matching("other", event.Key)) != 0 || len(service.Matching(senderA, "other")) != 0 {
		t.Fatal("unexpected match")
	}
	if _, err = service.Create(ctx, validInput(event.Key, senderA)); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
	input := validInput("outra_chave", senderA)
	if _, err = service.Update(ctx, event.ID, input); err == nil {
		t.Fatal("event key should be immutable")
	}
	updated, err := service.SetEnabled(ctx, event.ID, false)
	if err != nil || updated.Enabled || len(service.Matching(senderA, event.Key)) != 0 {
		t.Fatalf("disabled event still matches: %v", err)
	}
	if err = service.Delete(event.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(event.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestEventValidationPersistenceAndSenderRemoval(t *testing.T) {
	service, dir, senderA, _ := eventFixture(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, validInput("Key Inválida", senderA)); err == nil {
		t.Fatal("invalid key accepted")
	}
	if _, err := service.Create(ctx, validInput("revogado_evento", "revoked")); err == nil {
		t.Fatal("revoked sender accepted")
	}
	event, err := service.Create(ctx, validInput("boleto_gerado", senderA))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if restored, exists := store.Get(event.ID); !exists || restored.Key != event.Key {
		t.Fatalf("event was not restored: %+v", restored)
	}
	if affected, err := service.RemoveSender(senderA); err != nil || affected != 1 {
		t.Fatalf("remove sender=%d %v", affected, err)
	}
	updated, err := service.Get(event.ID)
	if err != nil || updated.Enabled || len(updated.SenderIDs) != 0 {
		t.Fatalf("orphan event should be disabled: %+v %v", updated, err)
	}
}

func TestEventIndexConcurrentReadsAndUpdates(t *testing.T) {
	service, _, senderA, _ := eventFixture(t)
	ctx := context.Background()
	event, err := service.Create(ctx, validInput("concorrencia_ok", senderA))
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for read := 0; read < 50; read++ {
				_ = service.Matching(senderA, event.Key)
			}
		}()
	}
	for index := 0; index < 10; index++ {
		if _, err = service.SetEnabled(ctx, event.ID, index%2 == 0); err != nil {
			t.Fatal(err)
		}
	}
	wait.Wait()
}
