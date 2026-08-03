package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"logtheater/internal/domain"
	"logtheater/internal/storage"
)

type FileRepository struct {
	root  string
	locks *storage.LockManager
}

type persistedSender struct {
	domain.Sender
	KeyHash string `json:"key_hash,omitempty"`
}

type persistedInstances struct {
	Items []domain.SenderInstance `json:"items"`
}

func New(root string) *FileRepository {
	return &FileRepository{root: filepath.Join(root, "senders"), locks: &storage.LockManager{}}
}

// Locks returns the shared per-sender lock manager. Callers that coordinate
// with repository mutations (e.g. the domain service) must reuse this instance.
func (r *FileRepository) Locks() *storage.LockManager { return r.locks }

func (r *FileRepository) Init() error { return os.MkdirAll(r.root, 0750) }

func validID(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return false
	}
	for _, c := range id {
		if !(c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func (r *FileRepository) dir(id string) (string, error) {
	if !validID(id) {
		return "", domain.ErrNotFound
	}
	return filepath.Join(r.root, id), nil
}

func (r *FileRepository) acquireWrite(ctx context.Context, senderID string) (release func()) {
	if storage.SenderLockHeld(ctx, senderID) {
		return func() {}
	}
	lock := r.locks.Get(senderID)
	lock.Lock()
	return lock.Unlock
}

func (r *FileRepository) acquireRead(ctx context.Context, senderID string) (release func()) {
	if storage.SenderLockHeld(ctx, senderID) {
		return func() {}
	}
	lock := r.locks.Get(senderID)
	lock.RLock()
	return lock.RUnlock
}
