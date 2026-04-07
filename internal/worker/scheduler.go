package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// Scheduler polls the repository for scheduled notifications that are due
// and moves them into the queue for delivery.
type Scheduler struct {
	repo     domain.Repository
	queue    domain.Queue
	interval time.Duration
	log      *zap.Logger
}

// NewScheduler creates a new notification scheduler.
func NewScheduler(repo domain.Repository, queue domain.Queue, log *zap.Logger) *Scheduler {
	return &Scheduler{
		repo:     repo,
		queue:    queue,
		interval: 30 * time.Second,
		log:      log,
	}
}

// Start begins the polling loop. It blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	s.log.Info("scheduler started", zap.Duration("interval", s.interval))

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run once immediately on start.
	s.processDue(ctx)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.processDue(ctx)
		}
	}
}

// processDue finds all scheduled notifications due by now and enqueues them.
func (s *Scheduler) processDue(ctx context.Context) {
	now := time.Now()
	notifications, err := s.repo.GetPendingScheduled(ctx, now)
	if err != nil {
		s.log.Error("scheduler: get pending scheduled failed", zap.Error(err))
		return
	}

	if len(notifications) == 0 {
		return
	}

	s.log.Info("scheduler: processing due notifications", zap.Int("count", len(notifications)))

	for _, n := range notifications {
		n.Status = domain.StatusQueued
		n.UpdatedAt = time.Now()

		if err := s.repo.Update(ctx, n); err != nil {
			s.log.Error("scheduler: update status failed",
				zap.String("id", n.ID),
				zap.Error(err),
			)
			continue
		}

		if err := s.queue.Enqueue(ctx, n); err != nil {
			s.log.Error("scheduler: enqueue failed",
				zap.String("id", n.ID),
				zap.Error(err),
			)
			// Revert status.
			n.Status = domain.StatusScheduled
			if revertErr := s.repo.Update(ctx, n); revertErr != nil {
				s.log.Error("scheduler: revert status failed", zap.String("id", n.ID), zap.Error(revertErr))
			}
			continue
		}

		s.log.Info("scheduler: notification enqueued", zap.String("id", n.ID))
	}
}
