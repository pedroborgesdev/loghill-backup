package monitoring

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"logtheater/internal/domain"
)

type persistedRules struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

var everySeverity = []domain.LogSeverity{domain.Trace, domain.Debug, domain.Info, domain.Warn, domain.Error, domain.Fatal}

type persistedPending struct {
	Version int                 `json:"version"`
	Items   []PendingEvaluation `json:"items"`
}

type Store struct {
	mu                                  sync.RWMutex
	rulesPath, pendingPath, historyPath string
	rules                               map[string]Rule
	pending                             map[string]PendingEvaluation
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}
	s := &Store{rulesPath: filepath.Join(dataDir, "monitoring-rules.json"), pendingPath: filepath.Join(dataDir, "monitoring-pending.json"), historyPath: filepath.Join(dataDir, "monitoring-executions.jsonl"), rules: map[string]Rule{}, pending: map[string]PendingEvaluation{}}
	if err := s.loadRules(); err != nil {
		return nil, err
	}
	if err := s.loadPending(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadRules() error {
	b, err := os.ReadFile(s.rulesPath)
	if errors.Is(err, os.ErrNotExist) {
		return s.writeRules(nil)
	}
	if err != nil {
		return err
	}
	var p persistedRules
	if err = json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("decode monitoring rules: %w", err)
	}
	if p.Version != 1 {
		return fmt.Errorf("unsupported monitoring rules version %d", p.Version)
	}
	migrated := false
	for _, r := range p.Rules {
		if r.ID == "" {
			return errors.New("stored monitoring rule has no id")
		}
		if r.Status == "" {
			r.Status = "active"
		}
		if migrateLogReceived(&r.Expression) {
			migrated = true
		}
		s.rules[r.ID] = r
	}
	if migrated {
		items := make([]Rule, 0, len(s.rules))
		for _, r := range s.rules {
			items = append(items, r)
		}
		return s.writeRules(items)
	}
	return nil
}

// migrateLogReceived converte o gatilho genérico antigo, gravado como
// "severity in [todas as severities]", no tipo dedicado log_received.
func migrateLogReceived(g *ExpressionGroup) bool {
	changed := false
	for i := range g.Nodes {
		if group := g.Nodes[i].Group; group != nil && migrateLogReceived(group) {
			changed = true
		}
		condition := g.Nodes[i].Condition
		if condition == nil || condition.Type != ConditionSeverity || condition.Operator != "in" {
			continue
		}
		value, err := rawMap(condition.Value)
		if err != nil {
			continue
		}
		if stringValue(value, "source") != "log_received" && !coversEverySeverity(value["severities"]) {
			continue
		}
		condition.Type = ConditionLogReceived
		condition.Operator = "received"
		condition.Value = json.RawMessage("{}")
		changed = true
	}
	return changed
}

func coversEverySeverity(raw any) bool {
	list, ok := raw.([]any)
	if !ok {
		return false
	}
	present := map[string]bool{}
	for _, item := range list {
		present[fmt.Sprint(item)] = true
	}
	for _, severity := range everySeverity {
		if !present[string(severity)] {
			return false
		}
	}
	return true
}
func (s *Store) loadPending() error {
	b, err := os.ReadFile(s.pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return s.writePending(nil)
	}
	if err != nil {
		return err
	}
	var p persistedPending
	if err = json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("decode monitoring pending: %w", err)
	}
	if p.Version != 1 {
		return fmt.Errorf("unsupported monitoring pending version %d", p.Version)
	}
	for _, v := range p.Items {
		if v.ID != "" {
			s.pending[v.ID] = v
		}
	}
	return nil
}
func atomicJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
func (s *Store) writeRules(items []Rule) error {
	if items == nil {
		items = []Rule{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return atomicJSON(s.rulesPath, persistedRules{Version: 1, Rules: items})
}
func (s *Store) writePending(items []PendingEvaluation) error {
	if items == nil {
		items = []PendingEvaluation{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DueAt.Before(items[j].DueAt) })
	return atomicJSON(s.pendingPath, persistedPending{Version: 1, Items: items})
}
func ruleValues(m map[string]Rule) []Rule {
	out := make([]Rule, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
func pendingValues(m map[string]PendingEvaluation) []PendingEvaluation {
	out := make([]PendingEvaluation, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
func (s *Store) All() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := ruleValues(s.rules)
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
func (s *Store) Get(id string) (Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.rules[id]
	return v, ok
}
func (s *Store) Put(v Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]Rule, len(s.rules)+1)
	for k, x := range s.rules {
		next[k] = x
	}
	next[v.ID] = v
	if err := s.writeRules(ruleValues(next)); err != nil {
		return err
	}
	s.rules = next
	return nil
}
func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rules[id]; !ok {
		return false, nil
	}
	next := map[string]Rule{}
	for k, v := range s.rules {
		if k != id {
			next[k] = v
		}
	}
	if err := s.writeRules(ruleValues(next)); err != nil {
		return false, err
	}
	s.rules = next
	return true, nil
}
func (s *Store) Pending() []PendingEvaluation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return pendingValues(s.pending)
}
func (s *Store) PutPending(v PendingEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := map[string]PendingEvaluation{}
	for k, x := range s.pending {
		next[k] = x
	}
	next[v.ID] = v
	if err := s.writePending(pendingValues(next)); err != nil {
		return err
	}
	s.pending = next
	return nil
}
func (s *Store) DeletePending(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := map[string]PendingEvaluation{}
	for k, v := range s.pending {
		if k != id {
			next[k] = v
		}
	}
	if err := s.writePending(pendingValues(next)); err != nil {
		return err
	}
	s.pending = next
	return nil
}
func (s *Store) AppendExecution(v Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.historyPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	return err
}
func (s *Store) Executions(ruleID string, limit int) ([]Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(s.historyPath)
	if errors.Is(err, os.ErrNotExist) {
		return []Execution{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Execution{}
	scan := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scan.Buffer(buf, 1024*1024)
	for scan.Scan() {
		var v Execution
		if json.Unmarshal(scan.Bytes(), &v) == nil && (ruleID == "" || v.RuleID == ruleID) {
			out = append(out, v)
		}
	}
	if err = scan.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (s *Store) Execution(id string) (Execution, bool, error) {
	items, err := s.Executions("", 0)
	if err != nil {
		return Execution{}, false, err
	}
	for _, v := range items {
		if v.ID == id {
			return v, true, nil
		}
	}
	return Execution{}, false, nil
}
