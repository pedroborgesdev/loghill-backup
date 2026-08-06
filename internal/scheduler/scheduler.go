package scheduler

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"logtheater/internal/services"
)

type Scheduler struct {
	svc      *services.SenderService
	interval time.Duration
	ready    atomic.Bool
}

func New(s *services.SenderService, d time.Duration) *Scheduler {
	return &Scheduler{svc: s, interval: d}
}
func (s *Scheduler) Run(ctx context.Context) {
	s.ready.Store(true)
	defer s.ready.Store(false)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.svc.Tick(ctx); err != nil {
				slog.Error("cleanup failed", "error", err)
			}
		}
	}
}
func (s *Scheduler) Ready() bool { return s.ready.Load() }
