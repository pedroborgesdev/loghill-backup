package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"logtheater/internal/domain"
)

type persistedEvents struct {
	Version int                      `json:"version"`
	Events  []domain.EventDefinition `json:"events"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]domain.EventDefinition
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}
	store := &Store{path: filepath.Join(dataDir, "events.json"), items: make(map[string]domain.EventDefinition)}
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
	var persisted persistedEvents
	if err = json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}
	if persisted.Version != 1 {
		return nil, fmt.Errorf("unsupported events file version %d", persisted.Version)
	}
	keys := make(map[string]bool)
	for _, event := range persisted.Events {
		if event.ID == "" || !ValidKey(event.Key) {
			return nil, fmt.Errorf("stored event %q is invalid", event.ID)
		}
		if _, exists := store.items[event.ID]; exists {
			return nil, fmt.Errorf("duplicate stored event id %q", event.ID)
		}
		if keys[event.Key] {
			return nil, fmt.Errorf("duplicate stored event key %q", event.Key)
		}
		keys[event.Key] = true
		store.items[event.ID] = cloneEvent(event)
	}
	return store, nil
}

func (s *Store) All() []domain.EventDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := mapValues(s.items)
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) Get(id string) (domain.EventDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	event, ok := s.items[id]
	return cloneEvent(event), ok
}

func (s *Store) Put(event domain.EventDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneMap(s.items)
	next[event.ID] = cloneEvent(event)
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

func (s *Store) Mutate(id string, change func(*domain.EventDefinition)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	event, exists := s.items[id]
	if !exists {
		return ErrNotFound
	}
	change(&event)
	next := cloneMap(s.items)
	next[id] = cloneEvent(event)
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
	for id, event := range next {
		remaining := make([]string, 0, len(event.SenderIDs))
		removed := false
		for _, current := range event.SenderIDs {
			if current == senderID {
				removed = true
				continue
			}
			remaining = append(remaining, current)
		}
		if !removed {
			continue
		}
		affected++
		event.SenderIDs = remaining
		event.UpdatedAt = now
		if len(remaining) == 0 {
			event.Enabled = false
		}
		next[id] = cloneEvent(event)
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

func (s *Store) writeAtomic(items []domain.EventDefinition) error {
	if items == nil {
		items = []domain.EventDefinition{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	data, err := json.MarshalIndent(persistedEvents{Version: 1, Events: items}, "", "  ")
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

func cloneEvent(event domain.EventDefinition) domain.EventDefinition {
	event.SenderIDs = append([]string{}, event.SenderIDs...)
	event.Recipients = append([]string{}, event.Recipients...)
	return event
}

func cloneMap(source map[string]domain.EventDefinition) map[string]domain.EventDefinition {
	result := make(map[string]domain.EventDefinition, len(source))
	for id, event := range source {
		result[id] = cloneEvent(event)
	}
	return result
}

func mapValues(source map[string]domain.EventDefinition) []domain.EventDefinition {
	result := make([]domain.EventDefinition, 0, len(source))
	for _, event := range source {
		result = append(result, cloneEvent(event))
	}
	return result
}
