package notification

import "time"

// CreateNotificationRequest is the inbound DTO for creating a single notification.
type CreateNotificationRequest struct {
	Recipient      string            `json:"recipient" binding:"required"`
	Channel        string            `json:"channel" binding:"required,oneof=sms email push"`
	Content        string            `json:"content"`
	Priority       string            `json:"priority" binding:"omitempty,oneof=high normal low"`
	IdempotencyKey string            `json:"idempotency_key"`
	TemplateID     string            `json:"template_id"`
	TemplateVars   map[string]string `json:"template_vars"`
	ScheduledAt    *time.Time        `json:"scheduled_at"`
}

// CreateBatchRequest wraps multiple notification requests into one batch.
type CreateBatchRequest struct {
	Notifications []CreateNotificationRequest `json:"notifications" binding:"required,min=1,max=1000"`
}

// NotificationResponse is the outbound DTO for a single notification.
type NotificationResponse struct {
	ID             string            `json:"id"`
	BatchID        string            `json:"batch_id,omitempty"`
	Recipient      string            `json:"recipient"`
	Channel        string            `json:"channel"`
	Content        string            `json:"content"`
	Status         string            `json:"status"`
	Priority       string            `json:"priority"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time        `json:"scheduled_at,omitempty"`
	SentAt         *time.Time        `json:"sent_at,omitempty"`
	RetryCount     int               `json:"retry_count"`
	ErrorMsg       string            `json:"error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// BatchResponse wraps multiple notifications created in one batch operation.
type BatchResponse struct {
	BatchID       string                 `json:"batch_id"`
	Total         int                    `json:"total"`
	Notifications []NotificationResponse `json:"notifications"`
}

// ListResponse is the paginated outbound DTO for listing notifications.
type ListResponse struct {
	Data       []NotificationResponse `json:"data"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"page_size"`
	TotalPages int64                  `json:"total_pages"`
}

// MetricsResponse carries observability data for the /metrics endpoint.
type MetricsResponse struct {
	QueueDepth   map[string]int64 `json:"queue_depth"`
	SuccessRate  float64          `json:"success_rate"`
	FailureRate  float64          `json:"failure_rate"`
	TotalSent    int64            `json:"total_sent"`
	TotalFailed  int64            `json:"total_failed"`
	TotalPending int64            `json:"total_pending"`
	AvgLatencyMs float64          `json:"avg_latency_ms"`
}

// CreateTemplateRequest is the inbound DTO for creating a message template.
type CreateTemplateRequest struct {
	Name    string `json:"name" binding:"required"`
	Channel string `json:"channel" binding:"required,oneof=sms email push"`
	Content string `json:"content" binding:"required"`
}

// TemplateResponse is the outbound DTO for a message template.
type TemplateResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}
