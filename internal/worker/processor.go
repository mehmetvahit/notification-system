package worker

import (
	"context"
	"math"
	"time"

	"go.uber.org/zap"

	appNotification "github.com/insider/notification-system/internal/application/notification"
	domain "github.com/insider/notification-system/internal/domain/notification"
	"github.com/insider/notification-system/pkg/logger"
	"github.com/insider/notification-system/pkg/metrics"
)

// Processor manages a pool of worker goroutines that dequeue and deliver notifications.
type Processor struct {
	service           *appNotification.Service
	queue             domain.Queue
	repo              domain.Repository
	workersPerChannel int
	retryBaseDelay    time.Duration
	maxRetries        int
	metrics           *metrics.Metrics
	log               *zap.Logger
}

// NewProcessor creates a new worker processor.
func NewProcessor(
	service *appNotification.Service,
	queue domain.Queue,
	repo domain.Repository,
	workersPerChannel int,
	retryBaseDelay time.Duration,
	maxRetries int,
	m *metrics.Metrics,
	log *zap.Logger,
) *Processor {
	return &Processor{
		service:           service,
		queue:             queue,
		repo:              repo,
		workersPerChannel: workersPerChannel,
		retryBaseDelay:    retryBaseDelay,
		maxRetries:        maxRetries,
		metrics:           m,
		log:               log,
	}
}

// Start launches N worker goroutines per channel. It blocks until ctx is cancelled.
func (p *Processor) Start(ctx context.Context) {
	channels := []domain.Channel{
		domain.ChannelSMS,
		domain.ChannelEmail,
		domain.ChannelPush,
	}

	for _, ch := range channels {
		for i := 0; i < p.workersPerChannel; i++ {
			workerID := i
			channel := ch
			go p.runWorker(ctx, channel, workerID)
		}
	}

	p.log.Info("worker processor started",
		zap.Int("workers_per_channel", p.workersPerChannel),
		zap.Int("total_workers", len(channels)*p.workersPerChannel),
	)

	<-ctx.Done()
	p.log.Info("worker processor stopped")
}

// runWorker continuously dequeues and processes notifications for a given channel.
func (p *Processor) runWorker(ctx context.Context, channel domain.Channel, workerID int) {
	p.log.Info("worker started",
		zap.String("channel", string(channel)),
		zap.Int("worker_id", workerID),
	)

	if p.metrics != nil {
		p.metrics.ActiveWorkers.WithLabelValues(string(channel)).Inc()
		defer p.metrics.ActiveWorkers.WithLabelValues(string(channel)).Dec()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := p.queue.Dequeue(ctx)
		if err != nil {
			p.log.Error("dequeue error", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		if n == nil {
			// Queue empty, back off.
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Only process notifications matching this worker's channel (best-effort).
		if n.Channel != channel {
			// Re-enqueue to the correct channel queue.
			if enqErr := p.queue.Enqueue(ctx, n); enqErr != nil {
				p.log.Error("re-enqueue error", zap.String("id", n.ID), zap.Error(enqErr))
			}
			continue
		}

		workerCtx := logger.WithLogger(ctx, p.log.With(
			zap.String("notification_id", n.ID),
			zap.String("channel", string(n.Channel)),
			zap.Int("worker_id", workerID),
		))

		p.processWithRetry(workerCtx, n)
	}
}

// processWithRetry attempts to process a notification, re-enqueuing on transient failures.
func (p *Processor) processWithRetry(ctx context.Context, n *domain.Notification) {
	log := logger.FromContext(ctx)

	err := p.service.ProcessNotification(ctx, n)
	if err == nil {
		return
	}

	if err == domain.ErrRateLimitExceeded {
		log.Warn("rate limit exceeded, backing off", zap.String("id", n.ID))
		if p.metrics != nil {
			p.metrics.RateLimitHits.WithLabelValues(string(n.Channel)).Inc()
		}
		// Sleep and re-enqueue.
		time.Sleep(time.Second)
		if enqErr := p.queue.Enqueue(ctx, n); enqErr != nil {
			log.Error("failed to re-enqueue after rate limit", zap.Error(enqErr))
		}
		return
	}

	// Re-fetch to get updated retry count.
	updated, fetchErr := p.repo.GetByID(ctx, n.ID)
	if fetchErr != nil {
		log.Error("failed to fetch notification after send error", zap.Error(fetchErr))
		return
	}

	if updated.CanRetry() {
		delay := backoffDelay(p.retryBaseDelay, updated.RetryCount)
		log.Info("scheduling retry",
			zap.String("id", n.ID),
			zap.Int("retry_count", updated.RetryCount),
			zap.Duration("delay", delay),
		)
		time.Sleep(delay)
		if enqErr := p.queue.Enqueue(ctx, updated); enqErr != nil {
			log.Error("failed to re-enqueue for retry", zap.Error(enqErr))
		}
	} else {
		log.Error("notification permanently failed",
			zap.String("id", n.ID),
			zap.Int("retry_count", updated.RetryCount),
		)
	}
}

// backoffDelay computes exponential backoff: 2^retryCount * baseDelay.
func backoffDelay(base time.Duration, retryCount int) time.Duration {
	multiplier := math.Pow(2, float64(retryCount))
	return time.Duration(multiplier * float64(base))
}
