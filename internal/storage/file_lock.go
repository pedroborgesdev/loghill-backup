package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var errFileLockBusy = errors.New("file lock is already held")

// FileLock is an advisory, cross-process exclusive lock. The lock file is
// intentionally kept after release: deleting it could let two processes lock
// different inodes for the same logical resource.
type FileLock struct {
	file *os.File
	once sync.Once
}

func AcquireFileLock(ctx context.Context, path string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return nil, err
	}
	for {
		err = tryLockFile(file)
		if err == nil {
			return &FileLock{file: file}, nil
		}
		if !errors.Is(err, errFileLockBusy) {
			_ = file.Close()
			return nil, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	var releaseErr error
	l.once.Do(func() {
		unlockErr := unlockFile(l.file)
		closeErr := l.file.Close()
		if unlockErr != nil {
			releaseErr = unlockErr
		} else {
			releaseErr = closeErr
		}
	})
	return releaseErr
}
