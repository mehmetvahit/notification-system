package notification

import "time"

// Channel represents the notification delivery channel.
type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

// Status represents the lifecycle state of a notification.
type Status string

const (
	StatusPending    Status = "pending"
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
	StatusScheduled  Status = "scheduled"
)

// Priority represents the delivery priority of a notification.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

// Notification is the core domain entity.
type Notification struct {
	ID             string
	BatchID        string
	Recipient      string
	Channel        Channel
	Content        string
	Status         Status
	Priority       Priority
	IdempotencyKey string
	TemplateID     string
	TemplateVars   map[string]string
	ScheduledAt    *time.Time
	SentAt         *time.Time
	RetryCount     int
	MaxRetries     int
	ProviderMsgID  string
	ErrorMsg       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CanCancel returns true if the notification can be cancelled in its current state.
func (n *Notification) CanCancel() bool {
	return n.Status == StatusPending || n.Status == StatusQueued || n.Status == StatusScheduled
}

// CanRetry returns true if the notification is eligible for another delivery attempt.
func (n *Notification) CanRetry() bool {
	return n.Status == StatusFailed && n.RetryCount < n.MaxRetries
}

// IsScheduled returns true when the notification is a future-dated scheduled item.
func (n *Notification) IsScheduled() bool {
	return n.ScheduledAt != nil && n.Status == StatusScheduled
}

// Template holds reusable message content with variable placeholders.
type Template struct {
	ID        string
	Name      string
	Channel   Channel
	Content   string // supports {{.VarName}} placeholders
	CreatedAt time.Time
	UpdatedAt time.Time
}
