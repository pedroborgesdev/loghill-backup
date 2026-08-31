package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/repositories"
	settingsstore "logtheater/internal/settings"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }

func testConfig(dir string) config.Config {
	return config.Config{DataDir: dir, MaxLogLines: 10, CompactTarget: 8, CompactKeep: 2, MaxPageSize: 100, MaxMessageSize: 1024, MaxMetadataSize: 1024, InactiveAfter: 5 * time.Minute, DeleteAfter: 7 * 24 * time.Hour, SSEBuffer: 10, SSEMaxClients: 10}
}
func TestNormalizeName(t *testing.T) {
	got, err := NormalizeName("  Automação Teste  ")
	if err != nil || got != "automacao-teste" {
		t.Fatalf("got %q, %v", got, err)
	}
	got, err = NormalizeName(" Consulta PJe - TRF3 ")
	if err != nil || got != "consulta-pje-trf3" {
		t.Fatalf("got %q, %v", got, err)
	}
	got, err = NormalizeName("Cobrança / Santander")
	if err != nil || got != "cobranca-santander" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err = NormalizeName("///"); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("expected empty normalized name, got %v", err)
	}
}

func TestReceiveLogPersistsAndStreamsEvent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC)}
	settings, err := settingsstore.Open(dir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dir), clock, settings)
	sender, credentials, err := svc.CreateSender(context.Background(), "Worker Event", "")
	if err != nil {
		t.Fatal(err)
	}
	stream, cancel, err := svc.Hub.Subscribe(sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	entry, _, err := svc.ReceiveLogWithEvent(context.Background(), sender.ID, credentials.SenderKey, "INFO", "concluído", "processamento_finalizado", "ocorrencia-123", nil, map[string]any{"protocolo": "ABC"})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Event != "processamento_finalizado" || entry.SenderID != sender.ID {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	select {
	case streamed := <-stream:
		if streamed.Event != entry.Event {
			t.Fatalf("SSE lost event: %+v", streamed)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not published")
	}
	page, err := svc.Logs(context.Background(), sender.ID, domain.LogFilters{Page: 1, PageSize: 10, Order: "desc", EventMode: "with", EventKey: entry.Event})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].EventOccurrenceID != "ocorrencia-123" {
		t.Fatalf("event was not persisted: %+v", page.Items)
	}
	if _, _, err = svc.ReceiveLogWithEvent(context.Background(), sender.ID, credentials.SenderKey, "INFO", "inválido", "../evento", "", nil, nil); !errors.Is(err, domain.ErrInvalidEventKey) {
		t.Fatalf("expected invalid event key, got %v", err)
	}
}

func TestReplayedLogKeepsOriginInstanceAndOriginalActivityTime(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 8, 28, 18, 30, 0, 0, time.UTC)}
	settings, err := settingsstore.Open(dir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dir), clock, settings)
	sender, _, err := svc.CreateSender(context.Background(), "Worker Replay", "")
	if err != nil {
		t.Fatal(err)
	}
	_, origin, _, err := svc.InitInstanceByName(context.Background(), sender.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, current, currentToken, err := svc.InitInstanceByName(context.Background(), sender.Name)
	if err != nil {
		t.Fatal(err)
	}
	originalTime := clock.now.Add(-10 * time.Minute)
	entry, _, err := svc.ReceiveLogWithAuthenticatedInstanceAndEvent(
		context.Background(), sender.ID, "", current.ID, currentToken, origin.ID,
		"UNDEFINED", "Finished server process", "", "", &originalTime, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if entry.InstanceID != origin.ID {
		t.Fatalf("replayed log instance=%q, want %q", entry.InstanceID, origin.ID)
	}
	page, err := svc.Logs(context.Background(), sender.ID, domain.LogFilters{InstanceID: origin.ID, Page: 1, PageSize: 10, Order: "asc"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Message != "Finished server process" {
		t.Fatalf("origin logs=%+v err=%v", page.Items, err)
	}
	instances, err := svc.Instances(context.Background(), sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundOrigin := false
	for _, instance := range instances {
		if instance.ID == origin.ID {
			foundOrigin = true
			if instance.Status != domain.StatusInactive {
				t.Fatalf("origin instance status=%q, want inactive", instance.Status)
			}
		}
	}
	if !foundOrigin {
		t.Fatalf("origin instance %q was not listed", origin.ID)
	}
}

func TestLifecycleAndCompaction(t *testing.T) {
	dir := t.TempDir()
	repo := repositories.New(filepath.Join(dir, "data"))
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)}
	settings, err := settingsstore.Open(filepath.Join(dir, "data"), clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(filepath.Join(dir, "data")), clock, settings)
	ctx := context.Background()
	if _, err = svc.UpdateSettings(domain.Settings{
		LogLimit:             domain.NumberUnitValue{Value: 10, Unit: domain.StorageLines},
		InactivePreservation: domain.NumberUnitValue{Value: 2, Unit: domain.StorageLines},
		InactiveAfterSeconds: 300,
		DeleteInactiveDays:   7,
	}); err != nil {
		t.Fatal(err)
	}
	sender, credentials, err := svc.CreateSender(ctx, "Worker", "")
	if err != nil {
		t.Fatal(err)
	}
	if sender.ID != "worker" || sender.Status != domain.StatusNeverConnected {
		t.Fatalf("unexpected id %q", sender.ID)
	}
	for i := 0; i < 5; i++ {
		if _, _, err = svc.ReceiveLog(ctx, sender.ID, credentials.SenderKey, "info", "line", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	clock.now = clock.now.Add(6 * time.Minute)
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	sender, err = svc.Get(ctx, sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sender.Status != domain.StatusInactive || sender.LogLineCount != 2 {
		t.Fatalf("unexpected inactive state: %+v", sender)
	}
	if _, _, err = svc.Health(ctx, sender.ID, credentials.SenderKey); err != nil {
		t.Fatal(err)
	}
	sender, _ = svc.Get(ctx, sender.ID)
	if sender.Status != domain.StatusOnline {
		t.Fatalf("expected online, got %s", sender.Status)
	}
}

func TestSendersGroupedByName(t *testing.T) {
	dir := t.TempDir()
	repo := repositories.New(filepath.Join(dir, "data"))
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)}
	settings, err := settingsstore.Open(filepath.Join(dir, "data"), clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(filepath.Join(dir, "data")), clock, settings)
	ctx := context.Background()

	for _, sender := range []domain.Sender{
		{ID: "simulador-teste-old-a", Name: "simulador-teste", Status: domain.StatusNeverConnected, CreatedAt: clock.now, UpdatedAt: clock.now},
		{ID: "simulador-teste-old-b", Name: "simulador-teste", Status: domain.StatusNeverConnected, CreatedAt: clock.now.Add(time.Second), UpdatedAt: clock.now.Add(time.Second)},
		{ID: "worker", Name: "worker", Status: domain.StatusNeverConnected, CreatedAt: clock.now.Add(2 * time.Second), UpdatedAt: clock.now.Add(2 * time.Second)},
	} {
		if err := repo.Create(ctx, sender); err != nil {
			t.Fatal(err)
		}
	}

	page, err := svc.Senders(ctx, domain.SenderFilters{
		GroupByName: true,
		Sort:        "name",
		Order:       "asc",
		Page:        1,
		PageSize:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Pagination.Total != 2 || page.Pagination.TotalPages != 2 {
		t.Fatalf("unexpected grouped pagination: %+v", page.Pagination)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected both instances in the first group, got %d", len(page.Items))
	}
	for _, sender := range page.Items {
		if sender.Name != "simulador-teste" {
			t.Fatalf("unexpected sender in grouped page: %+v", sender)
		}
	}
}

func TestSettingsAreAppliedWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)}
	store, err := settingsstore.Open(dataDir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dataDir), clock, store)
	updated, err := svc.UpdateSettings(domain.Settings{
		LogLimit:             domain.NumberUnitValue{Value: 3, Unit: domain.StorageLines},
		InactivePreservation: domain.NumberUnitValue{Value: 1, Unit: domain.StorageLines},
		InactiveAfterSeconds: 300,
		DeleteInactiveDays:   7,
	})
	if err != nil || updated.LogLimit.Value != 3 {
		t.Fatalf("could not update settings: %+v, %v", updated, err)
	}
	sender, credentials, err := svc.CreateSender(context.Background(), "dynamic-worker", "")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		if _, _, err = svc.ReceiveLog(context.Background(), sender.ID, credentials.SenderKey, "INFO", "line", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	sender, _ = svc.Get(context.Background(), sender.ID)
	if sender.LogLineCount != 2 {
		t.Fatalf("dynamic line limit was not applied: %+v", sender)
	}
	clock.now = clock.now.Add(6 * time.Minute)
	if err = svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	sender, _ = svc.Get(context.Background(), sender.ID)
	if sender.LogLineCount != 1 || sender.Status != domain.StatusInactive {
		t.Fatalf("dynamic preservation was not applied: %+v", sender)
	}
}

func TestDashboardAndSenderCountOnlyActiveInstances(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)}
	store, err := settingsstore.Open(dataDir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dataDir), clock, store)
	ctx := context.Background()
	sender, oldInstance, _, err := svc.InitInstanceByName(ctx, "Worker Dashboard")
	if err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(6 * time.Minute)
	_, activeInstance, _, err := svc.InitInstanceByName(ctx, sender.Name)
	if err != nil {
		t.Fatal(err)
	}
	if oldInstance.ID == activeInstance.ID {
		t.Fatal("initializations returned the same instance")
	}

	page, err := svc.Senders(ctx, domain.SenderFilters{Page: 1, PageSize: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("senders=%+v err=%v", page.Items, err)
	}
	if page.Items[0].InstanceCount != 1 {
		t.Fatalf("instance_count=%d, want only one active instance", page.Items[0].InstanceCount)
	}

	summary, err := svc.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	instances, ok := summary["instances"].(map[string]int64)
	if !ok || instances["active"] != 1 || instances["inactive"] != 1 {
		t.Fatalf("unexpected instance summary: %#v", summary["instances"])
	}
}

func TestTickPermanentlyDeletesSenderWhenLastInstanceExpires(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)}
	store, err := settingsstore.Open(dataDir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dataDir), clock, store)
	if _, err = svc.UpdateSettings(domain.Settings{
		LogLimit:             domain.NumberUnitValue{Value: 10, Unit: domain.StorageLines},
		InactivePreservation: domain.NumberUnitValue{Value: 2, Unit: domain.StorageLines},
		InactiveAfterSeconds: 300,
		DeleteInactiveDays:   1,
	}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sender, instance, token, err := svc.InitInstanceByName(ctx, "Worker Expirável")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.ReceiveLogWithAuthenticatedInstanceAndEvent(
		ctx, sender.ID, "", instance.ID, token, instance.ID,
		"INFO", "primeiro log", "", "", nil, nil,
	); err != nil {
		t.Fatal(err)
	}

	clock.now = clock.now.Add(6 * time.Minute)
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.Instances(ctx, sender.ID)
	if err != nil || len(listed) != 1 || listed[0].Status != domain.StatusInactive {
		t.Fatalf("instance should wait inactive before deletion: %+v err=%v", listed, err)
	}

	clock.now = clock.now.Add(24 * time.Hour)
	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, sender.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sender should disappear after its last instance expires: %v", err)
	}
	if _, err = os.Stat(filepath.Join(dataDir, "senders", sender.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired sender directory still exists: %v", err)
	}
	summary, err := svc.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	senders := summary["senders"].(map[string]int64)
	instances := summary["instances"].(map[string]int64)
	if senders["total"] != 0 || instances["active"] != 0 || instances["inactive"] != 0 {
		t.Fatalf("expired sender remained visible in dashboard: %#v", summary)
	}
}

func TestTickDeletesPreviouslyExpiredSender(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)}
	store, err := settingsstore.Open(dataDir, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, testConfig(dataDir), clock, store)
	ctx := context.Background()
	sender, _, err := svc.CreateSender(ctx, "Sender Expirado Antigo", "")
	if err != nil {
		t.Fatal(err)
	}
	sender.Status = domain.StatusExpired
	sender.UpdatedAt = clock.Now()
	if err = repo.Update(ctx, sender); err != nil {
		t.Fatal(err)
	}

	if err = svc.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, sender.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("previously expired sender should be permanently deleted: %v", err)
	}
}
