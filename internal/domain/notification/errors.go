package notification

import "errors"

// Sentinel errors for the notification domain.
var (
	ErrNotFound          = errors.New("notification not found")
	ErrAlreadyExists     = errors.New("notification already exists (idempotency)")
	ErrInvalidChannel    = errors.New("invalid channel")
	ErrInvalidPriority   = errors.New("invalid priority")
	ErrInvalidStatus     = errors.New("invalid status")
	ErrCannotCancel      = errors.New("notification cannot be cancelled in current status")
	ErrBatchTooLarge     = errors.New("batch size exceeds maximum of 1000")
	ErrContentTooLong    = errors.New("content exceeds maximum length for channel")
	ErrMissingRecipient  = errors.New("recipient is required")
	ErrRateLimitExceeded = errors.New("rate limit exceeded for channel")
	ErrTemplateNotFound  = errors.New("template not found")
)
