package notification

import (
	"context"
	"time"
)

// Repository defines the persistence port for notifications.
type Repository interface {
	Create(ctx context.Context, n *Notification) error
	CreateBatch(ctx context.Context, notifications []*Notification) error
	GetByID(ctx context.Context, id string) (*Notification, error)
	GetByBatchID(ctx context.Context, batchID string) ([]*Notification, error)
	Update(ctx context.Context, n *Notification) error
	List(ctx context.Context, filter ListFilter) ([]*Notification, int64, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Notification, error)
	GetPendingScheduled(ctx context.Context, before time.Time) ([]*Notification, error)
	GetTemplateByID(ctx context.Context, id string) (*Template, error)
	CreateTemplate(ctx context.Context, t *Template) error
}

// Queue defines the messaging port for async notification delivery.
type Queue interface {
	Enqueue(ctx context.Context, n *Notification) error
	Dequeue(ctx context.Context) (*Notification, error)
	Len(ctx context.Context, channel Channel, priority Priority) (int64, error)
}

// Provider defines the external delivery port (e.g. webhook, SMS gateway).
type Provider interface {
	Send(ctx context.Context, n *Notification) (providerMsgID string, err error)
}

// RateLimiter defines the rate-control port per channel.
type RateLimiter interface {
	Allow(ctx context.Context, channel Channel) (bool, error)
	Remaining(ctx context.Context, channel Channel) (int64, error)
}

// ListFilter defines query parameters for listing notifications.
type ListFilter struct {
	Status    *Status
	Channel   *Channel
	BatchID   *string
	StartDate *time.Time
	EndDate   *time.Time
	Page      int
	PageSize  int
}
