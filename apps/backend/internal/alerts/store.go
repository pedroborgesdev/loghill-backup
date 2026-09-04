package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"logmate/internal/domain"
)

type persistedAlerts struct {
	Version int                 `json:"version"`
	Items   []domain.EmailAlert `json:"items"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]domain.EmailAlert
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}
	store := &Store{path: filepath.Join(dataDir, "alerts.json"), items: make(map[string]domain.EmailAlert)}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		if err = store.writeAtomic(nil); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var persisted persistedAlerts
	if err = json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode alerts: %w", err)
	}
	if persisted.Version != 1 {
		return nil, fmt.Errorf("unsupported alerts file version %d", persisted.Version)
	}
	for _, alert := range persisted.Items {
		if alert.ID == "" {
			return nil, errors.New("stored alert has no id")
		}
		if _, exists := store.items[alert.ID]; exists {
			return nil, fmt.Errorf("duplicate stored alert %q", alert.ID)
		}
		store.items[alert.ID] = cloneAlert(alert)
	}
	return store, nil
}

func (s *Store) All() []domain.EmailAlert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]domain.EmailAlert, 0, len(s.items))
	for _, alert := range s.items {
		items = append(items, cloneAlert(alert))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) Get(id string) (domain.EmailAlert, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alert, ok := s.items[id]
	return cloneAlert(alert), ok
}

func (s *Store) Put(alert domain.EmailAlert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneMap(s.items)
	next[alert.ID] = cloneAlert(alert)
	if err := s.writeAtomic(mapValues(next)); err != nil {
		return err
	}
	s.items = next
	return nil
}

func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[id]; !exists {
		return false, nil
	}
	next := cloneMap(s.items)
	delete(next, id)
	if err := s.writeAtomic(mapValues(next)); err != nil {
		return false, err
	}
	s.items = next
	return true, nil
}

func (s *Store) Mutate(id string, change func(*domain.EmailAlert)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	alert, exists := s.items[id]
	if !exists {
		return ErrNotFound
	}
	change(&alert)
	next := cloneMap(s.items)
	next[id] = cloneAlert(alert)
	if err := s.writeAtomic(mapValues(next)); err != nil {
		return err
	}
	s.items = next
	return nil
}

func (s *Store) RemoveSender(senderID string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneMap(s.items)
	affected := 0
	for id, alert := range next {
		ids := make([]string, 0, len(alert.SenderIDs))
		names := make([]string, 0, len(alert.SenderNames))
		removed := false
		for index, currentID := range alert.SenderIDs {
			if currentID == senderID {
				removed = true
				continue
			}
			ids = append(ids, currentID)
			if index < len(alert.SenderNames) {
				names = append(names, alert.SenderNames[index])
			}
		}
		if !removed {
			continue
		}
		affected++
		alert.SenderIDs = ids
		alert.SenderNames = names
		alert.UpdatedAt = now
		if len(ids) == 0 {
			alert.Enabled = false
		}
		next[id] = cloneAlert(alert)
	}
	if affected == 0 {
		return 0, nil
	}
	if err := s.writeAtomic(mapValues(next)); err != nil {
		return 0, err
	}
	s.items = next
	return affected, nil
}

func (s *Store) RenameSender(senderID, name string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneMap(s.items)
	changed := false
	for id, alert := range next {
		for index, currentID := range alert.SenderIDs {
			if currentID != senderID {
				continue
			}
			for len(alert.SenderNames) <= index {
				alert.SenderNames = append(alert.SenderNames, "")
			}
			alert.SenderNames[index] = name
			alert.UpdatedAt = now
			next[id] = cloneAlert(alert)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := s.writeAtomic(mapValues(next)); err != nil {
		return err
	}
	s.items = next
	return nil
}

func (s *Store) writeAtomic(items []domain.EmailAlert) error {
	if items == nil {
		items = []domain.EmailAlert{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	data, err := json.MarshalIndent(persistedAlerts{Version: 1, Items: items}, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
	}
	return err
}

func cloneAlert(alert domain.EmailAlert) domain.EmailAlert {
	alert.SenderIDs = append([]string(nil), alert.SenderIDs...)
	alert.SenderNames = append([]string(nil), alert.SenderNames...)
	alert.Severities = append([]domain.LogSeverity(nil), alert.Severities...)
	alert.Recipients = append([]string(nil), alert.Recipients...)
	return alert
}

func cloneMap(source map[string]domain.EmailAlert) map[string]domain.EmailAlert {
	result := make(map[string]domain.EmailAlert, len(source))
	for id, alert := range source {
		result[id] = cloneAlert(alert)
	}
	return result
}

func mapValues(source map[string]domain.EmailAlert) []domain.EmailAlert {
	result := make([]domain.EmailAlert, 0, len(source))
	for _, alert := range source {
		result = append(result, cloneAlert(alert))
	}
	return result
}

func deliveryPointer(value domain.DeliveryStatus) *domain.DeliveryStatus { return &value }
func stringPointer(value string) *string                                 { return &value }
func timePointer(value time.Time) *time.Time                             { return &value }
