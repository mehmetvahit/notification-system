package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appNotification "github.com/insider/notification-system/internal/application/notification"
	domain "github.com/insider/notification-system/internal/domain/notification"
	"github.com/insider/notification-system/internal/mocks"
)

func newTestService(
	repo *mocks.MockRepository,
	queue *mocks.MockQueue,
	provider *mocks.MockProvider,
	rateLimiter *mocks.MockRateLimiter,
) *appNotification.Service {
	return appNotification.NewService(repo, queue, provider, rateLimiter, nil)
}

func TestCreateNotification_Success(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	// No IdempotencyKey set, so GetByIdempotencyKey will NOT be called.
	repo.On("Create", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateNotificationRequest{
		Recipient: "user@example.com",
		Channel:   "email",
		Content:   "Hello World",
		Priority:  "normal",
	}

	resp, err := svc.CreateNotification(context.Background(), req)

	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "user@example.com", resp.Recipient)
	assert.Equal(t, "email", resp.Channel)
	assert.Equal(t, "Hello World", resp.Content)
	assert.Equal(t, "queued", resp.Status)

	repo.AssertExpectations(t)
	queue.AssertExpectations(t)
}

func TestCreateNotification_MissingRecipient(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateNotificationRequest{
		Channel: "email",
		Content: "Hello",
	}

	_, err := svc.CreateNotification(context.Background(), req)
	assert.ErrorIs(t, err, domain.ErrMissingRecipient)
}

func TestCreateNotification_InvalidChannel(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateNotificationRequest{
		Recipient: "user@example.com",
		Channel:   "carrier_pigeon",
		Content:   "Hello",
	}

	_, err := svc.CreateNotification(context.Background(), req)
	assert.ErrorIs(t, err, domain.ErrInvalidChannel)
}

func TestCreateNotification_ContentTooLong_SMS(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	svc := newTestService(repo, queue, provider, rateLimiter)

	// 161 characters - exceeds SMS limit.
	longContent := make([]byte, 161)
	for i := range longContent {
		longContent[i] = 'a'
	}

	req := appNotification.CreateNotificationRequest{
		Recipient: "+1234567890",
		Channel:   "sms",
		Content:   string(longContent),
	}

	_, err := svc.CreateNotification(context.Background(), req)
	assert.ErrorIs(t, err, domain.ErrContentTooLong)
}

func TestCreateNotification_Idempotency(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	existing := &domain.Notification{
		ID:             "existing-id",
		Recipient:      "user@example.com",
		Channel:        domain.ChannelEmail,
		Content:        "Hello",
		Status:         domain.StatusSent,
		Priority:       domain.PriorityNormal,
		IdempotencyKey: "idem-key-123",
	}

	repo.On("GetByIdempotencyKey", mock.Anything, "idem-key-123").Return(existing, nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateNotificationRequest{
		Recipient:      "user@example.com",
		Channel:        "email",
		Content:        "Hello",
		IdempotencyKey: "idem-key-123",
	}

	resp, err := svc.CreateNotification(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "existing-id", resp.ID)
	repo.AssertExpectations(t)
	// Ensure Create was NOT called.
	repo.AssertNotCalled(t, "Create")
}

func TestCreateNotification_Scheduled(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	// No IdempotencyKey, so GetByIdempotencyKey will NOT be called.
	repo.On("Create", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	future := time.Now().Add(time.Hour)
	req := appNotification.CreateNotificationRequest{
		Recipient:   "user@example.com",
		Channel:     "push",
		Content:     "Reminder!",
		ScheduledAt: &future,
	}

	resp, err := svc.CreateNotification(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "scheduled", resp.Status)
	// Queue should NOT be called for scheduled notifications.
	queue.AssertNotCalled(t, "Enqueue")
	repo.AssertExpectations(t)
}

func TestCreateBatch_Success(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	repo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateBatchRequest{
		Notifications: []appNotification.CreateNotificationRequest{
			{Recipient: "user1@example.com", Channel: "email", Content: "Hello 1"},
			{Recipient: "user2@example.com", Channel: "sms", Content: "Hello 2"},
		},
	}

	resp, err := svc.CreateBatch(context.Background(), req)

	require.NoError(t, err)
	assert.NotEmpty(t, resp.BatchID)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Notifications, 2)

	repo.AssertExpectations(t)
}

func TestCreateBatch_TooLarge(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	svc := newTestService(repo, queue, provider, rateLimiter)

	notifications := make([]appNotification.CreateNotificationRequest, 1001)
	for i := range notifications {
		notifications[i] = appNotification.CreateNotificationRequest{
			Recipient: "user@example.com",
			Channel:   "email",
			Content:   "Hello",
		}
	}

	req := appNotification.CreateBatchRequest{Notifications: notifications}
	_, err := svc.CreateBatch(context.Background(), req)
	assert.ErrorIs(t, err, domain.ErrBatchTooLarge)
}

func TestGetByID_Found(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:        "test-id",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
		Status:    domain.StatusSent,
	}
	repo.On("GetByID", mock.Anything, "test-id").Return(n, nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	resp, err := svc.GetByID(context.Background(), "test-id")

	require.NoError(t, err)
	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "sent", resp.Status)
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	repo.On("GetByID", mock.Anything, "missing-id").Return(nil, domain.ErrNotFound)

	svc := newTestService(repo, queue, provider, rateLimiter)

	_, err := svc.GetByID(context.Background(), "missing-id")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCancelNotification_Success(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:     "test-id",
		Status: domain.StatusPending,
	}
	repo.On("GetByID", mock.Anything, "test-id").Return(n, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	err := svc.CancelNotification(context.Background(), "test-id")
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestCancelNotification_AlreadySent(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:     "test-id",
		Status: domain.StatusSent,
	}
	repo.On("GetByID", mock.Anything, "test-id").Return(n, nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	err := svc.CancelNotification(context.Background(), "test-id")
	assert.ErrorIs(t, err, domain.ErrCannotCancel)
}

func TestProcessNotification_Success(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:         "test-id",
		Recipient:  "user@example.com",
		Channel:    domain.ChannelEmail,
		Content:    "Hello",
		Status:     domain.StatusQueued,
		MaxRetries: 3,
	}

	rateLimiter.On("Allow", mock.Anything, domain.ChannelEmail).Return(true, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil).Times(2)
	provider.On("Send", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return("provider-msg-123", nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	err := svc.ProcessNotification(context.Background(), n)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusSent, n.Status)
	assert.Equal(t, "provider-msg-123", n.ProviderMsgID)
	assert.NotNil(t, n.SentAt)
}

func TestProcessNotification_RateLimited(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:      "test-id",
		Channel: domain.ChannelSMS,
	}

	rateLimiter.On("Allow", mock.Anything, domain.ChannelSMS).Return(false, nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	err := svc.ProcessNotification(context.Background(), n)
	assert.ErrorIs(t, err, domain.ErrRateLimitExceeded)
}

func TestProcessNotification_ProviderError_WithRetry(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	n := &domain.Notification{
		ID:         "test-id",
		Channel:    domain.ChannelEmail,
		Status:     domain.StatusQueued,
		RetryCount: 0,
		MaxRetries: 3,
	}

	sendErr := errors.New("upstream timeout")

	rateLimiter.On("Allow", mock.Anything, domain.ChannelEmail).Return(true, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil).Times(2)
	provider.On("Send", mock.Anything, mock.Anything).Return("", sendErr)

	svc := newTestService(repo, queue, provider, rateLimiter)

	err := svc.ProcessNotification(context.Background(), n)
	assert.Error(t, err)
	assert.Equal(t, domain.StatusFailed, n.Status)
	assert.Equal(t, 1, n.RetryCount)
	// CanRetry should still be true (1 < 3).
	assert.True(t, n.CanRetry())
}

func TestListNotifications_WithPagination(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	notifications := []*domain.Notification{
		{ID: "1", Channel: domain.ChannelEmail, Status: domain.StatusSent},
		{ID: "2", Channel: domain.ChannelSMS, Status: domain.StatusSent},
	}

	repo.On("List", mock.Anything, mock.AnythingOfType("notification.ListFilter")).
		Return(notifications, int64(2), nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	filter := domain.ListFilter{Page: 1, PageSize: 10}
	resp, err := svc.ListNotifications(context.Background(), filter)

	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, int64(1), resp.TotalPages)
}

func TestCreateTemplate_Success(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	repo.On("CreateTemplate", mock.Anything, mock.AnythingOfType("*notification.Template")).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateTemplateRequest{
		Name:    "welcome-email",
		Channel: "email",
		Content: "Welcome {{.Name}}!",
	}

	resp, err := svc.CreateTemplate(context.Background(), req)

	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "welcome-email", resp.Name)
	assert.Equal(t, "email", resp.Channel)
}

func TestCreateNotification_WithTemplate(t *testing.T) {
	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}

	tmpl := &domain.Template{
		ID:      "tmpl-123",
		Name:    "welcome",
		Channel: domain.ChannelEmail,
		Content: "Hello {{.Name}}, welcome!",
	}

	repo.On("GetByIdempotencyKey", mock.Anything, mock.Anything).Return(nil, domain.ErrNotFound)
	repo.On("GetTemplateByID", mock.Anything, "tmpl-123").Return(tmpl, nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	svc := newTestService(repo, queue, provider, rateLimiter)

	req := appNotification.CreateNotificationRequest{
		Recipient:    "user@example.com",
		Channel:      "email",
		TemplateID:   "tmpl-123",
		TemplateVars: map[string]string{"Name": "Alice"},
	}

	resp, err := svc.CreateNotification(context.Background(), req)

	require.NoError(t, err)
	// The content should have been rendered from the template.
	assert.Contains(t, resp.Content, "Alice")
}
