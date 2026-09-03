package monitoring

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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
	return RuleInput{Name: "Erro de conexão", SenderIDs: []string{"sender-a"}, Enabled: true, Expression: ExpressionGroup{Operator: LogicalAnd, Nodes: []ExpressionNode{{Condition: &Condition{Type: ConditionSeverity, Operator: "equals", Value: raw(map[string]any{"severity": "ERROR"})}}, {Connector: LogicalAnd, Condition: &Condition{Type: ConditionMessage, Operator: "contains", Value: raw(map[string]any{"text": "ECONNRESET"}), Negated: false}}}}, Actions: []Action{{Type: ActionEmail, Config: raw(map[string]any{"recipients": []string{"ops@example.com"}, "subject": "Failed", "message": "Failed detectada"})}}}
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

func TestHTTPActionAcceptsMethodHeadersCookiesAndBody(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.Actions = []Action{{Type: ActionHTTP, Config: raw(domain.HTTPRequestConfig{Method: "DELETE", URL: "https://api.example.com/items/1", Headers: map[string]string{"Authorization": "Bearer token"}, Cookies: map[string]string{"session": "value"}, Body: `{"reason":"{{log.message}}"}`})}}
	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Actions) != 1 || created.Actions[0].Type != ActionHTTP {
		t.Fatalf("unexpected HTTP action: %#v", created.Actions)
	}
	input.Actions[0].Config = raw(domain.HTTPRequestConfig{Method: "POST", URL: "https://localhost/private"})
	if _, err = service.Create(context.Background(), input); err == nil {
		t.Fatal("unsafe HTTP action was accepted")
	}
}

func TestLogReceivedTriggerMatchesEveryLogWithoutSeverityFilter(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{{Condition: &Condition{Type: ConditionLogReceived, Operator: "received"}}}
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, severity := range everySeverity {
		result := service.Evaluate(rule, domain.Sender{ID: "sender-a"}, domain.LogEntry{SenderID: "sender-a", Severity: severity, Message: "qualquer"}, "", false)
		if !result.Matched {
			t.Fatalf("severity %s should have matched the generic log trigger", severity)
		}
	}
}

func TestMessageRegexUsesValidatedRE2Pattern(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.Expression.Nodes[1].Condition.Operator = "matches_regex"
	input.Expression.Nodes[1].Condition.Value = raw(map[string]any{"text": `(?i)^timeout\s+after\s+\d+ms$`})
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	matching := domain.LogEntry{SenderID: "sender-a", Severity: domain.Error, Message: "TIMEOUT after 2500ms"}
	if result := service.Evaluate(rule, domain.Sender{ID: "sender-a"}, matching, "", false); !result.Matched {
		t.Fatalf("valid regex did not match: %#v", result)
	}
	input.Expression.Nodes[1].Condition.Value = raw(map[string]any{"text": `^(unclosed`})
	if _, err = service.Create(context.Background(), input); err == nil {
		t.Fatal("invalid regex was accepted")
	}
	input.Expression.Nodes[1].Condition.Value = raw(map[string]any{"text": strings.Repeat("a", 501)})
	if _, err = service.Create(context.Background(), input); err == nil {
		t.Fatal("oversized regex was accepted")
	}
}

func TestLegacySeverityTriggerIsMigratedToLogReceived(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	legacy := Rule{ID: "mon_legacy", Name: "Logs", SenderIDs: []string{"sender-a"}, Enabled: true, Status: "active", Expression: ExpressionGroup{ID: "grp", Operator: LogicalAnd, Nodes: []ExpressionNode{{Condition: &Condition{ID: "cond", Type: ConditionSeverity, Operator: "in", Value: raw(map[string]any{"severities": []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}, "source": "log_received"})}}}}}
	if err = store.Put(legacy); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	migrated, ok := reopened.Get("mon_legacy")
	if !ok {
		t.Fatal("legacy rule was not loaded")
	}
	condition := migrated.Expression.Nodes[0].Condition
	if condition.Type != ConditionLogReceived || condition.Operator != "received" || string(condition.Value) != "{}" {
		t.Fatalf("legacy trigger was not migrated: %#v", condition)
	}
	persisted, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := persisted.Get("mon_legacy"); again.Expression.Nodes[0].Condition.Type != ConditionLogReceived {
		t.Fatal("migration was not written back to disk")
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

func TestWaitUntilDefersFridayLogUntilMonday(t *testing.T) {
	service, clock, executor := testService(t)
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	clock.now = time.Date(2026, time.August, 28, 16, 30, 0, 0, location)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{
		{Condition: &Condition{Type: ConditionLogReceived, Operator: "received", Value: raw(map[string]any{})}},
		{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWeekday, Operator: "equals", Value: raw(map[string]any{"weekday": "friday"})}},
		{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWaitUntil, Operator: "next_occurrence", Value: raw(map[string]any{"weekday": "monday", "time": "09:00", "timezone": "America/Sao_Paulo"})}},
	}
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	entry := domain.LogEntry{SenderID: "sender-a", Timestamp: clock.now, Severity: domain.Error, Message: "Friday failure"}
	service.Notify(context.Background(), domain.Sender{ID: "sender-a"}, entry)
	if executor.calls != 0 {
		t.Fatalf("action executed before the wait elapsed: %d", executor.calls)
	}
	pending := service.store.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected one pending evaluation, got %#v", pending)
	}
	wantDueAt := time.Date(2026, time.August, 31, 9, 0, 0, 0, location)
	if !pending[0].DueAt.Equal(wantDueAt) {
		t.Fatalf("unexpected due date: got %s want %s", pending[0].DueAt, wantDueAt)
	}

	clock.now = wantDueAt.Add(-time.Second)
	service.ProcessPending(context.Background())
	if executor.calls != 0 {
		t.Fatal("action executed before Monday at 09:00")
	}
	clock.now = wantDueAt
	service.ProcessPending(context.Background())
	if executor.calls != 1 {
		t.Fatalf("expected action at Monday 09:00, got %d calls", executor.calls)
	}
	if len(service.store.Pending()) != 0 {
		t.Fatal("completed wait was not removed from pending storage")
	}
	loaded, err := service.Get(rule.ID)
	if err != nil || loaded.ExecutionCount != 1 || loaded.LastResult != "success" {
		t.Fatalf("rule execution was not recorded: %#v %v", loaded, err)
	}
}

func TestPureTemporalRuleIsScheduledAndSurvivesRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Date(2026, time.August, 31, 8, 58, 30, 0, location)}
	deps := fakeDeps{sender: domain.Sender{ID: "sender-a", Name: "Sender A", Status: domain.StatusOnline}, email: true}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, deps, clock)
	executor := &fakeExecutor{}
	service.SetExecutor(executor)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{
		{Condition: &Condition{Type: ConditionWeekday, Operator: "equals", Value: raw(map[string]any{"weekday": "monday"})}},
		{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWaitUntil, Operator: "next_occurrence", Value: raw(map[string]any{"weekday": "monday", "time": "09:00", "timezone": "America/Sao_Paulo"})}},
	}
	rule, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	service.ProcessPending(context.Background())
	pending := store.Pending()
	if len(pending) != 1 || pending[0].Trigger.Type != "schedule" {
		t.Fatalf("scheduled evaluation was not persisted: %#v", pending)
	}
	clock.now = time.Date(2026, time.August, 31, 8, 59, 0, 0, location)
	service.ProcessPending(context.Background())
	if executor.calls != 0 {
		t.Fatal("temporal action ran before its configured time")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(reopened, deps, clock)
	restartedExecutor := &fakeExecutor{}
	restarted.SetExecutor(restartedExecutor)
	clock.now = time.Date(2026, time.August, 31, 9, 0, 0, 0, location)
	restarted.ProcessPending(context.Background())
	if restartedExecutor.calls != 1 {
		t.Fatalf("scheduled action calls=%d, want 1", restartedExecutor.calls)
	}
	restarted.ProcessPending(context.Background())
	if restartedExecutor.calls != 1 {
		t.Fatal("the same scheduled minute executed more than once")
	}
	loaded, err := restarted.Get(rule.ID)
	if err != nil || loaded.ExecutionCount != 1 || loaded.LastResult != "success" {
		t.Fatalf("scheduled rule state=%#v err=%v", loaded, err)
	}
}

func TestWaitUntilRejectsInvalidTimezone(t *testing.T) {
	service, _, _ := testService(t)
	input := baseInput()
	input.Expression.Nodes = append(input.Expression.Nodes, ExpressionNode{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWaitUntil, Operator: "next_occurrence", Value: raw(map[string]any{"weekday": "monday", "time": "09:00", "timezone": "not/a-timezone"})}})
	if _, err := service.Create(context.Background(), input); err == nil {
		t.Fatal("expected invalid Wait Until timezone to be rejected")
	}
}

func TestWaitUntilDoesNotScheduleWhenEarlierConditionDoesNotMatch(t *testing.T) {
	service, clock, executor := testService(t)
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	clock.now = time.Date(2026, time.August, 27, 16, 30, 0, 0, location)
	input := baseInput()
	input.Expression.Nodes = []ExpressionNode{
		{Condition: &Condition{Type: ConditionLogReceived, Operator: "received", Value: raw(map[string]any{})}},
		{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWeekday, Operator: "equals", Value: raw(map[string]any{"weekday": "friday"})}},
		{Connector: LogicalAnd, Condition: &Condition{Type: ConditionWaitUntil, Operator: "next_occurrence", Value: raw(map[string]any{"weekday": "monday", "time": "09:00", "timezone": "America/Sao_Paulo"})}},
	}
	if _, err = service.Create(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	service.Notify(context.Background(), domain.Sender{ID: "sender-a"}, domain.LogEntry{SenderID: "sender-a", Timestamp: clock.now, Severity: domain.Error, Message: "Thursday failure"})
	if executor.calls != 0 || len(service.store.Pending()) != 0 {
		t.Fatalf("a non-matching Thursday log must not schedule Monday: calls=%d pending=%#v", executor.calls, service.store.Pending())
	}
}
