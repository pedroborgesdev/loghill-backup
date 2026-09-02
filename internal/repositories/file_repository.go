package repositories

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"logtheater/internal/domain"
	"logtheater/internal/storage"
)

type FileRepository struct {
	root     string
	lockRoot string
}

// SenderRepository is the persistence contract for senders, their process
// instances, and log entries. Implementations must keep mutations for the same
// sender serialized. The file driver combines atomic filesystem operations
// with locks scoped to the shared data directory.
type SenderRepository interface {
	Init() error
	LockSender(context.Context, string) (context.Context, func(), error)
	Create(context.Context, domain.Sender) error
	Get(context.Context, string) (domain.Sender, error)
	Update(context.Context, domain.Sender) error
	All(context.Context) ([]domain.Sender, error)
	DeleteLogs(context.Context, string) error
	Delete(context.Context, string) error
	Repair(context.Context, domain.Sender) (domain.Sender, error)
	RegisterInstance(context.Context, string, domain.SenderInstance) error
	InstanceExists(context.Context, string, string) (bool, error)
	GetInstance(context.Context, string, string) (domain.SenderInstance, error)
	InstanceCount(context.Context, string) (int, error)
	RegisteredInstances(context.Context, string) ([]domain.SenderInstance, error)
	DeleteInstance(context.Context, string, string) error
	TouchInstance(context.Context, string, string, time.Time, bool) error
	ListInstances(context.Context, string) ([]domain.SenderInstance, error)
	Append(context.Context, string, domain.LogEntry, domain.NumberUnitValue) (int64, int64, error)
	Compact(context.Context, string, int) (int64, int64, error)
	CompactByLimit(context.Context, string, domain.NumberUnitValue) (int64, int64, error)
	ListLogs(context.Context, string, domain.LogFilters) (domain.LogPage, error)
	RecentLogCounts(context.Context, string, time.Time) (int64, int64, int64, error)
	GetEventOccurrence(context.Context, string, string) (EventOccurrenceRecord, error)
	SaveEventOccurrence(context.Context, string, EventOccurrenceRecord) error
}

type EventOccurrenceRecord struct {
	ID          string          `json:"id"`
	Fingerprint string          `json:"fingerprint"`
	Entry       domain.LogEntry `json:"entry"`
	ReceivedAt  time.Time       `json:"received_at"`
}

var _ SenderRepository = (*FileRepository)(nil)

type persistedSender struct {
	domain.Sender
	KeyHash string `json:"key_hash,omitempty"`
}

type persistedInstances struct {
	Items []persistedInstance `json:"items"`
}

type persistedInstance struct {
	domain.SenderInstance
	TokenHash string `json:"token_hash,omitempty"`
}

func New(root string) *FileRepository {
	return &FileRepository{
		root:     filepath.Join(root, "senders"),
		lockRoot: filepath.Join(root, ".locks", "senders"),
	}
}

func NewSenderRepository(root string) SenderRepository { return New(root) }

func (r *FileRepository) Init() error {
	if err := os.MkdirAll(r.root, 0o750); err != nil {
		return err
	}
	return os.MkdirAll(r.lockRoot, 0o750)
}

func (r *FileRepository) LockSender(ctx context.Context, senderID string) (context.Context, func(), error) {
	if !validID(senderID) {
		return ctx, nil, domain.ErrNotFound
	}
	if storage.SenderLockHeld(ctx, senderID) {
		return ctx, func() {}, nil
	}
	fileLock, err := storage.AcquireFileLock(ctx, filepath.Join(r.lockRoot, senderID+".lock"))
	if err != nil {
		return ctx, nil, err
	}
	lockedCtx := storage.ContextWithSenderLock(ctx, senderID)
	return lockedCtx, func() {
		_ = fileLock.Release()
	}, nil
}

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

func (r *FileRepository) acquireWrite(ctx context.Context, senderID string) (release func(), err error) {
	if storage.SenderLockHeld(ctx, senderID) {
		return func() {}, nil
	}
	_, release, err = r.LockSender(ctx, senderID)
	return release, err
}

func (r *FileRepository) acquireRead(ctx context.Context, senderID string) (release func(), err error) {
	if storage.SenderLockHeld(ctx, senderID) {
		return func() {}, nil
	}
	_, release, err = r.LockSender(ctx, senderID)
	return release, err
}
