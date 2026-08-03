package storage

import (
	"context"
	"sync"
)

type LockManager struct{ locks sync.Map }

type senderLockKey struct{}

func (m *LockManager) Get(id string) *sync.RWMutex {
	v, _ := m.locks.LoadOrStore(id, &sync.RWMutex{})
	return v.(*sync.RWMutex)
}

// ContextWithSenderLock marks that the caller already holds the per-sender mutex.
// Repository methods honor this to avoid deadlocks when service and repository share LockManager.
func ContextWithSenderLock(ctx context.Context, senderID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, senderLockKey{}, senderID)
}

func SenderLockHeld(ctx context.Context, senderID string) bool {
	if ctx == nil {
		return false
	}
	held, _ := ctx.Value(senderLockKey{}).(string)
	return held == senderID
}
