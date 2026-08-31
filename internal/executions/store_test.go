package executions

import (
	"testing"
	"time"

	"logtheater/internal/domain"
)

func TestExecutionLifecycleFiltersAndRestore(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.FixedZone("America/Sao_Paulo", -3*60*60))
	clock := func() time.Time { return now }
	dir := t.TempDir()
	store, err := Open(dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	severity := domain.Error
	record, err := store.Create(Record{SourceType: SourceAlert, SourceID: "alt_1", SourceName: "Erros", SenderID: "api", Severity: &severity, Actions: []ActionResult{{ID: "a1", Type: "send_email", Status: StatusPending}}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(840 * time.Millisecond)
	message := "falha sanitizada\nsem stack"
	record, err = store.Update(record.ID, func(v *Record) {
		v.Status = StatusFailed
		v.ErrorMessage = &message
		v.AttemptCount = 3
		v.Actions[0].Status = StatusFailed
		v.Actions[0].AttemptCount = 3
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.DurationMs == nil || *record.DurationMs != 840 || record.ErrorMessage == nil || *record.ErrorMessage != "falha sanitizada sem stack" {
		t.Fatalf("invalid final record: %#v", record)
	}
	page := store.List(Filters{SourceType: SourceAlert, Statuses: map[Status]bool{StatusFailed: true}, Recent: true, Page: 1, PageSize: 20})
	if len(page.Items) != 1 || page.Pagination.Total != 1 {
		t.Fatalf("invalid filter: %#v", page)
	}
	restored, err := Open(dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := restored.Get(record.ID); !ok || value.Status != StatusFailed {
		t.Fatalf("invalid restoration: %#v %v", value, ok)
	}
}

func TestCleanupPreservesRunningExecutions(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store, err := Open(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-100 * 24 * time.Hour)
	running, err := store.Create(Record{SourceType: SourceEvent, SourceID: "evt", SenderID: "sender", StartedAt: old, Status: StatusProcessing})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := store.Create(Record{SourceType: SourceEvent, SourceID: "evt", SenderID: "sender", StartedAt: old, Status: StatusSuccess})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Cleanup(90*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(running.ID); !ok {
		t.Fatal("running execution was removed")
	}
	if _, ok := store.Get(finished.ID); ok {
		t.Fatal("expired execution was not removed")
	}
}
