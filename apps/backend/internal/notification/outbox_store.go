package notification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"logmate/internal/domain"
	"logmate/internal/storage"
)

type OutboxStatus string

const (
	OutboxPending OutboxStatus = "pending"
	OutboxLeased  OutboxStatus = "leased"
)

type OutboxJob struct {
	ID             string              `json:"id"`
	Notification   domain.Notification `json:"notification"`
	Status         OutboxStatus        `json:"status"`
	AttemptCount   int                 `json:"attempt_count"`
	AvailableAt    time.Time           `json:"available_at"`
	LeaseOwner     string              `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time          `json:"lease_expires_at,omitempty"`
	LastError      string              `json:"last_error,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type outboxSnapshot struct {
	Version int         `json:"version"`
	Jobs    []OutboxJob `json:"jobs"`
}

type OutboxStore struct {
	path     string
	lockPath string
	capacity int
	now      func() time.Time
}

func OpenOutbox(dataDir string, capacity int, now func() time.Time) (*OutboxStore, error) {
	if capacity < 1 {
		return nil, errors.New("outbox capacity must be positive")
	}
	if now == nil {
		now = time.Now
	}
	dir := filepath.Join(dataDir, "outbox")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	store := &OutboxStore{
		path:     filepath.Join(dir, "notifications.json"),
		lockPath: filepath.Join(dataDir, ".locks", "notification-outbox.lock"),
		capacity: capacity,
		now:      now,
	}
	lock, err := storage.AcquireFileLock(context.Background(), store.lockPath)
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	if _, err = store.read(); errors.Is(err, os.ErrNotExist) {
		err = store.write(outboxSnapshot{Version: 1, Jobs: []OutboxJob{}})
	}
	return store, err
}

func (s *OutboxStore) Enqueue(ctx context.Context, value domain.Notification) (OutboxJob, error) {
	lock, err := storage.AcquireFileLock(ctx, s.lockPath)
	if err != nil {
		return OutboxJob{}, err
	}
	defer lock.Release()
	snapshot, err := s.read()
	if err != nil {
		return OutboxJob{}, err
	}
	if len(snapshot.Jobs) >= s.capacity {
		return OutboxJob{}, ErrQueueFull
	}
	now := s.now().UTC()
	id, err := newOutboxID()
	if err != nil {
		return OutboxJob{}, err
	}
	job := OutboxJob{ID: id, Notification: value, Status: OutboxPending, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	snapshot.Jobs = append(snapshot.Jobs, job)
	if err = s.write(snapshot); err != nil {
		return OutboxJob{}, err
	}
	return job, nil
}

func (s *OutboxStore) Claim(ctx context.Context, owner string, lease time.Duration) (OutboxJob, bool, error) {
	if owner == "" || lease <= 0 {
		return OutboxJob{}, false, errors.New("outbox owner and lease are required")
	}
	lock, err := storage.AcquireFileLock(ctx, s.lockPath)
	if err != nil {
		return OutboxJob{}, false, err
	}
	defer lock.Release()
	snapshot, err := s.read()
	if err != nil {
		return OutboxJob{}, false, err
	}
	now := s.now().UTC()
	sort.SliceStable(snapshot.Jobs, func(i, j int) bool { return snapshot.Jobs[i].CreatedAt.Before(snapshot.Jobs[j].CreatedAt) })
	for index := range snapshot.Jobs {
		job := &snapshot.Jobs[index]
		if job.Status == OutboxLeased && job.LeaseExpiresAt != nil && !job.LeaseExpiresAt.After(now) {
			job.Status, job.LeaseOwner, job.LeaseExpiresAt = OutboxPending, "", nil
		}
		if job.Status != OutboxPending || job.AvailableAt.After(now) {
			continue
		}
		expires := now.Add(lease)
		job.Status, job.LeaseOwner, job.LeaseExpiresAt, job.UpdatedAt = OutboxLeased, owner, &expires, now
		job.AttemptCount++
		if err = s.write(snapshot); err != nil {
			return OutboxJob{}, false, err
		}
		return *job, true, nil
	}
	return OutboxJob{}, false, nil
}

func (s *OutboxStore) Complete(ctx context.Context, id, owner string) error {
	return s.updateClaimed(ctx, id, owner, func(snapshot *outboxSnapshot, index int, _ time.Time) {
		snapshot.Jobs = append(snapshot.Jobs[:index], snapshot.Jobs[index+1:]...)
	})
}

func (s *OutboxStore) Retry(ctx context.Context, id, owner, message string, availableAt time.Time) error {
	return s.updateClaimed(ctx, id, owner, func(snapshot *outboxSnapshot, index int, now time.Time) {
		job := &snapshot.Jobs[index]
		job.Status, job.LeaseOwner, job.LeaseExpiresAt = OutboxPending, "", nil
		job.LastError, job.AvailableAt, job.UpdatedAt = message, availableAt.UTC(), now
	})
}

func (s *OutboxStore) updateClaimed(ctx context.Context, id, owner string, update func(*outboxSnapshot, int, time.Time)) error {
	lock, err := storage.AcquireFileLock(ctx, s.lockPath)
	if err != nil {
		return err
	}
	defer lock.Release()
	snapshot, err := s.read()
	if err != nil {
		return err
	}
	for index := range snapshot.Jobs {
		job := snapshot.Jobs[index]
		if job.ID != id {
			continue
		}
		if job.Status != OutboxLeased || job.LeaseOwner != owner {
			return errors.New("outbox job lease is not owned by worker")
		}
		update(&snapshot, index, s.now().UTC())
		return s.write(snapshot)
	}
	return os.ErrNotExist
}

func (s *OutboxStore) read() (outboxSnapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return outboxSnapshot{}, err
	}
	var snapshot outboxSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return outboxSnapshot{}, err
	}
	if snapshot.Version != 1 || snapshot.Jobs == nil {
		return outboxSnapshot{}, errors.New("invalid notification outbox snapshot")
	}
	return snapshot, nil
}

func (s *OutboxStore) write(snapshot outboxSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}

func newOutboxID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "job_" + hex.EncodeToString(value), nil
}
