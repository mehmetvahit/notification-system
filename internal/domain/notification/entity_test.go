package notification_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/insider/notification-system/internal/domain/notification"
)

func TestNotification_CanCancel(t *testing.T) {
	tests := []struct {
		name     string
		status   notification.Status
		expected bool
	}{
		{"pending can cancel", notification.StatusPending, true},
		{"queued can cancel", notification.StatusQueued, true},
		{"scheduled can cancel", notification.StatusScheduled, true},
		{"processing cannot cancel", notification.StatusProcessing, false},
		{"sent cannot cancel", notification.StatusSent, false},
		{"failed cannot cancel", notification.StatusFailed, false},
		{"cancelled cannot cancel", notification.StatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &notification.Notification{Status: tt.status}
			assert.Equal(t, tt.expected, n.CanCancel())
		})
	}
}

func TestNotification_CanRetry(t *testing.T) {
	tests := []struct {
		name       string
		status     notification.Status
		retryCount int
		maxRetries int
		expected   bool
	}{
		{"failed with retries remaining", notification.StatusFailed, 1, 3, true},
		{"failed at max retries", notification.StatusFailed, 3, 3, false},
		{"failed exceeds max retries", notification.StatusFailed, 5, 3, false},
		{"pending cannot retry", notification.StatusPending, 0, 3, false},
		{"sent cannot retry", notification.StatusSent, 0, 3, false},
		{"zero retries remaining", notification.StatusFailed, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &notification.Notification{
				Status:     tt.status,
				RetryCount: tt.retryCount,
				MaxRetries: tt.maxRetries,
			}
			assert.Equal(t, tt.expected, n.CanRetry())
		})
	}
}

func TestNotification_IsScheduled(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name        string
		status      notification.Status
		scheduledAt *time.Time
		expected    bool
	}{
		{"scheduled status with future time", notification.StatusScheduled, &future, true},
		{"scheduled status with past time", notification.StatusScheduled, &past, true},
		{"scheduled status without time", notification.StatusScheduled, nil, false},
		{"pending status with future time", notification.StatusPending, &future, false},
		{"sent status with future time", notification.StatusSent, &future, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &notification.Notification{
				Status:      tt.status,
				ScheduledAt: tt.scheduledAt,
			}
			assert.Equal(t, tt.expected, n.IsScheduled())
		})
	}
}

func TestChannelConstants(t *testing.T) {
	assert.Equal(t, notification.Channel("sms"), notification.ChannelSMS)
	assert.Equal(t, notification.Channel("email"), notification.ChannelEmail)
	assert.Equal(t, notification.Channel("push"), notification.ChannelPush)
}

func TestStatusConstants(t *testing.T) {
	assert.Equal(t, notification.Status("pending"), notification.StatusPending)
	assert.Equal(t, notification.Status("queued"), notification.StatusQueued)
	assert.Equal(t, notification.Status("processing"), notification.StatusProcessing)
	assert.Equal(t, notification.Status("sent"), notification.StatusSent)
	assert.Equal(t, notification.Status("failed"), notification.StatusFailed)
	assert.Equal(t, notification.Status("cancelled"), notification.StatusCancelled)
	assert.Equal(t, notification.Status("scheduled"), notification.StatusScheduled)
}

func TestPriorityConstants(t *testing.T) {
	assert.Equal(t, notification.Priority("high"), notification.PriorityHigh)
	assert.Equal(t, notification.Priority("normal"), notification.PriorityNormal)
	assert.Equal(t, notification.Priority("low"), notification.PriorityLow)
}

func TestNotification_DefaultValues(t *testing.T) {
	n := &notification.Notification{
		ID:        "test-id",
		Recipient: "user@example.com",
		Channel:   notification.ChannelEmail,
	}

	assert.Empty(t, n.BatchID)
	assert.Empty(t, n.ProviderMsgID)
	assert.Empty(t, n.ErrorMsg)
	assert.Nil(t, n.ScheduledAt)
	assert.Nil(t, n.SentAt)
	assert.Equal(t, 0, n.RetryCount)
}
