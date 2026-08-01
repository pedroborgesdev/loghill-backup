package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/repository"
)

func TestAlertSupportsMultipleSendersAndMaintainsIndex(t *testing.T) {
	service, _, repo, clock, first := alertFixture(t)
	now := clock.Now()
	second := domain.Sender{ID: "worker-second", Name: "worker second", Status: domain.StatusNeverConnected, CreatedAt: now, UpdatedAt: now}
	if err := repo.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	input := validInput(first)
	input.SenderIDs = []string{first.ID, second.ID}
	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.SenderIDs) != 2 || len(service.Matching(first.ID, domain.Error)) != 1 || len(service.Matching(second.ID, domain.Fatal)) != 1 {
		t.Fatalf("multi-sender matching failed: %+v", created)
	}
	if service.SenderUsageCount(second.ID) != 1 {
		t.Fatal("sender index did not record the rule")
	}
	if affected, err := service.RemoveSender(first.ID); err != nil || affected != 1 {
		t.Fatalf("remove first sender: affected=%d err=%v", affected, err)
	}
	remaining, _ := service.Get(created.ID)
	if !remaining.Enabled || len(remaining.SenderIDs) != 1 || remaining.SenderIDs[0] != second.ID || len(service.Matching(first.ID, domain.Error)) != 0 {
		t.Fatalf("rule or index not updated after removal: %+v", remaining)
	}
	if affected, err := service.RemoveSender(second.ID); err != nil || affected != 1 {
		t.Fatalf("remove second sender: affected=%d err=%v", affected, err)
	}
	empty, _ := service.Get(created.ID)
	if empty.Enabled || len(empty.SenderIDs) != 0 || len(service.Matching(second.ID, domain.Error)) != 0 {
		t.Fatalf("empty rule was not disabled: %+v", empty)
	}
}

func TestLegacySenderIDIsMigratedOnReadAndNextWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.json")
	legacy := `{"version":1,"items":[{"id":"alert-1234567890abcdef","name":"Legada","sender_id":"legacy-worker-a1b2c3d4","sender_name":"Legacy Worker","severities":["ERROR"],"recipients":["dev@example.com"],"provider":"outlook","enabled":false,"created_at":"2026-07-31T12:00:00Z","updated_at":"2026-07-31T12:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	item, ok := store.Get("alert-1234567890abcdef")
	if !ok || len(item.SenderIDs) != 1 || item.SenderIDs[0] != "legacy-worker-a1b2c3d4" || item.SenderNames[0] != "Legacy Worker" {
		t.Fatalf("legacy rule was not migrated in memory: %+v", item)
	}
	if err = store.Put(item); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(persisted, &value); err != nil {
		t.Fatal(err)
	}
	text := string(persisted)
	if !strings.Contains(text, `"sender_ids"`) || strings.Contains(text, `"sender_id"`) {
		t.Fatalf("legacy fields remained after write: %s", persisted)
	}
}

type fixedClock struct{ value time.Time }

func (c *fixedClock) Now() time.Time { return c.value }

func alertFixture(t *testing.T) (*Service, *Store, *repository.FileRepository, *fixedClock, domain.Sender) {
	t.Helper()
	dir := t.TempDir()
	repo := repository.New(filepath.Join(dir, "data"))
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fixedClock{value: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)}
	cfg := config.Config{EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill"}
	emailSettings, err := emailconfig.Open(filepath.Join(dir, "data"), cfg, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	now := clock.Now()
	sender := domain.Sender{ID: "worker-1234abcd", Name: "worker", Status: domain.StatusOnline, CreatedAt: now, UpdatedAt: now, LastActivityAt: &now}
	if err = repo.Create(context.Background(), sender); err != nil {
		t.Fatal(err)
	}
	return NewService(store, repo, emailSettings, clock), store, repo, clock, sender
}

func validInput(sender domain.Sender) domain.AlertInput {
	return domain.AlertInput{Name: "Erros críticos", SenderIDs: []string{sender.ID}, Severities: []domain.LogSeverity{domain.Error, domain.Fatal}, Recipients: []string{"DEV@example.com", "ops@example.com"}, Provider: domain.EmailProviderOutlook, Enabled: true}
}

func TestAlertCRUDMatchingAndPersistence(t *testing.T) {
	service, store, _, clock, sender := alertFixture(t)
	created, err := service.Create(context.Background(), validInput(sender))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Recipients[0] != "dev@example.com" {
		t.Fatalf("unexpected alert: %+v", created)
	}
	if len(service.Matching(sender.ID, domain.Error)) != 1 || len(service.Matching(sender.ID, domain.Info)) != 0 {
		t.Fatal("matching did not honor sender and severity")
	}
	clock.value = clock.value.Add(time.Minute)
	input := validInput(sender)
	input.Name = "Erros atualizados"
	input.Enabled = false
	updated, err := service.Update(context.Background(), created.ID, input)
	if err != nil || updated.Enabled || !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("unexpected update: %+v %v", updated, err)
	}
	if len(service.Matching(sender.ID, domain.Error)) != 0 {
		t.Fatal("inactive alert matched")
	}
	if err = service.MarkPending(created.ID, false); err != nil {
		t.Fatal(err)
	}
	if err = service.RecordDelivery(created.ID, false, domain.DeliverySent, ""); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Dir(store.path))
	if err != nil {
		t.Fatal(err)
	}
	persisted, ok := reopened.Get(created.ID)
	if !ok || persisted.DeliveryCount != 1 || persisted.LastDeliveryStatus == nil || *persisted.LastDeliveryStatus != domain.DeliverySent {
		t.Fatalf("delivery state was not restored: %+v", persisted)
	}
	if err = service.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestAlertValidation(t *testing.T) {
	service, _, repo, clock, sender := alertFixture(t)
	tests := []struct {
		name     string
		mutate   func(*domain.AlertInput)
		expected error
	}{
		{"missing severity", func(input *domain.AlertInput) { input.Severities = nil }, nil},
		{"invalid severity", func(input *domain.AlertInput) { input.Severities = []domain.LogSeverity{"NOPE"} }, nil},
		{"invalid email", func(input *domain.AlertInput) { input.Recipients = []string{"not-an-email"} }, nil},
		{"too many recipients", func(input *domain.AlertInput) {
			input.Recipients = make([]string, 21)
			for i := range input.Recipients {
				input.Recipients[i] = "x" + string(rune('a'+i)) + "@example.com"
			}
		}, nil},
		{"gmail", func(input *domain.AlertInput) { input.Provider = domain.EmailProviderGmail }, nil},
		{"missing sender", func(input *domain.AlertInput) { input.SenderIDs = []string{"missing-1234abcd"} }, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validInput(sender)
			test.mutate(&input)
			if _, err := service.Create(context.Background(), input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	input := validInput(sender)
	input.Recipients = []string{"DEV@example.com", " dev@example.com "}
	created, err := service.Create(context.Background(), input)
	if err != nil || len(created.Recipients) != 1 {
		t.Fatalf("duplicates were not normalized: %+v %v", created, err)
	}
	now := clock.Now()
	expired := domain.Sender{ID: "expired-1234abcd", Name: "expired", Status: domain.StatusExpired, CreatedAt: now, UpdatedAt: now, LastActivityAt: &now}
	if err = repo.Create(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	input = validInput(expired)
	if _, err = service.Create(context.Background(), input); err == nil {
		t.Fatal("expired sender should be rejected")
	}
}

func TestEnabledAlertRequiresOutlook(t *testing.T) {
	dir := t.TempDir()
	repo := repository.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fixedClock{value: time.Now()}
	emailSettings, err := emailconfig.Open(dir, config.Config{}, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := clock.Now()
	sender := domain.Sender{ID: "worker-1234abcd", Name: "worker", Status: domain.StatusOnline, CreatedAt: now, UpdatedAt: now, LastActivityAt: &now}
	if err = repo.Create(context.Background(), sender); err != nil {
		t.Fatal(err)
	}
	service := NewService(store, repo, emailSettings, clock)
	if _, err = service.Create(context.Background(), validInput(sender)); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("expected provider error, got %v", err)
	}
	input := validInput(sender)
	input.Enabled = false
	if _, err = service.Create(context.Background(), input); err != nil {
		t.Fatalf("inactive alert should be saved: %v", err)
	}
}
