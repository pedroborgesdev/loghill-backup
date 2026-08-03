package executions

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"logtheater/internal/domain"
)

type Store struct {
	mu      sync.RWMutex
	path    string
	records map[string]Record
	now     func() time.Time
}

func Open(dataDir string, now func() time.Time) (*Store, error) {
	dir := filepath.Join(dataDir, "executions")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "executions.jsonl"), records: map[string]Record{}, now: now}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scan.Scan() {
		var record Record
		if json.Unmarshal(scan.Bytes(), &record) == nil && record.ID != "" {
			s.records[record.ID] = record
		}
	}
	return scan.Err()
}

func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return prefix + time.Now().Format("20060102150405.000000000")
	}
	return prefix + hex.EncodeToString(b)
}

func sanitize(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "\r", " "), "\n", " ")
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func (s *Store) appendLocked(record Record) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func (s *Store) Create(record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if record.ID == "" {
		record.ID = NewID("exec_")
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = now
	}
	if record.Status == "" {
		record.Status = StatusPending
	}
	if record.AttemptCount < 1 {
		record.AttemptCount = 1
	}
	if record.Actions == nil {
		record.Actions = []ActionResult{}
	}
	record.TriggerMessage = sanitize(record.TriggerMessage)
	record.UpdatedAt = now
	if err := s.appendLocked(record); err != nil {
		return Record{}, err
	}
	s.records[record.ID] = record
	return record, nil
}

func (s *Store) Update(id string, change func(*Record)) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return Record{}, os.ErrNotExist
	}
	change(&record)
	record.ErrorMessage = sanitizePointer(record.ErrorMessage)
	for i := range record.Actions {
		record.Actions[i].ErrorMessage = sanitizePointer(record.Actions[i].ErrorMessage)
	}
	record.UpdatedAt = s.now()
	if terminal(record.Status) && record.FinishedAt == nil {
		finished := record.UpdatedAt
		record.FinishedAt = &finished
	}
	if record.FinishedAt != nil {
		duration := record.FinishedAt.Sub(record.StartedAt).Milliseconds()
		if duration < 0 {
			duration = 0
		}
		record.DurationMs = &duration
	}
	if err := s.appendLocked(record); err != nil {
		return Record{}, err
	}
	s.records[id] = record
	return record, nil
}

func sanitizePointer(value *string) *string {
	if value == nil {
		return nil
	}
	safe := sanitize(*value)
	return &safe
}
func terminal(status Status) bool {
	return status == StatusSuccess || status == StatusPartial || status == StatusFailed || status == StatusCancelled || status == StatusSkipped
}

func (s *Store) Get(id string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.records[id]
	return v, ok
}

func (s *Store) List(filters Filters) Page {
	s.mu.RLock()
	items := make([]Record, 0, len(s.records))
	for _, v := range s.records {
		items = append(items, v)
	}
	s.mu.RUnlock()
	now := s.now()
	filtered := items[:0]
	for _, v := range items {
		if v.Status == StatusSkipped {
			continue
		}
		if filters.SourceType != "" && v.SourceType != filters.SourceType || filters.SourceID != "" && v.SourceID != filters.SourceID || filters.SenderID != "" && v.SenderID != filters.SenderID || filters.TriggerType != "" && v.TriggerType != filters.TriggerType {
			continue
		}
		if len(filters.Statuses) > 0 && !filters.Statuses[v.Status] {
			continue
		}
		if len(filters.Severities) > 0 && (v.Severity == nil || !filters.Severities[*v.Severity]) {
			continue
		}
		if filters.StartedFrom != nil && v.StartedAt.Before(*filters.StartedFrom) || filters.StartedTo != nil && v.StartedAt.After(*filters.StartedTo) || filters.Recent && v.StartedAt.Before(now.Add(-time.Hour)) {
			continue
		}
		if filters.ActionType != "" {
			found := false
			for _, a := range v.Actions {
				if a.Type == filters.ActionType {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		q := strings.ToLower(strings.TrimSpace(filters.Search))
		if q != "" && !strings.Contains(strings.ToLower(v.SourceName+" "+v.SenderName+" "+v.TriggerName+" "+v.TriggerMessage), q) {
			continue
		}
		filtered = append(filtered, v)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filters.Order == "asc" {
			return filtered[i].StartedAt.Before(filtered[j].StartedAt)
		}
		return filtered[i].StartedAt.After(filtered[j].StartedAt)
	})
	page, pageSize := filters.Page, filters.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pages := (total + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	return Page{Items: filtered[start:end], Pagination: domain.Pagination{Page: page, PageSize: pageSize, Returned: end - start, Total: int64(total), TotalPages: pages}}
}
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Store) Summary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now()
	hour, day := now.Add(-time.Hour), now.Add(-24*time.Hour)
	var out Summary
	for _, v := range s.records {
		if v.Status == StatusSkipped {
			continue
		}
		if !v.StartedAt.Before(hour) {
			out.LastHour++
			if v.Status == StatusFailed {
				out.FailedLastHour++
			}
		}
		if v.Status == StatusPending || v.Status == StatusProcessing {
			out.Running++
		}
		if !v.StartedAt.Before(day) {
			out.Last24Hours++
			switch v.SourceType {
			case SourceAlert:
				out.AlertsLast24Hours++
			case SourceEvent:
				out.EventsLast24Hours++
			case SourceMonitoring:
				out.MonitoringLast24Hours++
			}
		}
	}
	return out
}

func (s *Store) Cleanup(retention time.Duration, maxRecords int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-retention)
	items := make([]Record, 0, len(s.records))
	for _, v := range s.records {
		if v.Status == StatusPending || v.Status == StatusProcessing || !v.StartedAt.Before(cutoff) {
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	if maxRecords > 0 && len(items) > maxRecords {
		running := []Record{}
		finished := []Record{}
		for _, v := range items {
			if v.Status == StatusPending || v.Status == StatusProcessing {
				running = append(running, v)
			} else {
				finished = append(finished, v)
			}
		}
		keep := maxRecords - len(running)
		if keep < 0 {
			keep = 0
		}
		if len(finished) > keep {
			finished = finished[:keep]
		}
		items = append(running, finished...)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.Before(items[j].StartedAt) })
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, v := range items {
		if err = enc.Encode(v); err != nil {
			break
		}
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	next := map[string]Record{}
	for _, v := range items {
		next[v.ID] = v
	}
	s.records = next
	return nil
}
