package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLockHonorsContextAndCanBeReacquired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.lock")
	first, err := AcquireFileLock(context.Background(), path)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if _, err = AcquireFileLock(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline while lock is held, got %v", err)
	}
	if err = first.Release(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}
	second, err := AcquireFileLock(context.Background(), path)
	if err != nil {
		t.Fatalf("reacquire released lock: %v", err)
	}
	if err = second.Release(); err != nil {
		t.Fatalf("release second lock: %v", err)
	}
	if err = second.Release(); err != nil {
		t.Fatalf("second release should be idempotent: %v", err)
	}
}
