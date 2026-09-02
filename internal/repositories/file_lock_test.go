package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"logtheater/internal/domain"
)

func TestRepositoriesSharingDataDirSerializeSenderMutations(t *testing.T) {
	root := t.TempDir()
	first, second := New(root), New(root)
	if err := first.Init(); err != nil {
		t.Fatalf("init first repository: %v", err)
	}
	if err := second.Init(); err != nil {
		t.Fatalf("init second repository: %v", err)
	}
	lockedCtx, release, err := first.LockSender(context.Background(), "worker")
	if err != nil {
		t.Fatalf("lock sender: %v", err)
	}
	sender := domain.Sender{ID: "worker", Name: "Worker", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err = first.Create(lockedCtx, sender); err != nil {
		release()
		t.Fatalf("create while holding reentrant lock: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if _, err = second.Get(waitCtx, sender.ID); !errors.Is(err, context.DeadlineExceeded) {
		release()
		t.Fatalf("expected second repository to wait for lock, got %v", err)
	}
	release()
	if _, err = second.Get(context.Background(), sender.ID); err != nil {
		t.Fatalf("read after release: %v", err)
	}
}
