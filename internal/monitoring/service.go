package monitoring

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"logtheater/internal/domain"
	"logtheater/internal/executions"
	"logtheater/internal/validation"
)

var ErrNotFound = errors.New("monitoring rule not found")

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Message }

type Dependencies interface {
	Sender(context.Context, string) (domain.Sender, error)
	Event(string) (domain.EventDefinition, error)
	Alert(string) (domain.EmailAlert, error)
	EmailReady() bool
}
type ActionExecutor interface {
	ExecuteMonitoringAction(context.Context, Rule, Action, domain.Sender, domain.LogEntry, string, int) error
}

type Service struct {
	store      *Store
	deps       Dependencies
	clock      domain.Clock
	executor   ActionExecutor
	executions *executions.Store
	mu         sync.RWMutex
	bySender   map[string][]string
	byEvent    map[string][]string
	byAlert    map[string][]string
}

func NewService(store *Store, deps Dependencies, clock domain.Clock) *Service {
	s := &Service{store: store, deps: deps, clock: clock, bySender: map[string][]string{}, byEvent: map[string][]string{}, byAlert: map[string][]string{}}
	s.rebuild()
	return s
}
func (s *Service) SetExecutor(v ActionExecutor)      { s.executor = v }
func (s *Service) SetExecutions(v *executions.Store) { s.executions = v }
func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Service) List(f Filters) Page {
	items := s.store.All()
	filtered := make([]Rule, 0, len(items))
	var total, active, recent, failures int64
	now := s.clock.Now()
	for _, r := range items {
		total++
		if r.Enabled && r.Status != "draft" {
			active++
		}
		if r.LastExecutedAt != nil && now.Sub(*r.LastExecutedAt) <= 24*time.Hour {
			recent++
		}
		if r.LastError != "" && r.LastEvaluatedAt != nil && now.Sub(*r.LastEvaluatedAt) <= 24*time.Hour {
			failures++
		}
		if f.Enabled != nil && r.Enabled != *f.Enabled {
			continue
		}
		if f.SenderID != "" && !contains(r.SenderIDs, f.SenderID) {
			continue
		}
		if f.SenderName != "" && !s.matchesSenderName(r.SenderIDs, f.SenderName) {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(r.Name+" "+r.Description), strings.ToLower(f.Search)) {
			continue
		}
		if f.ConditionType != "" && !hasCondition(r.Expression, f.ConditionType) {
			continue
		}
		if f.ActionType != "" && !hasAction(r.Actions, f.ActionType) {
			continue
		}
		filtered = append(filtered, r)
	}
	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	pages := int(math.Ceil(float64(len(filtered)) / float64(pageSize)))
	if pages < 1 {
		pages = 1
	}
	return Page{Items: filtered[start:end], Pagination: domain.Pagination{Page: page, PageSize: pageSize, Total: int64(len(filtered)), TotalPages: pages, Returned: end - start}, Summary: map[string]int64{"total": total, "active": active, "recent_executions": recent, "recent_failures": failures}}
}

func (s *Service) matchesSenderName(ids []string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, id := range ids {
		sender, err := s.deps.Sender(context.Background(), id)
		if err == nil && strings.Contains(strings.ToLower(sender.Name), query) {
			return true
		}
	}
	return false
}

func (s *Service) Get(id string) (Rule, error) {
	v, ok := s.store.Get(id)
	if !ok {
		return Rule{}, ErrNotFound
	}
	return v, nil
}
func (s *Service) Create(ctx context.Context, in RuleInput) (Rule, error) {
	if err := s.Validate(ctx, in, ""); err != nil {
		return Rule{}, err
	}
	now := s.clock.Now()
	normalizeInput(&in)
	r := Rule{ID: newID("mon_"), Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), SenderIDs: in.SenderIDs, IncludeNewSenders: in.IncludeNewSenders, Expression: in.Expression, Actions: in.Actions, Enabled: in.Enabled && in.Status != "draft", Status: in.Status, CreatedAt: now, UpdatedAt: now}
	assignIDs(&r)
	if err := s.store.Put(r); err != nil {
		return Rule{}, err
	}
	s.rebuild()
	slog.Info("monitoring rule created", "rule_id", r.ID)
	return r, nil
}
func (s *Service) Update(ctx context.Context, id string, in RuleInput) (Rule, error) {
	old, err := s.Get(id)
	if err != nil {
		return Rule{}, err
	}
	if err = s.Validate(ctx, in, id); err != nil {
		return Rule{}, err
	}
	normalizeInput(&in)
	old.Name, old.Description, old.SenderIDs, old.IncludeNewSenders, old.Expression, old.Actions, old.Enabled, old.Status = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), in.SenderIDs, in.IncludeNewSenders, in.Expression, in.Actions, in.Enabled && in.Status != "draft", in.Status
	old.UpdatedAt = s.clock.Now()
	assignIDs(&old)
	if err = s.store.Put(old); err != nil {
		return Rule{}, err
	}
	s.rebuild()
	slog.Info("monitoring rule updated", "rule_id", id)
	return old, nil
}
func (s *Service) Delete(id string) error {
	ok, err := s.store.Delete(id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	s.rebuild()
	slog.Info("monitoring rule deleted", "rule_id", id)
	return nil
}
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (Rule, error) {
	r, err := s.Get(id)
	if err != nil {
		return Rule{}, err
	}
	if r.Status == "draft" && enabled {
		return Rule{}, &ValidationError{"status", "Complete the draft before enabling it."}
	}
	in := RuleInput{Name: r.Name, Description: r.Description, SenderIDs: r.SenderIDs, IncludeNewSenders: r.IncludeNewSenders, Expression: r.Expression, Actions: r.Actions, Enabled: enabled, Status: r.Status}
	return s.Update(ctx, id, in)
}
func (s *Service) Duplicate(ctx context.Context, id string) (Rule, error) {
	r, err := s.Get(id)
	if err != nil {
		return Rule{}, err
	}
	return s.Create(ctx, RuleInput{Name: "Copy of " + r.Name, Description: r.Description, SenderIDs: r.SenderIDs, IncludeNewSenders: r.IncludeNewSenders, Expression: r.Expression, Actions: r.Actions, Enabled: false, Status: "draft"})
}

func normalizeInput(in *RuleInput) {
	if in.Status == "" {
		in.Status = "active"
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range in.SenderIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	in.SenderIDs = ids
	if in.Expression.Operator == "" {
		in.Expression.Operator = LogicalAnd
	}
}
func assignIDs(r *Rule) {
	if r.Expression.ID == "" {
		r.Expression.ID = newID("grp_")
	}
	var walk func(*ExpressionGroup)
	walk = func(g *ExpressionGroup) {
		for i := range g.Nodes {
			if g.Nodes[i].Condition != nil && g.Nodes[i].Condition.ID == "" {
				g.Nodes[i].Condition.ID = newID("cond_")
			}
			if g.Nodes[i].Group != nil {
				if g.Nodes[i].Group.ID == "" {
					g.Nodes[i].Group.ID = newID("grp_")
				}
				walk(g.Nodes[i].Group)
			}
		}
	}
	walk(&r.Expression)
	for i := range r.Actions {
		if r.Actions[i].ID == "" {
			r.Actions[i].ID = newID("act_")
		}
	}
}
func (s *Service) Validate(ctx context.Context, in RuleInput, currentID string) error {
	normalizeInput(&in)
	n := len([]rune(strings.TrimSpace(in.Name)))
	if n < 3 || n > 100 {
		return &ValidationError{"name", "The name must be between 3 and 100 characters."}
	}
	if len([]rune(in.Description)) > 500 {
		return &ValidationError{"description", "The description must be at most 500 characters."}
	}
	if in.Status == "draft" {
		return s.validateDraft(ctx, in)
	}
	if in.Status != "active" {
		return &ValidationError{"status", "Invalid rule status."}
	}
	if !in.IncludeNewSenders && (len(in.SenderIDs) < 1 || len(in.SenderIDs) > 100) {
		return &ValidationError{"sender_ids", "Select between 1 and 100 senders."}
	}
	for _, id := range in.SenderIDs {
		v, err := s.deps.Sender(ctx, id)
		if err != nil {
			return &ValidationError{"sender_ids", "A selected sender does not exist."}
		}
		if v.Status == domain.StatusExpired || v.Status == domain.StatusRevoked {
			return &ValidationError{"sender_ids", "Expired or revoked senders cannot be monitored."}
		}
	}
	count, depth := 0, 0
	if err := s.validateGroup(in.Expression, 1, &count, &depth); err != nil {
		return err
	}
	if count < 1 {
		return &ValidationError{"expression", "Add at least one condition."}
	}
	if count > 50 || depth > 5 {
		return &ValidationError{"expression", "The rule exceeds the condition or group limits."}
	}
	if len(in.Actions) < 1 || len(in.Actions) > 10 {
		return &ValidationError{"actions", "Add between 1 and 10 actions."}
	}
	hasTrigger := false
	walkConditions(in.Expression, func(c Condition) {
		if !c.Negated && (c.Type == ConditionEvent || c.Type == ConditionAlert || c.Type == ConditionSenderStatus || c.Type == ConditionLogReceived || c.Type == ConditionMessage || c.Type == ConditionSeverity || c.Type == ConditionMetadata) {
			hasTrigger = true
		}
	})
	if !hasTrigger {
		return &ValidationError{"expression", "Add a positive log, event, or alert trigger."}
	}
	for _, a := range in.Actions {
		if err := s.validateAction(a, in.SenderIDs, in.Enabled); err != nil {
			return err
		}
	}
	if err := s.validateCycles(currentID, in); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateDraft(ctx context.Context, in RuleInput) error {
	if !in.IncludeNewSenders && len(in.SenderIDs) > 100 {
		return &ValidationError{"sender_ids", "The draft exceeds the sender limit."}
	}
	for _, id := range in.SenderIDs {
		sender, err := s.deps.Sender(ctx, id)
		if err != nil {
			return &ValidationError{"sender_ids", "A selected sender does not exist."}
		}
		if sender.Status == domain.StatusExpired || sender.Status == domain.StatusRevoked {
			return &ValidationError{"sender_ids", "Expired or revoked senders cannot be monitored."}
		}
	}
	if len(in.Actions) > 10 {
		return &ValidationError{"actions", "The draft exceeds the action limit."}
	}
	count := 0
	walkConditions(in.Expression, func(Condition) { count++ })
	if count > 50 {
		return &ValidationError{"expression", "The draft exceeds the condition limit."}
	}
	return nil
}
func (s *Service) validateGroup(g ExpressionGroup, level int, count, depth *int) error {
	if level > *depth {
		*depth = level
	}
	if g.Operator != LogicalAnd && g.Operator != LogicalOr {
		return &ValidationError{"expression.operator", "Use AND or OR to combine conditions."}
	}
	if len(g.Nodes) == 0 {
		return &ValidationError{"expression.nodes", "A group cannot be empty."}
	}
	for _, n := range g.Nodes {
		if (n.Condition == nil) == (n.Group == nil) {
			return &ValidationError{"expression.nodes", "Each block must contain a condition or group."}
		}
		if n.Condition != nil {
			*count++
			if err := validateCondition(*n.Condition); err != nil {
				return err
			}
		} else if err := s.validateGroup(*n.Group, level+1, count, depth); err != nil {
			return err
		}
	}
	return nil
}
func rawMap(raw json.RawMessage) (map[string]any, error) {
	var v map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return nil, errors.New("empty configuration")
	}
	return v, nil
}
func stringValue(v map[string]any, key string) string {
	x, _ := v[key].(string)
	return strings.TrimSpace(x)
}
func validateCondition(c Condition) error {
	if c.Type == ConditionLogReceived {
		if c.Operator != "received" {
			return &ValidationError{"expression", "The operator is not compatible with the condition."}
		}
		return nil
	}
	v, err := rawMap(c.Value)
	if err != nil {
		return &ValidationError{"expression", "Fill in the condition values."}
	}
	valid := map[ConditionType]map[string]bool{ConditionEvent: {"triggered": true, "not_triggered": true, "previously_triggered": true, "not_previously_triggered": true}, ConditionAlert: {"triggered": true, "not_triggered": true, "previously_triggered": true, "not_previously_triggered": true}, ConditionSenderStatus: {"became": true}, ConditionMessage: {"contains": true, "not_contains": true, "equals": true, "not_equals": true, "starts_with": true, "not_starts_with": true, "ends_with": true, "not_ends_with": true}, ConditionSeverity: {"equals": true, "not_equals": true, "in": true, "not_in": true}, ConditionMetadata: {"exists": true, "not_exists": true, "equals": true, "not_equals": true, "contains": true, "not_contains": true, "gt": true, "gte": true, "lt": true, "lte": true}, ConditionTime: {"between": true, "not_between": true, "after": true, "before": true}, ConditionWeekday: {"equals": true, "not_equals": true, "in": true, "not_in": true}, ConditionDate: {"between": true, "after": true, "before": true}}
	ops, ok := valid[c.Type]
	if !ok || !ops[c.Operator] {
		return &ValidationError{"expression", "The operator is not compatible with the condition."}
	}
	if c.Type == ConditionMetadata && stringValue(v, "path") == "" {
		return &ValidationError{"expression", "Enter the metadata path."}
	}
	if c.Type == ConditionSenderStatus && stringValue(v, "status") != "online" && stringValue(v, "status") != "inactive" {
		return &ValidationError{"expression", "Select active or inactive status."}
	}
	return nil
}
func (s *Service) validateAction(a Action, senders []string, enabled bool) error {
	v, err := rawMap(a.Config)
	if err != nil {
		return &ValidationError{"actions", "Fill in the action configuration."}
	}
	switch a.Type {
	case ActionEvent:
		id := stringValue(v, "event_id")
		e, err := s.deps.Event(id)
		if err != nil || !e.Enabled {
			return &ValidationError{"actions", "Select an active event."}
		}
		compatible := false
		for _, sid := range senders {
			if contains(e.SenderIDs, sid) {
				compatible = true
			}
		}
		if !compatible {
			return &ValidationError{"actions", "The event is not compatible with the rule senders."}
		}
	case ActionEmail:
		if enabled && !s.deps.EmailReady() {
			return &ValidationError{"actions", "Configure email before enabling this action."}
		}
		var recipients []string
		_ = json.Unmarshal(mustJSON(v["recipients"]), &recipients)
		if len(recipients) < 1 || len(recipients) > 20 {
			return &ValidationError{"actions", "Enter between 1 and 20 recipients."}
		}
		for _, recipient := range recipients {
			if _, ok := validation.EmailAddress(recipient); !ok {
				return &ValidationError{"actions", "There is an invalid email recipient."}
			}
		}
		subject, message := stringValue(v, "subject"), stringValue(v, "message")
		if subject == "" || strings.ContainsAny(subject, "\r\n") || len([]rune(subject)) > 200 {
			return &ValidationError{"actions", "Enter a valid subject without line breaks."}
		}
		if message == "" || len([]rune(message)) > 10_000 {
			return &ValidationError{"actions", "The message must be between 1 and 10,000 characters."}
		}
	default:
		return &ValidationError{"actions", "Invalid action type."}
	}
	return nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Service) validateCycles(current string, in RuleInput) error {
	graph := map[string][]string{}
	add := func(expression ExpressionGroup, actions []Action) {
		triggers := eventTriggers(expression)
		targets := s.eventTargets(actions)
		for _, from := range triggers {
			graph[from] = append(graph[from], targets...)
		}
	}
	for _, rule := range s.store.All() {
		if rule.ID != current {
			add(rule.Expression, rule.Actions)
		}
	}
	add(in.Expression, in.Actions)
	visiting, visited := map[string]bool{}, map[string]bool{}
	var cycle func(string) bool
	cycle = func(node string) bool {
		if visiting[node] {
			return true
		}
		if visited[node] {
			return false
		}
		visiting[node] = true
		for _, next := range graph[node] {
			if cycle(next) {
				return true
			}
		}
		visiting[node] = false
		visited[node] = true
		return false
	}
	for node := range graph {
		if cycle(node) {
			return &ValidationError{"actions", "The action creates a direct or indirect cycle between events and rules."}
		}
	}
	return nil
}
func eventTriggers(expression ExpressionGroup) []string {
	values := []string{}
	walkConditions(expression, func(condition Condition) {
		if condition.Type == ConditionEvent && !condition.Negated && !strings.Contains(condition.Operator, "not_") {
			value, _ := rawMap(condition.Value)
			if key := stringValue(value, "event_key"); key != "" {
				values = append(values, key)
			}
		}
	})
	return values
}
func (s *Service) eventTargets(actions []Action) []string {
	values := []string{}
	for _, action := range actions {
		if action.Type != ActionEvent {
			continue
		}
		value, _ := rawMap(action.Config)
		event, err := s.deps.Event(stringValue(value, "event_id"))
		if err == nil {
			values = append(values, event.Key)
		}
	}
	return values
}

func hasCondition(g ExpressionGroup, t ConditionType) bool {
	found := false
	walkConditions(g, func(c Condition) {
		if c.Type == t {
			found = true
		}
	})
	return found
}
func hasAction(a []Action, t ActionType) bool {
	for _, v := range a {
		if v.Type == t {
			return true
		}
	}
	return false
}
func walkConditions(g ExpressionGroup, fn func(Condition)) {
	for _, n := range g.Nodes {
		if n.Condition != nil {
			fn(*n.Condition)
		} else if n.Group != nil {
			walkConditions(*n.Group, fn)
		}
	}
}

func (s *Service) Evaluate(rule Rule, sender domain.Sender, entry domain.LogEntry, alertID string, expired bool) EvaluationResult {
	results := []ConditionResult{}
	matched, pending := s.evalGroup(rule.Expression, entry, alertID, expired, &results)
	actions := []ActionType{}
	if matched && !pending {
		for _, a := range rule.Actions {
			actions = append(actions, a.Type)
		}
	}
	return EvaluationResult{Matched: matched, Pending: pending, Conditions: results, Actions: actions, Summary: Summary(rule)}
}
func (s *Service) evalGroup(g ExpressionGroup, e domain.LogEntry, alertID string, expired bool, out *[]ConditionResult) (bool, bool) {
	value := g.Operator == LogicalAnd
	pending := false
	for _, n := range g.Nodes {
		var matched, p bool
		if n.Condition != nil {
			matched, p = s.evalCondition(*n.Condition, e, alertID, expired)
			*out = append(*out, ConditionResult{ID: n.Condition.ID, Matched: matched, Description: conditionSummary(*n.Condition)})
		} else {
			matched, p = s.evalGroup(*n.Group, e, alertID, expired, out)
		}
		pending = pending || p
		if g.Operator == LogicalAnd {
			value = value && matched
		} else {
			value = value || matched
		}
	}
	if g.Negated {
		value = !value
	}
	return value, pending
}
func (s *Service) evalCondition(c Condition, e domain.LogEntry, alertID string, expired bool) (bool, bool) {
	v, _ := rawMap(c.Value)
	var matched bool
	switch c.Type {
	case ConditionEvent:
		key := stringValue(v, "event_key")
		negative := strings.Contains(c.Operator, "not_")
		if negative && numberValue(v, "window_minutes") > 0 && !expired {
			return true, true
		}
		matched = e.Event == key
		if negative {
			matched = !matched
		}
	case ConditionAlert:
		id := stringValue(v, "alert_id")
		matched = alertID == id
		if !matched && alertID == "" {
			if alert, err := s.deps.Alert(id); err == nil && alert.Enabled && contains(alert.SenderIDs, e.SenderID) && containsSeverity(alert.Severities, e.Severity) {
				matched = true
			}
		}
		if strings.Contains(c.Operator, "not_") {
			matched = !matched
		}
	case ConditionSenderStatus:
		matched = stringValue(v, "status") == stringValue(e.Metadata, "sender_status") && stringValue(e.Metadata, "previous_sender_status") != stringValue(e.Metadata, "sender_status")
	case ConditionLogReceived:
		matched = true
	case ConditionMessage:
		matched = compareText(e.Message, stringValue(v, "text"), c.Operator)
	case ConditionSeverity:
		matched = compareSeverity(e.Severity, v, c.Operator)
	case ConditionMetadata:
		value, exists := metadataPath(e.Metadata, stringValue(v, "path"))
		matched = compareMetadata(value, exists, v["value"], c.Operator)
	case ConditionTime:
		matched = compareTime(e.Timestamp, v, c.Operator)
	case ConditionWeekday:
		matched = compareWeekday(e.Timestamp, v, c.Operator)
	case ConditionDate:
		matched = compareDate(e.Timestamp, v, c.Operator)
	}
	if c.Negated {
		matched = !matched
	}
	return matched, false
}
func compareText(a, b, op string) bool {
	switch op {
	case "contains":
		return strings.Contains(a, b)
	case "not_contains":
		return !strings.Contains(a, b)
	case "equals":
		return a == b
	case "not_equals":
		return a != b
	case "starts_with":
		return strings.HasPrefix(a, b)
	case "not_starts_with":
		return !strings.HasPrefix(a, b)
	case "ends_with":
		return strings.HasSuffix(a, b)
	case "not_ends_with":
		return !strings.HasSuffix(a, b)
	}
	return false
}
func compareSeverity(s domain.LogSeverity, v map[string]any, op string) bool {
	target := stringValue(v, "severity")
	if list, ok := v["severities"].([]any); ok {
		hit := false
		for _, x := range list {
			if fmt.Sprint(x) == string(s) {
				hit = true
			}
		}
		if op == "not_in" {
			return !hit
		}
		return hit
	}
	if op == "not_equals" {
		return string(s) != target
	}
	return string(s) == target
}
func containsSeverity(values []domain.LogSeverity, value domain.LogSeverity) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}
func metadataPath(m map[string]any, path string) (any, bool) {
	var current any = m
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		n, e := x.Float64()
		return n, e == nil
	case string:
		n, e := strconv.ParseFloat(x, 64)
		return n, e == nil
	}
	return 0, false
}
func numberValue(v map[string]any, key string) float64 { n, _ := number(v[key]); return n }
func compareMetadata(a any, exists bool, b any, op string) bool {
	switch op {
	case "exists":
		return exists
	case "not_exists":
		return !exists
	}
	if !exists {
		return false
	}
	switch op {
	case "equals":
		return reflect.DeepEqual(a, b)
	case "not_equals":
		return !reflect.DeepEqual(a, b)
	case "contains":
		return strings.Contains(fmt.Sprint(a), fmt.Sprint(b))
	case "not_contains":
		return !strings.Contains(fmt.Sprint(a), fmt.Sprint(b))
	}
	x, xok := number(a)
	y, yok := number(b)
	if !xok || !yok {
		return false
	}
	switch op {
	case "gt":
		return x > y
	case "gte":
		return x >= y
	case "lt":
		return x < y
	case "lte":
		return x <= y
	}
	return false
}
func compareTime(t time.Time, v map[string]any, op string) bool {
	current := t.Format("15:04")
	start, end := stringValue(v, "start"), stringValue(v, "end")
	switch op {
	case "between":
		return current >= start && current <= end
	case "not_between":
		return !(current >= start && current <= end)
	case "after":
		return current > start
	case "before":
		return current < start
	}
	return false
}
func compareWeekday(t time.Time, v map[string]any, op string) bool {
	day := strings.ToLower(t.Weekday().String())
	target := strings.ToLower(stringValue(v, "weekday"))
	hit := day == target
	if list, ok := v["weekdays"].([]any); ok {
		hit = false
		for _, x := range list {
			if strings.ToLower(fmt.Sprint(x)) == day {
				hit = true
			}
		}
	}
	if op == "not_equals" || op == "not_in" {
		return !hit
	}
	return hit
}
func compareDate(t time.Time, v map[string]any, op string) bool {
	d := t.Format("2006-01-02")
	start, end := stringValue(v, "start"), stringValue(v, "end")
	switch op {
	case "between":
		return d >= start && d <= end
	case "after":
		return d > start
	case "before":
		return d < start
	}
	return false
}

func Summary(r Rule) string {
	parts := []string{}
	walkConditions(r.Expression, func(c Condition) { parts = append(parts, conditionSummary(c)) })
	actions := []string{}
	for _, a := range r.Actions {
		if a.Type == ActionEvent {
			actions = append(actions, "trigger an event")
		} else {
			actions = append(actions, "send an email")
		}
	}
	return "When " + strings.Join(parts, " and ") + ", then " + strings.Join(actions, " and ") + "."
}
func conditionSummary(c Condition) string {
	prefix := ""
	if c.Negated {
		prefix = "NOT "
	}
	v, _ := rawMap(c.Value)
	switch c.Type {
	case ConditionEvent:
		return prefix + "event “" + stringValue(v, "event_key") + "” for avaliado"
	case ConditionAlert:
		return prefix + "the selected alert is triggered"
	case ConditionSenderStatus:
		status := map[string]string{"online": "active", "inactive": "inactive"}[stringValue(v, "status")]
		return prefix + "the sender becomes " + status
	case ConditionLogReceived:
		return prefix + "qualquer log for recebido"
	case ConditionMessage:
		return prefix + "the message " + strings.ReplaceAll(c.Operator, "_", " ") + " “" + stringValue(v, "text") + "”"
	case ConditionSeverity:
		return prefix + "a severity " + c.Operator
	case ConditionMetadata:
		return prefix + "metadata." + stringValue(v, "path") + " " + c.Operator
	case ConditionTime:
		return prefix + "the time " + c.Operator
	case ConditionWeekday:
		return prefix + "the weekday " + c.Operator
	case ConditionDate:
		return prefix + "a data " + c.Operator
	}
	return prefix + "condition"
}

func (s *Service) Test(ctx context.Context, rule Rule, input TestInput) (EvaluationResult, error) {
	sender, err := s.deps.Sender(ctx, input.SenderID)
	if err != nil {
		return EvaluationResult{}, &ValidationError{"sender_id", "Sender not found."}
	}
	if !contains(rule.SenderIDs, sender.ID) {
		return EvaluationResult{}, &ValidationError{"sender_id", "The sender is outside the rule scope."}
	}
	t := input.Trigger.Timestamp
	if t.IsZero() {
		t = s.clock.Now()
	}
	entry := domain.LogEntry{SenderID: sender.ID, Timestamp: t, Severity: input.Trigger.Severity, Message: input.Trigger.Message, Event: input.Trigger.EventKey, Metadata: input.Trigger.Metadata}
	result := s.Evaluate(rule, sender, entry, input.Trigger.AlertID, false)
	if input.ExecuteActions && result.Matched && !result.Pending {
		if err = s.execute(ctx, rule, sender, entry, newID("corr_"), 0); err != nil {
			return result, err
		}
	}
	slog.Info("monitoring rule tested", "rule_id", rule.ID, "matched", result.Matched)
	return result, nil
}
func (s *Service) Notify(ctx context.Context, sender domain.Sender, entry domain.LogEntry) {
	s.notify(ctx, sender, entry, "", newID("corr_"), 0)
}
func (s *Service) NotifySenderStatus(ctx context.Context, sender domain.Sender, previous domain.SenderStatus) {
	entry := domain.LogEntry{SenderID: sender.ID, Timestamp: s.clock.Now(), Severity: domain.Info, Metadata: map[string]any{"sender_status": string(sender.Status), "previous_sender_status": string(previous)}}
	s.notify(ctx, sender, entry, "", newID("corr_"), 0)
}
func (s *Service) NotifyAlert(ctx context.Context, sender domain.Sender, entry domain.LogEntry, alertID string) {
	s.notify(ctx, sender, entry, alertID, newID("corr_"), 0)
}
func (s *Service) notify(ctx context.Context, sender domain.Sender, entry domain.LogEntry, alertID, correlation string, depth int) {
	s.cancelPending(sender.ID, entry.Event)
	s.mu.RLock()
	ids := append([]string(nil), s.bySender[sender.ID]...)
	s.mu.RUnlock()
	for _, id := range ids {
		r, err := s.Get(id)
		if err != nil || !r.Enabled || r.Status == "draft" {
			continue
		}
		started := s.clock.Now()
		executionID := ""
		if s.executions != nil {
			severity := entry.Severity
			actions := make([]executions.ActionResult, 0, len(r.Actions))
			for _, action := range r.Actions {
				actions = append(actions, executions.ActionResult{ID: action.ID, Type: string(action.Type), Status: executions.StatusPending})
			}
			record, createErr := s.executions.Create(executions.Record{SourceType: executions.SourceMonitoring, SourceID: r.ID, SourceName: r.Name, SenderID: sender.ID, SenderName: sender.Name, TriggerType: "log", TriggerID: entry.EventOccurrenceID, TriggerName: entry.Event, TriggerMessage: entry.Message, Severity: &severity, Status: executions.StatusPending, CorrelationID: correlation, CausationID: entry.EventOccurrenceID, Actions: actions})
			if createErr == nil {
				executionID = record.ID
				_, _ = s.executions.Update(executionID, func(v *executions.Record) { v.Status = executions.StatusProcessing })
			}
		}
		result := s.Evaluate(r, sender, entry, alertID, false)
		now := s.clock.Now()
		r.LastEvaluatedAt = &now
		r.LastResult = "not_matched"
		if result.Pending {
			r.LastResult = "pending"
			minutes := pendingMinutes(r.Expression)
			p := PendingEvaluation{ID: newID("pend_"), ExecutionID: executionID, RuleID: r.ID, SenderID: sender.ID, TriggeredAt: now, DueAt: now.Add(time.Duration(minutes) * time.Minute), Status: "pending", Trigger: Trigger{Type: "log", AlertID: alertID, EventKey: entry.Event, Severity: entry.Severity, Message: entry.Message, Timestamp: entry.Timestamp, Metadata: entry.Metadata}, CorrelationID: correlation}
			_ = s.store.PutPending(p)
			if s.executions != nil && executionID != "" {
				_, _ = s.executions.Update(executionID, func(v *executions.Record) {
					v.Status = executions.StatusPending
					v.Conditions = executionConditions(result.Conditions)
				})
			}
		} else if result.Matched {
			r.LastResult = "success"
			r.LastExecutedAt = &now
			r.ExecutionCount++
			if err = s.execute(ctx, r, sender, entry, correlation, depth); err != nil {
				r.LastResult = "failed"
				r.LastError = sanitize(err.Error())
				r.FailureCount++
			}
		}
		r.UpdatedAt = r.UpdatedAt
		_ = s.store.Put(r)
		status := r.LastResult
		if s.executions != nil && executionID != "" && !result.Pending {
			_, _ = s.executions.Update(executionID, func(v *executions.Record) {
				v.Conditions = executionConditions(result.Conditions)
				switch status {
				case "success":
					v.Status = executions.StatusSuccess
				case "failed":
					v.Status = executions.StatusFailed
					message := r.LastError
					v.ErrorMessage = &message
				default:
					v.Status = executions.StatusSkipped
				}
				for i := range v.Actions {
					v.Actions[i].Status = v.Status
					v.Actions[i].AttemptCount = 1
				}
			})
		}
		_ = s.store.AppendExecution(Execution{ID: newID("exec_"), RuleID: r.ID, SenderID: sender.ID, TriggerType: "log", TriggerID: entry.Event, CorrelationID: correlation, StartedAt: started, FinishedAt: s.clock.Now(), Status: status, Result: result, Error: r.LastError})
	}
}

func (s *Service) cancelPending(senderID, eventKey string) {
	if eventKey == "" {
		return
	}
	for _, pending := range s.store.Pending() {
		if pending.Status != "pending" || pending.SenderID != senderID {
			continue
		}
		rule, err := s.Get(pending.RuleID)
		if err != nil {
			continue
		}
		cancels := false
		walkConditions(rule.Expression, func(condition Condition) {
			if condition.Type != ConditionEvent || !strings.Contains(condition.Operator, "not_") {
				return
			}
			value, _ := rawMap(condition.Value)
			if stringValue(value, "event_key") == eventKey {
				cancels = true
			}
		})
		if cancels {
			if s.executions != nil && pending.ExecutionID != "" {
				_, _ = s.executions.Update(pending.ExecutionID, func(v *executions.Record) { v.Status = executions.StatusCancelled })
			}
			_ = s.store.DeletePending(pending.ID)
			slog.Info("pending monitoring evaluation canceled", "pending_id", pending.ID, "rule_id", pending.RuleID)
		}
	}
}
func pendingMinutes(g ExpressionGroup) int {
	minutes := 1
	walkConditions(g, func(c Condition) {
		if c.Type == ConditionEvent && (strings.Contains(c.Operator, "not_")) {
			v, _ := rawMap(c.Value)
			if n := int(numberValue(v, "window_minutes")); n > minutes && n <= 1440 {
				minutes = n
			}
		}
	})
	return minutes
}
func sanitize(v string) string {
	v = strings.ReplaceAll(strings.ReplaceAll(v, "\r", " "), "\n", " ")
	if len(v) > 300 {
		return v[:300]
	}
	return v
}
func (s *Service) execute(ctx context.Context, r Rule, sender domain.Sender, entry domain.LogEntry, correlation string, depth int) error {
	if depth >= 10 {
		return errors.New("depth limit reached")
	}
	if s.executor == nil {
		return errors.New("action executor unavailable")
	}
	var errs []string
	for _, a := range r.Actions {
		if err := s.executor.ExecuteMonitoringAction(ctx, r, a, sender, entry, correlation, depth+1); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
func (s *Service) ProcessPending(ctx context.Context) {
	now := s.clock.Now()
	for _, p := range s.store.Pending() {
		if p.Status != "pending" || p.DueAt.After(now) {
			continue
		}
		r, err := s.Get(p.RuleID)
		if err != nil || !r.Enabled {
			_ = s.store.DeletePending(p.ID)
			continue
		}
		sender, err := s.deps.Sender(ctx, p.SenderID)
		if err != nil {
			continue
		}
		entry := domain.LogEntry{SenderID: p.SenderID, Timestamp: p.Trigger.Timestamp, Severity: p.Trigger.Severity, Message: p.Trigger.Message, Event: p.Trigger.EventKey, Metadata: p.Trigger.Metadata}
		result := s.Evaluate(r, sender, entry, p.Trigger.AlertID, true)
		if result.Matched {
			_ = s.execute(ctx, r, sender, entry, p.CorrelationID, 0)
			at := now
			r.LastExecutedAt = &at
			r.ExecutionCount++
			r.LastResult = "success"
			_ = s.store.Put(r)
		}
		_ = s.store.AppendExecution(Execution{ID: newID("exec_"), RuleID: r.ID, SenderID: sender.ID, TriggerType: "pending", CorrelationID: p.CorrelationID, StartedAt: now, FinishedAt: s.clock.Now(), Status: map[bool]string{true: "success", false: "not_matched"}[result.Matched], Result: result})
		if s.executions != nil && p.ExecutionID != "" {
			_, _ = s.executions.Update(p.ExecutionID, func(v *executions.Record) {
				v.Conditions = executionConditions(result.Conditions)
				if result.Matched {
					v.Status = executions.StatusSuccess
				} else {
					v.Status = executions.StatusSkipped
				}
				for i := range v.Actions {
					v.Actions[i].Status = v.Status
					v.Actions[i].AttemptCount = 1
				}
			})
		}
		_ = s.store.DeletePending(p.ID)
	}
}

func executionConditions(items []ConditionResult) []executions.ConditionResult {
	out := make([]executions.ConditionResult, 0, len(items))
	for _, item := range items {
		out = append(out, executions.ConditionResult{ID: item.ID, Matched: item.Matched, Description: item.Description, Error: item.Error})
	}
	return out
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	s.ProcessPending(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ProcessPending(ctx)
		}
	}
}
func (s *Service) Executions(ruleID string, limit int) ([]Execution, error) {
	return s.store.Executions(ruleID, limit)
}
func (s *Service) Execution(id string) (Execution, error) {
	v, ok, err := s.store.Execution(id)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (s *Service) SenderUsageCount(id string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bySender[id])
}
func (s *Service) EventUsageCount(id string) int {
	count := 0
	for _, rule := range s.store.All() {
		used := false
		for _, action := range rule.Actions {
			if action.Type == ActionEvent {
				value, _ := rawMap(action.Config)
				if stringValue(value, "event_id") == id {
					used = true
				}
			}
		}
		walkConditions(rule.Expression, func(condition Condition) {
			if condition.Type != ConditionEvent {
				return
			}
			value, _ := rawMap(condition.Value)
			event, err := s.deps.Event(id)
			if err == nil && stringValue(value, "event_key") == event.Key {
				used = true
			}
		})
		if used {
			count++
		}
	}
	return count
}
func (s *Service) AlertUsageCount(id string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byAlert[id])
}
func (s *Service) RemoveSender(id string) error {
	for _, r := range s.store.All() {
		if !contains(r.SenderIDs, id) {
			continue
		}
		ids := []string{}
		for _, x := range r.SenderIDs {
			if x != id {
				ids = append(ids, x)
			}
		}
		r.SenderIDs = ids
		if len(ids) == 0 && !r.IncludeNewSenders {
			r.Enabled = false
		}
		r.UpdatedAt = s.clock.Now()
		if err := s.store.Put(r); err != nil {
			return err
		}
	}
	s.rebuild()
	return nil
}
func (s *Service) NotifySenderCreated(_ context.Context, sender domain.Sender) {
	changed := false
	for _, r := range s.store.All() {
		if !r.IncludeNewSenders || contains(r.SenderIDs, sender.ID) {
			continue
		}
		r.SenderIDs = append(r.SenderIDs, sender.ID)
		r.UpdatedAt = s.clock.Now()
		if err := s.store.Put(r); err == nil {
			changed = true
		} else {
			slog.Error("failed to associate new sender with monitoring rule", "rule_id", r.ID, "sender_id", sender.ID, "error", err)
		}
	}
	if changed {
		s.rebuild()
	}
}
func (s *Service) rebuild() {
	bySender := map[string][]string{}
	byEvent := map[string][]string{}
	byAlert := map[string][]string{}
	for _, r := range s.store.All() {
		for _, id := range r.SenderIDs {
			bySender[id] = append(bySender[id], r.ID)
		}
		for _, action := range r.Actions {
			if action.Type != ActionEvent {
				continue
			}
			value, _ := rawMap(action.Config)
			if eventID := stringValue(value, "event_id"); eventID != "" {
				byEvent[eventID] = append(byEvent[eventID], r.ID)
			}
		}
		walkConditions(r.Expression, func(c Condition) {
			v, _ := rawMap(c.Value)
			if c.Type == ConditionEvent {
				if key := stringValue(v, "event_key"); key != "" {
					byEvent[key] = append(byEvent[key], r.ID)
				}
			}
			if c.Type == ConditionAlert {
				if alertID := stringValue(v, "alert_id"); alertID != "" {
					byAlert[alertID] = append(byAlert[alertID], r.ID)
				}
			}
		})
	}
	for _, group := range []map[string][]string{bySender, byEvent, byAlert} {
		for _, v := range group {
			sort.Strings(v)
		}
	}
	s.mu.Lock()
	s.bySender, s.byEvent, s.byAlert = bySender, byEvent, byAlert
	s.mu.Unlock()
}
