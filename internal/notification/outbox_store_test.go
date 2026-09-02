package notification

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOutboxPersistsClaimsAndRecoversExpiredLeases(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	store, err := OpenOutbox(dir, 2, clock)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Enqueue(context.Background(), testNotification())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenOutbox(dir, 2, clock)
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := reopened.Claim(context.Background(), "worker-a", time.Minute)
	if err != nil || !ok || claimed.ID != created.ID || claimed.AttemptCount != 1 {
		t.Fatalf("unexpected first claim: job=%+v ok=%v err=%v", claimed, ok, err)
	}
	if _, ok, err = store.Claim(context.Background(), "worker-b", time.Minute); err != nil || ok {
		t.Fatalf("active lease should not be claimed: ok=%v err=%v", ok, err)
	}
	now = now.Add(2 * time.Minute)
	claimed, ok, err = store.Claim(context.Background(), "worker-b", time.Minute)
	if err != nil || !ok || claimed.AttemptCount != 2 {
		t.Fatalf("expired lease was not recovered: job=%+v ok=%v err=%v", claimed, ok, err)
	}
	if err = store.Complete(context.Background(), claimed.ID, "worker-a"); err == nil {
		t.Fatal("worker without the lease completed the job")
	}
	if err = store.Complete(context.Background(), claimed.ID, "worker-b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = store.Claim(context.Background(), "worker-c", time.Minute); err != nil || ok {
		t.Fatalf("completed job remained in queue: ok=%v err=%v", ok, err)
	}
}

func TestOutboxCapacityAndRetrySchedule(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store, err := OpenOutbox(dir, 1, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Enqueue(context.Background(), testNotification()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Enqueue(context.Background(), testNotification()); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected full outbox, got %v", err)
	}
	job, ok, err := store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err = store.Retry(context.Background(), job.ID, "worker", "temporary", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = store.Claim(context.Background(), "worker", time.Minute); err != nil || ok {
		t.Fatalf("retry became available too early: ok=%v err=%v", ok, err)
	}
	now = now.Add(time.Minute)
	job, ok, err = store.Claim(context.Background(), "worker", time.Minute)
	if err != nil || !ok || job.LastError != "temporary" {
		t.Fatalf("scheduled retry unavailable: job=%+v ok=%v err=%v", job, ok, err)
	}
}
