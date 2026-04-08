package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	appNotification "github.com/insider/notification-system/internal/application/notification"
	httpAdapter "github.com/insider/notification-system/internal/adapters/http"
	domain "github.com/insider/notification-system/internal/domain/notification"
	"github.com/insider/notification-system/internal/mocks"
	"github.com/insider/notification-system/pkg/metrics"
)

var (
	testMetricsOnce sync.Once
	testMetrics     *metrics.Metrics
)

func getTestMetrics() *metrics.Metrics {
	testMetricsOnce.Do(func() {
		testMetrics = metrics.New()
	})
	return testMetrics
}

func init() {
	gin.SetMode(gin.TestMode)
}

// setupRouter creates a test router with mocked dependencies.
func setupRouter(t *testing.T) (*gin.Engine, *mocks.MockRepository, *mocks.MockQueue, *mocks.MockProvider, *mocks.MockRateLimiter) {
	t.Helper()

	repo := &mocks.MockRepository{}
	queue := &mocks.MockQueue{}
	provider := &mocks.MockProvider{}
	rateLimiter := &mocks.MockRateLimiter{}
	m := getTestMetrics()

	svc := appNotification.NewService(repo, queue, provider, rateLimiter, m)
	log := zap.NewNop()
	handler := httpAdapter.NewHandler(svc, log)
	handler.StartHub()
	router := httpAdapter.NewRouter(handler, log, m)

	return router, repo, queue, provider, rateLimiter
}

func TestHealthCheck(t *testing.T) {
	router, _, _, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
}

func TestCreateNotification_Handler_Success(t *testing.T) {
	router, repo, queue, _, _ := setupRouter(t)

	repo.On("GetByIdempotencyKey", mock.Anything, mock.Anything).Return(nil, domain.ErrNotFound)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)

	payload := map[string]interface{}{
		"recipient": "user@example.com",
		"channel":   "email",
		"content":   "Hello from test",
		"priority":  "high",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp appNotification.NotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "user@example.com", resp.Recipient)
	assert.Equal(t, "email", resp.Channel)
}

func TestCreateNotification_Handler_InvalidBody(t *testing.T) {
	router, _, _, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications", bytes.NewReader([]byte(`{invalid}`)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateNotification_Handler_MissingRecipient(t *testing.T) {
	router, _, _, _, _ := setupRouter(t)

	payload := map[string]interface{}{
		"channel": "email",
		"content": "Hello",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetNotification_Handler_Found(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	n := &domain.Notification{
		ID:        "test-id-123",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
		Content:   "Hello",
		Status:    domain.StatusSent,
		Priority:  domain.PriorityNormal,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo.On("GetByID", mock.Anything, "test-id-123").Return(n, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications/test-id-123", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp appNotification.NotificationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "test-id-123", resp.ID)
}

func TestGetNotification_Handler_NotFound(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	repo.On("GetByID", mock.Anything, "missing-id").Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications/missing-id", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCancelNotification_Handler_Success(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	n := &domain.Notification{
		ID:     "test-id-456",
		Status: domain.StatusPending,
	}
	repo.On("GetByID", mock.Anything, "test-id-456").Return(n, nil)
	repo.On("Update", mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/notifications/test-id-456", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestCancelNotification_Handler_AlreadySent(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	n := &domain.Notification{
		ID:     "test-id-789",
		Status: domain.StatusSent,
	}
	repo.On("GetByID", mock.Anything, "test-id-789").Return(n, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/notifications/test-id-789", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestListNotifications_Handler(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	notifications := []*domain.Notification{
		{
			ID:        "n1",
			Channel:   domain.ChannelEmail,
			Status:    domain.StatusSent,
			Priority:  domain.PriorityNormal,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	repo.On("List", mock.Anything, mock.AnythingOfType("notification.ListFilter")).
		Return(notifications, int64(1), nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications?page=1&page_size=20", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp appNotification.ListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Total)
	assert.Len(t, resp.Data, 1)
}

func TestCreateBatch_Handler_Success(t *testing.T) {
	router, repo, queue, _, _ := setupRouter(t)

	repo.On("CreateBatch", mock.Anything, mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	payload := map[string]interface{}{
		"notifications": []map[string]interface{}{
			{"recipient": "a@example.com", "channel": "email", "content": "Msg 1"},
			{"recipient": "b@example.com", "channel": "sms", "content": "Msg 2"},
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp appNotification.BatchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.BatchID)
	assert.Equal(t, 2, resp.Total)
}

func TestCreateTemplate_Handler_Success(t *testing.T) {
	router, repo, _, _, _ := setupRouter(t)

	repo.On("CreateTemplate", mock.Anything, mock.AnythingOfType("*notification.Template")).Return(nil)

	payload := map[string]interface{}{
		"name":    "promo",
		"channel": "email",
		"content": "Hi {{.Name}}, check this out!",
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp appNotification.TemplateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "promo", resp.Name)
}

func TestCorrelationID_Header_Set(t *testing.T) {
	router, _, _, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Correlation-ID", "my-correlation-id")
	router.ServeHTTP(w, req)

	assert.Equal(t, "my-correlation-id", w.Header().Get("X-Correlation-ID"))
}

func TestCorrelationID_Header_Generated(t *testing.T) {
	router, _, _, _, _ := setupRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(w, req)

	// Should have generated a correlation ID.
	assert.NotEmpty(t, w.Header().Get("X-Correlation-ID"))
}
