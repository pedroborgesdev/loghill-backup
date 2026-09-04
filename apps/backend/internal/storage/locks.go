package storage

import "context"

type senderLockKey struct{}

// ContextWithSenderLock marks that the caller already holds the cross-process
// lock for the sender. Repository methods honor this marker to keep a composed
// mutation inside the same critical section.
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
