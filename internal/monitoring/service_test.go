package monitoring

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"logtheater/internal/domain"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type fakeDeps struct {
	sender domain.Sender
	event  domain.EventDefinition
	alert  domain.EmailAlert
	email  bool
}

func (d fakeDeps) Sender(context.Context, string) (domain.Sender, error) { return d.sender, nil }
func (d fakeDeps) Event(string) (domain.EventDefinition, error)          { return d.event, nil }
func (d fakeDeps) Alert(string) (domain.EmailAlert, error)               { return d.alert, nil }
func (d fakeDeps) EmailReady() bool                                      { return d.email }

type fakeExecutor struct{ calls int }

func (e *fakeExecutor) ExecuteMonitoringAction(context.Context, Rule, Action, domain.Sender, domain.LogEntry, string, int) error {
	e.calls++
	return nil
}
func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func baseInput() RuleInput {
	return RuleInput{Name: "Erro de conexão", SenderIDs: []string{"sender-a"}, Enabled: true, Expression: ExpressionGroup{Operator: LogicalAnd, Nodes: []ExpressionNode{{Condition: &Condition{Type: ConditionSeverity, Operator: "equals", Value: raw(map[string]any{"severity": "ERROR"})}}, {Connector: LogicalAnd, Condition: &Condition{Type: ConditionMessage, Operator: "contains", Value: raw(map[string]any{"text": "ECONNRESET"}), Negated: false}}}}, Actions: []Action{{Type: ActionEmail, Config: raw(map[string]any{"recipients": []string{"ops@example.com"}, "subject": "Falha", "message": "Falha detectada"})}}}
}
func testService(t *testing.T) (*Service, *fakeClock, *fakeExecutor) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, 8, 2, 14, 0, 0, 0, time.Local)}
	deps := fakeDeps{sender: domain.Sender{ID: "sender-a", Name: "Sender A", Status: domain.StatusOnline}, event: domain.EventDefinition{ID: "evt_target", Key: "target_event", Enabled: true, SenderIDs: []string{"sender-a"}}, alert: domain.EmailAlert{ID: "alert-a", Enabled: true, SenderIDs: []string{"sender-a"}, Severities: []domain.LogSeverity{domain.Error}}, email: true}
	service := NewService(store, deps, clock)
	executor := &fakeExecutor{}
	service.SetExecutor(executor)
	return service, clock, executor
}
func TestCRUDPersistenceAndEvaluation(t *testing.T) {
	service, _, executor := testService(t)
	created, err := service.Create(context.Background(), baseInput())
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.LogEntry{SenderID: "sender-a", Timestamp: time.Now(), Severity: domain.Error, Message: "socket ECONNRESET"}
	sender := domain.Sender{ID: "sender-a"}
	result := service.Evaluate(created, sender, entry, "", false)
	if !result.Matched || len(result.Conditions) != 2 {
		t.Fatalf("unexpected evaluation: %#v", result)
	}
	service.Notify(context.Background(), sender, entry)
	if executor.calls != 1 {
		t.Fatalf("expected one async action, got %d", executor.calls)
	}
	loaded, err := service.Get(created.ID)
	if err != nil || loaded.ExecutionCount != 1 {
		t.Fatalf("rule state was not persisted: %#v %v", loaded, err)
	}
	history, err := service.Executions(created.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("execution history missing: %v %v", history, err)
	}
}

func TestSenderStatusTriggerMatchesOnlyTheConfiguredTransition(t *testing.T) {
	service, _, executor := testService(t)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{{Condition: &Condition{Type: ConditionSenderStatus, Operator: "became", Value: raw(map[string]any{"status": "inactive"})}}}
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	sender := domain.Sender{ID: "sender-a", Status: domain.StatusInactive}
	service.NotifySenderStatus(context.Background(), sender, domain.StatusOnline)
	if executor.calls != 1 {
		t.Fatalf("expected inactive transition to execute once, got %d", executor.calls)
	}
	service.NotifySenderStatus(context.Background(), sender, domain.StatusInactive)
	if executor.calls != 1 {
		t.Fatalf("unchanged status must not execute again, got %d", executor.calls)
	}
	loaded, err := service.Get(rule.ID)
	if err != nil || loaded.ExecutionCount != 1 {
		t.Fatalf("status execution was not persisted: %#v %v", loaded, err)
	}
}

func TestNewSendersAreAutomaticallyAssociatedWithOptedInRules(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.SenderIDs = nil
	input.IncludeNewSenders = true
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	service.NotifySenderCreated(context.Background(), domain.Sender{ID: "sender-new", Status: domain.StatusNeverConnected})
	loaded, err := service.Get(rule.ID)
	if err != nil || !contains(loaded.SenderIDs, "sender-new") {
		t.Fatalf("new sender was not associated: %#v %v", loaded, err)
	}
	service.NotifySenderCreated(context.Background(), domain.Sender{ID: "sender-new", Status: domain.StatusNeverConnected})
	loaded, _ = service.Get(rule.ID)
	if len(loaded.SenderIDs) != 1 {
		t.Fatalf("sender association must remain unique: %#v", loaded.SenderIDs)
	}
}
func TestNegationOrAndPendingSurviveRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Now()}
	deps := fakeDeps{sender: domain.Sender{ID: "sender-a", Status: domain.StatusOnline}, event: domain.EventDefinition{ID: "evt_target", Key: "target_event", Enabled: true, SenderIDs: []string{"sender-a"}}, email: true}
	service := NewService(store, deps, clock)
	input := baseInput()
	input.Expression.Operator = LogicalOr
	input.Expression.Nodes[0].Condition.Negated = true
	input.Expression.Nodes[1].Connector = LogicalOr
	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	result := service.Evaluate(created, deps.sender, domain.LogEntry{SenderID: "sender-a", Severity: domain.Info, Message: "ok"}, "", false)
	if !result.Matched {
		t.Fatal("negated OR condition should match")
	}
	pending := PendingEvaluation{ID: "pend_test", RuleID: created.ID, SenderID: "sender-a", DueAt: clock.now.Add(time.Minute), Status: "pending"}
	if err = store.PutPending(pending); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Pending()) != 1 || len(reopened.All()) != 1 {
		t.Fatal("rules or pending evaluations did not survive restart")
	}
}
func TestDirectEventCycleIsRejected(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{{Condition: &Condition{Type: ConditionEvent, Operator: "triggered", Value: raw(map[string]any{"event_key": "target_event"})}}}
	input.Actions = []Action{{Type: ActionEvent, Config: raw(map[string]any{"event_id": "evt_target", "message": "loop", "severity": "INFO"})}}
	if _, err := service.Create(context.Background(), input); err == nil {
		t.Fatal("expected direct cycle validation error")
	}
}

func TestIncompleteDraftCanBePersistedButNotEnabled(t *testing.T) {
	service, _, _ := testService(t)
	draft, err := service.Create(context.Background(), RuleInput{Name: "Rascunho inicial", Status: "draft", Expression: ExpressionGroup{Operator: LogicalAnd}})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != "draft" || draft.Enabled {
		t.Fatalf("unexpected draft: %#v", draft)
	}
	if _, err = service.SetEnabled(context.Background(), draft.ID, true); err == nil {
		t.Fatal("an incomplete draft must not be enabled")
	}
}
