package settings

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"logtheater/internal/domain"
)

func TestOpenCreatesAndReloadsDefaults(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	store, err := Open(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); got.LogLimit.Value != 10_000 || got.InactivePreservation.Value != 2_000 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if _, err = os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("settings were not persisted: %v", err)
	}
	reloaded, err := Open(dir, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Get().UpdatedAt != now {
		t.Fatalf("valid settings were overwritten: %+v", reloaded.Get())
	}
}

func TestUpdatePersistsAtomicallyAndIsConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	store, err := Open(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	value := domain.Settings{
		LogLimit:             domain.NumberUnitValue{Value: 8, Unit: domain.StorageLines},
		InactivePreservation: domain.NumberUnitValue{Value: 1, Unit: domain.StorageMB},
		InactiveAfterSeconds: 300,
		DeleteInactiveDays:   7,
	}
	var wait sync.WaitGroup
	for index := 0; index < 20; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_ = store.Get()
		}()
	}
	updated, err := store.Update(value, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if updated.LogLimit.Value != 8 {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if _, err = os.Stat(filepath.Join(dir, "config.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary file remained after rename: %v", err)
	}
	reloaded, err := Open(dir, now)
	if err != nil || reloaded.Get().LogLimit.Value != 8 {
		t.Fatalf("update was not reloaded: %+v, %v", reloaded, err)
	}
}

func TestOpenRejectsCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, time.Now()); err == nil {
		t.Fatal("expected corrupted settings to fail")
	}
}

func TestOpenPreservesLegacyValueAboveCurrentLimit(t *testing.T) {
	dir := t.TempDir()
	legacy := []byte(`{"log_limit":{"value":100000,"unit":"lines"},"inactive_preservation":{"value":2000,"unit":"lines"},"updated_at":"2026-07-30T10:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if store.Get().LogLimit.Value != 100_000 {
		t.Fatalf("legacy value was changed: %+v", store.Get())
	}
}

func TestValidationRules(t *testing.T) {
	valid := domain.DefaultSettings(time.Now())
	cases := []struct {
		name   string
		change func(*domain.Settings)
		valid  bool
	}{
		{name: "minimum", change: func(v *domain.Settings) { v.LogLimit.Value = 0 }, valid: true},
		{name: "maximum", change: func(v *domain.Settings) { v.LogLimit.Value = 10_000 }, valid: true},
		{name: "negative", change: func(v *domain.Settings) { v.LogLimit.Value = -1 }},
		{name: "above maximum", change: func(v *domain.Settings) { v.LogLimit.Value = 10_001 }},
		{name: "invalid unit", change: func(v *domain.Settings) { v.LogLimit.Unit = "gb" }},
		{name: "preservation zero", change: func(v *domain.Settings) { v.InactivePreservation.Value = 0 }, valid: true},
		{name: "preservation over limit", change: func(v *domain.Settings) { v.LogLimit.Value = 10; v.InactivePreservation.Value = 11 }},
		{name: "different units", change: func(v *domain.Settings) {
			v.LogLimit.Value = 1
			v.InactivePreservation.Value = 10_000
			v.InactivePreservation.Unit = domain.StorageMB
		}, valid: true},
		{name: "unlimited maximum", change: func(v *domain.Settings) { v.LogLimit.Value = 0; v.InactivePreservation.Value = 10_000 }, valid: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.change(&candidate)
			err := domain.ValidateSettings(candidate)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, err=%v", test.valid, err)
			}
		})
	}
}
