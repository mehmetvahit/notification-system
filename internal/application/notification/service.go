package notification

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/google/uuid"
	domain "github.com/insider/notification-system/internal/domain/notification"
	"github.com/insider/notification-system/pkg/logger"
	"github.com/insider/notification-system/pkg/metrics"
	"go.uber.org/zap"
)

const (
	maxSMSLength   = 160
	maxEmailLength = 10000
	maxPushLength  = 256
	defaultRetries = 3
)

// Service implements all notification business logic.
type Service struct {
	repo        domain.Repository
	queue       domain.Queue
	provider    domain.Provider
	rateLimiter domain.RateLimiter
	metrics     *metrics.Metrics
}

// NewService constructs a new application service.
func NewService(
	repo domain.Repository,
	queue domain.Queue,
	provider domain.Provider,
	rateLimiter domain.RateLimiter,
	m *metrics.Metrics,
) *Service {
	return &Service{
		repo:        repo,
		queue:       queue,
		provider:    provider,
		rateLimiter: rateLimiter,
		metrics:     m,
	}
}

// CreateNotification validates, persists, and enqueues a single notification.
func (s *Service) CreateNotification(ctx context.Context, req CreateNotificationRequest) (*NotificationResponse, error) {
	log := logger.FromContext(ctx)

	if req.Recipient == "" {
		return nil, domain.ErrMissingRecipient
	}

	ch := domain.Channel(req.Channel)
	if err := validateChannel(ch); err != nil {
		return nil, err
	}

	priority := domain.PriorityNormal
	if req.Priority != "" {
		priority = domain.Priority(req.Priority)
		if err := validatePriority(priority); err != nil {
			return nil, err
		}
	}

	// Resolve content from template or direct content.
	content := req.Content
	if req.TemplateID != "" {
		tmpl, err := s.repo.GetTemplateByID(ctx, req.TemplateID)
		if err != nil {
			return nil, fmt.Errorf("get template: %w", err)
		}
		rendered, err := renderTemplate(tmpl.Content, req.TemplateVars)
		if err != nil {
			return nil, fmt.Errorf("render template: %w", err)
		}
		content = rendered
	}

	if err := validateContentLength(ch, content); err != nil {
		return nil, err
	}

	// Idempotency check.
	if req.IdempotencyKey != "" {
		existing, err := s.repo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
		if err == nil && existing != nil {
			log.Info("idempotent request, returning existing notification", zap.String("id", existing.ID))
			return toResponse(existing), nil
		}
	}

	status := domain.StatusPending
	if req.ScheduledAt != nil && req.ScheduledAt.After(time.Now()) {
		status = domain.StatusScheduled
	}

	n := &domain.Notification{
		ID:             uuid.NewString(),
		Recipient:      req.Recipient,
		Channel:        ch,
		Content:        content,
		Status:         status,
		Priority:       priority,
		IdempotencyKey: req.IdempotencyKey,
		TemplateID:     req.TemplateID,
		TemplateVars:   req.TemplateVars,
		ScheduledAt:    req.ScheduledAt,
		MaxRetries:     defaultRetries,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("persist notification: %w", err)
	}

	if status == domain.StatusPending {
		if err := s.enqueueNotification(ctx, n); err != nil {
			log.Error("failed to enqueue notification", zap.String("id", n.ID), zap.Error(err))
		}
	}

	log.Info("notification created", zap.String("id", n.ID), zap.String("channel", string(ch)))
	return toResponse(n), nil
}

// CreateBatch creates up to 1000 notifications sharing a common batchID.
func (s *Service) CreateBatch(ctx context.Context, req CreateBatchRequest) (*BatchResponse, error) {
	if len(req.Notifications) > 1000 {
		return nil, domain.ErrBatchTooLarge
	}

	batchID := uuid.NewString()
	notifications := make([]*domain.Notification, 0, len(req.Notifications))

	for _, nr := range req.Notifications {
		ch := domain.Channel(nr.Channel)
		if err := validateChannel(ch); err != nil {
			return nil, err
		}

		priority := domain.PriorityNormal
		if nr.Priority != "" {
			priority = domain.Priority(nr.Priority)
		}

		content := nr.Content
		if nr.TemplateID != "" {
			tmpl, err := s.repo.GetTemplateByID(ctx, nr.TemplateID)
			if err != nil {
				return nil, fmt.Errorf("get template: %w", err)
			}
			rendered, err := renderTemplate(tmpl.Content, nr.TemplateVars)
			if err != nil {
				return nil, fmt.Errorf("render template: %w", err)
			}
			content = rendered
		}

		if err := validateContentLength(ch, content); err != nil {
			return nil, err
		}

		status := domain.StatusPending
		if nr.ScheduledAt != nil && nr.ScheduledAt.After(time.Now()) {
			status = domain.StatusScheduled
		}

		n := &domain.Notification{
			ID:           uuid.NewString(),
			BatchID:      batchID,
			Recipient:    nr.Recipient,
			Channel:      ch,
			Content:      content,
			Status:       status,
			Priority:     priority,
			TemplateID:   nr.TemplateID,
			TemplateVars: nr.TemplateVars,
			ScheduledAt:  nr.ScheduledAt,
			MaxRetries:   defaultRetries,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		notifications = append(notifications, n)
	}

	if err := s.repo.CreateBatch(ctx, notifications); err != nil {
		return nil, fmt.Errorf("persist batch: %w", err)
	}

	// Enqueue all pending notifications asynchronously.
	for _, n := range notifications {
		if n.Status == domain.StatusPending {
			if err := s.enqueueNotification(ctx, n); err != nil {
				logger.FromContext(ctx).Error("failed to enqueue batch notification",
					zap.String("id", n.ID), zap.Error(err))
			}
		}
	}

	responses := make([]NotificationResponse, len(notifications))
	for i, n := range notifications {
		responses[i] = *toResponse(n)
	}

	return &BatchResponse{
		BatchID:       batchID,
		Total:         len(notifications),
		Notifications: responses,
	}, nil
}

// GetByID retrieves a notification by its primary key.
func (s *Service) GetByID(ctx context.Context, id string) (*NotificationResponse, error) {
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toResponse(n), nil
}

// GetByBatchID retrieves all notifications belonging to a batch.
func (s *Service) GetByBatchID(ctx context.Context, batchID string) (*BatchResponse, error) {
	notifications, err := s.repo.GetByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
	}
	responses := make([]NotificationResponse, len(notifications))
	for i, n := range notifications {
		responses[i] = *toResponse(n)
	}
	return &BatchResponse{
		BatchID:       batchID,
		Total:         len(notifications),
		Notifications: responses,
	}, nil
}

// CancelNotification cancels a notification if it is in a cancellable state.
func (s *Service) CancelNotification(ctx context.Context, id string) error {
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !n.CanCancel() {
		return domain.ErrCannotCancel
	}
	n.Status = domain.StatusCancelled
	n.UpdatedAt = time.Now()
	return s.repo.Update(ctx, n)
}

// ListNotifications returns a paginated, filtered list of notifications.
func (s *Service) ListNotifications(ctx context.Context, filter domain.ListFilter) (*ListResponse, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}

	notifications, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	data := make([]NotificationResponse, len(notifications))
	for i, n := range notifications {
		data[i] = *toResponse(n)
	}

	totalPages := total / int64(filter.PageSize)
	if total%int64(filter.PageSize) != 0 {
		totalPages++
	}

	return &ListResponse{
		Data:       data,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// ProcessNotification is invoked by the worker to deliver a notification.
func (s *Service) ProcessNotification(ctx context.Context, n *domain.Notification) error {
	log := logger.FromContext(ctx)

	// Rate limit check.
	allowed, err := s.rateLimiter.Allow(ctx, n.Channel)
	if err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}
	if !allowed {
		return domain.ErrRateLimitExceeded
	}

	// Mark as processing.
	n.Status = domain.StatusProcessing
	n.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, n); err != nil {
		return fmt.Errorf("update to processing: %w", err)
	}

	start := time.Now()
	providerMsgID, sendErr := s.provider.Send(ctx, n)
	elapsed := time.Since(start)

	if s.metrics != nil {
		s.metrics.NotificationProcessingDuration.WithLabelValues(string(n.Channel)).Observe(elapsed.Seconds())
	}

	if sendErr != nil {
		log.Error("provider send failed", zap.String("id", n.ID), zap.Error(sendErr))
		n.RetryCount++
		n.ErrorMsg = sendErr.Error()
		n.UpdatedAt = time.Now()

		if n.CanRetry() {
			n.Status = domain.StatusFailed
			if err := s.repo.Update(ctx, n); err != nil {
				return fmt.Errorf("update failed status: %w", err)
			}
			if s.metrics != nil {
				s.metrics.NotificationsFailed.WithLabelValues(string(n.Channel)).Inc()
			}
			return fmt.Errorf("send failed, will retry: %w", sendErr)
		}

		n.Status = domain.StatusFailed
		if err := s.repo.Update(ctx, n); err != nil {
			return fmt.Errorf("update final failed status: %w", err)
		}
		if s.metrics != nil {
			s.metrics.NotificationsFailed.WithLabelValues(string(n.Channel)).Inc()
		}
		return sendErr
	}

	now := time.Now()
	n.Status = domain.StatusSent
	n.SentAt = &now
	n.ProviderMsgID = providerMsgID
	n.UpdatedAt = now

	if err := s.repo.Update(ctx, n); err != nil {
		return fmt.Errorf("update sent status: %w", err)
	}

	if s.metrics != nil {
		s.metrics.NotificationsSent.WithLabelValues(string(n.Channel)).Inc()
	}

	log.Info("notification sent", zap.String("id", n.ID), zap.String("providerMsgID", providerMsgID))
	return nil
}

// GetMetrics returns queue depth and delivery statistics.
func (s *Service) GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	channels := []domain.Channel{domain.ChannelSMS, domain.ChannelEmail, domain.ChannelPush}
	priorities := []domain.Priority{domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow}

	queueDepth := make(map[string]int64)
	for _, ch := range channels {
		for _, p := range priorities {
			key := fmt.Sprintf("%s:%s", ch, p)
			length, err := s.queue.Len(ctx, ch, p)
			if err == nil {
				queueDepth[key] = length
			}
		}
	}

	// Aggregate counts from DB via list queries.
	sentStatus := domain.StatusSent
	failedStatus := domain.StatusFailed
	pendingStatus := domain.StatusPending

	_, totalSent, _ := s.repo.List(ctx, domain.ListFilter{Status: &sentStatus, Page: 1, PageSize: 1})
	_, totalFailed, _ := s.repo.List(ctx, domain.ListFilter{Status: &failedStatus, Page: 1, PageSize: 1})
	_, totalPending, _ := s.repo.List(ctx, domain.ListFilter{Status: &pendingStatus, Page: 1, PageSize: 1})

	var successRate, failureRate float64
	total := totalSent + totalFailed
	if total > 0 {
		successRate = float64(totalSent) / float64(total) * 100
		failureRate = float64(totalFailed) / float64(total) * 100
	}

	return &MetricsResponse{
		QueueDepth:   queueDepth,
		SuccessRate:  successRate,
		FailureRate:  failureRate,
		TotalSent:    totalSent,
		TotalFailed:  totalFailed,
		TotalPending: totalPending,
	}, nil
}

// CreateTemplate persists a new message template.
func (s *Service) CreateTemplate(ctx context.Context, req CreateTemplateRequest) (*TemplateResponse, error) {
	t := &domain.Template{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Channel:   domain.Channel(req.Channel),
		Content:   req.Content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.repo.CreateTemplate(ctx, t); err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	return &TemplateResponse{
		ID:      t.ID,
		Name:    t.Name,
		Channel: string(t.Channel),
		Content: t.Content,
	}, nil
}

// GetTemplate retrieves a template by ID.
func (s *Service) GetTemplate(ctx context.Context, id string) (*TemplateResponse, error) {
	t, err := s.repo.GetTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &TemplateResponse{
		ID:      t.ID,
		Name:    t.Name,
		Channel: string(t.Channel),
		Content: t.Content,
	}, nil
}

// enqueueNotification sets the status to queued and pushes to the queue.
func (s *Service) enqueueNotification(ctx context.Context, n *domain.Notification) error {
	n.Status = domain.StatusQueued
	n.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, n); err != nil {
		return err
	}
	return s.queue.Enqueue(ctx, n)
}

// --- Helpers ---

func validateChannel(ch domain.Channel) error {
	switch ch {
	case domain.ChannelSMS, domain.ChannelEmail, domain.ChannelPush:
		return nil
	}
	return domain.ErrInvalidChannel
}

func validatePriority(p domain.Priority) error {
	switch p {
	case domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow:
		return nil
	}
	return domain.ErrInvalidPriority
}

func validateContentLength(ch domain.Channel, content string) error {
	var max int
	switch ch {
	case domain.ChannelSMS:
		max = maxSMSLength
	case domain.ChannelEmail:
		max = maxEmailLength
	case domain.ChannelPush:
		max = maxPushLength
	}
	if len(content) > max {
		return domain.ErrContentTooLong
	}
	return nil
}

func renderTemplate(tmplContent string, vars map[string]string) (string, error) {
	t, err := template.New("notification").Parse(tmplContent)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func toResponse(n *domain.Notification) *NotificationResponse {
	return &NotificationResponse{
		ID:             n.ID,
		BatchID:        n.BatchID,
		Recipient:      n.Recipient,
		Channel:        string(n.Channel),
		Content:        n.Content,
		Status:         string(n.Status),
		Priority:       string(n.Priority),
		IdempotencyKey: n.IdempotencyKey,
		ScheduledAt:    n.ScheduledAt,
		SentAt:         n.SentAt,
		RetryCount:     n.RetryCount,
		ErrorMsg:       n.ErrorMsg,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
	}
}
