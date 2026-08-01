package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"logtheater/internal/domain"
)

type Store struct {
	mu      sync.RWMutex
	path    string
	current domain.Settings
}

func Open(dataDir string, now time.Time) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("initialize settings storage: %w", err)
	}
	store := &Store{path: filepath.Join(dataDir, "config.json")}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		store.current = domain.DefaultSettings(now)
		if err = store.writeAtomic(store.current); err != nil {
			return nil, fmt.Errorf("persist default settings: %w", err)
		}
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	if err = json.Unmarshal(data, &store.current); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}
	if err = domain.ValidateStoredSettings(store.current); err != nil {
		return nil, fmt.Errorf("validate stored settings: %w", err)
	}
	return store, nil
}

func (s *Store) Get() domain.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Update(value domain.Settings, now time.Time) (domain.Settings, error) {
	if err := domain.ValidateSettings(value); err != nil {
		return domain.Settings{}, err
	}
	value.UpdatedAt = now
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeAtomic(value); err != nil {
		return domain.Settings{}, fmt.Errorf("persist settings: %w", err)
	}
	s.current = value
	return value, nil
}

func (s *Store) writeAtomic(value domain.Settings) error {
	data, err := json.MarshalIndent(value, "", "  ")
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
		return err
	}
	return nil
}
