package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	appNotification "github.com/insider/notification-system/internal/application/notification"
	domain "github.com/insider/notification-system/internal/domain/notification"
	"github.com/insider/notification-system/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Handler holds all HTTP handler methods for the notification API.
type Handler struct {
	service *appNotification.Service
	log     *zap.Logger
	hub     *WSHub
}

// NewHandler creates a new HTTP handler.
func NewHandler(service *appNotification.Service, log *zap.Logger) *Handler {
	return &Handler{
		service: service,
		log:     log,
		hub:     NewWSHub(),
	}
}

// StartHub starts the WebSocket broadcast hub in a background goroutine.
func (h *Handler) StartHub() {
	go h.hub.Run()
}

// --- Notification endpoints ---

// CreateNotification godoc
// @Summary      Create a single notification
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        request body appNotification.CreateNotificationRequest true "Notification payload"
// @Success      201 {object} appNotification.NotificationResponse
// @Failure      400 {object} map[string]string
// @Router       /api/v1/notifications [post]
func (h *Handler) CreateNotification(c *gin.Context) {
	var req appNotification.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	resp, err := h.service.CreateNotification(ctx, req)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	h.hub.Broadcast(resp)
	c.JSON(http.StatusCreated, resp)
}

// CreateBatch godoc
// @Summary      Create a batch of notifications
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        request body appNotification.CreateBatchRequest true "Batch payload"
// @Success      201 {object} appNotification.BatchResponse
// @Failure      400 {object} map[string]string
// @Router       /api/v1/notifications/batch [post]
func (h *Handler) CreateBatch(c *gin.Context) {
	var req appNotification.CreateBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	resp, err := h.service.CreateBatch(ctx, req)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// GetNotification godoc
// @Summary      Get notification by ID
// @Tags         notifications
// @Produce      json
// @Param        id path string true "Notification ID"
// @Success      200 {object} appNotification.NotificationResponse
// @Failure      404 {object} map[string]string
// @Router       /api/v1/notifications/{id} [get]
func (h *Handler) GetNotification(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	resp, err := h.service.GetByID(ctx, id)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetBatch godoc
// @Summary      Get all notifications in a batch
// @Tags         notifications
// @Produce      json
// @Param        batchId path string true "Batch ID"
// @Success      200 {object} appNotification.BatchResponse
// @Failure      404 {object} map[string]string
// @Router       /api/v1/notifications/batch/{batchId} [get]
func (h *Handler) GetBatch(c *gin.Context) {
	batchID := c.Param("batchId")
	ctx := c.Request.Context()

	resp, err := h.service.GetByBatchID(ctx, batchID)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// CancelNotification godoc
// @Summary      Cancel a notification
// @Tags         notifications
// @Produce      json
// @Param        id path string true "Notification ID"
// @Success      204
// @Failure      404,409 {object} map[string]string
// @Router       /api/v1/notifications/{id} [delete]
func (h *Handler) CancelNotification(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	if err := h.service.CancelNotification(ctx, id); err != nil {
		h.handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListNotifications godoc
// @Summary      List notifications with filtering and pagination
// @Tags         notifications
// @Produce      json
// @Param        status    query string false "Filter by status"
// @Param        channel   query string false "Filter by channel"
// @Param        page      query int    false "Page number (default 1)"
// @Param        page_size query int    false "Page size (default 20)"
// @Success      200 {object} appNotification.ListResponse
// @Router       /api/v1/notifications [get]
func (h *Handler) ListNotifications(c *gin.Context) {
	ctx := c.Request.Context()
	log := logger.FromContext(ctx)

	filter := domain.ListFilter{
		Page:     1,
		PageSize: 20,
	}

	if s := c.Query("status"); s != "" {
		status := domain.Status(s)
		filter.Status = &status
	}
	if ch := c.Query("channel"); ch != "" {
		channel := domain.Channel(ch)
		filter.Channel = &channel
	}
	if bid := c.Query("batch_id"); bid != "" {
		filter.BatchID = &bid
	}
	if p := c.Query("page"); p != "" {
		if page, err := strconv.Atoi(p); err == nil {
			filter.Page = page
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if pageSize, err := strconv.Atoi(ps); err == nil {
			filter.PageSize = pageSize
		}
	}
	if sd := c.Query("start_date"); sd != "" {
		if t, err := time.Parse(time.RFC3339, sd); err == nil {
			filter.StartDate = &t
		} else {
			log.Warn("invalid start_date format", zap.String("value", sd))
		}
	}
	if ed := c.Query("end_date"); ed != "" {
		if t, err := time.Parse(time.RFC3339, ed); err == nil {
			filter.EndDate = &t
		} else {
			log.Warn("invalid end_date format", zap.String("value", ed))
		}
	}

	resp, err := h.service.ListNotifications(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// --- Metrics endpoint ---

// GetMetrics godoc
// @Summary      Get system metrics
// @Tags         metrics
// @Produce      json
// @Success      200 {object} appNotification.MetricsResponse
// @Router       /api/v1/metrics [get]
func (h *Handler) GetMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	resp, err := h.service.GetMetrics(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// --- Template endpoints ---

// CreateTemplate godoc
// @Summary      Create a message template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Param        request body appNotification.CreateTemplateRequest true "Template payload"
// @Success      201 {object} appNotification.TemplateResponse
// @Failure      400 {object} map[string]string
// @Router       /api/v1/templates [post]
func (h *Handler) CreateTemplate(c *gin.Context) {
	var req appNotification.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	resp, err := h.service.CreateTemplate(ctx, req)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// GetTemplate godoc
// @Summary      Get template by ID
// @Tags         templates
// @Produce      json
// @Param        id path string true "Template ID"
// @Success      200 {object} appNotification.TemplateResponse
// @Failure      404 {object} map[string]string
// @Router       /api/v1/templates/{id} [get]
func (h *Handler) GetTemplate(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	resp, err := h.service.GetTemplate(ctx, id)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// --- Health check ---

// HealthCheck godoc
// @Summary      Health check
// @Tags         health
// @Produce      json
// @Success      200 {object} map[string]string
// @Router       /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Broadcast ---

// BroadcastMessageRequest is the HTTP body for POST /api/v1/broadcast.
type BroadcastMessageRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Text   string `json:"text" binding:"required"`
}

// BroadcastMessage is the payload pushed over the WebSocket hub.
type BroadcastMessage struct {
	Type      string    `json:"type"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// BroadcastMessage godoc
// @Summary      Broadcast a text message from a user to all WebSocket clients
// @Tags         broadcast
// @Accept       json
// @Produce      json
// @Param        request body BroadcastMessageRequest true "Broadcast payload"
// @Success      202 {object} BroadcastMessage
// @Failure      400 {object} map[string]string
// @Router       /api/v1/broadcast [post]
func (h *Handler) BroadcastMessage(c *gin.Context) {
	var req BroadcastMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	msg := BroadcastMessage{
		Type:      "message",
		UserID:    req.UserID,
		Text:      req.Text,
		Timestamp: time.Now().UTC(),
	}
	h.hub.Broadcast(msg)
	c.JSON(http.StatusAccepted, msg)
}

// --- WebSocket ---

// WebSocketHandler handles WebSocket upgrade and registers the client with the hub.
func (h *Handler) WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.log.Error("websocket upgrade failed", zap.Error(err))
		return
	}

	client := &WSClient{
		hub:  h.hub,
		conn: conn,
		send: make(chan interface{}, 256),
	}
	h.hub.Register(client)

	go client.writePump()
	go client.readPump()
}

// --- Error handling ---

func (h *Handler) handleServiceError(c *gin.Context, err error) {
	switch err {
	case domain.ErrNotFound, domain.ErrTemplateNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case domain.ErrAlreadyExists:
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case domain.ErrCannotCancel:
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case domain.ErrInvalidChannel, domain.ErrInvalidPriority, domain.ErrInvalidStatus,
		domain.ErrBatchTooLarge, domain.ErrContentTooLong, domain.ErrMissingRecipient:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case domain.ErrRateLimitExceeded:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
	default:
		h.log.Error("unhandled service error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

// --- WebSocket Hub ---

// WSHub manages WebSocket client registrations and broadcasts.
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan interface{}
	register   chan *WSClient
	unregister chan *WSClient
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan interface{}, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the hub event loop.
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case msg := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
		}
	}
}

// Register adds a new WebSocket client.
func (h *WSHub) Register(client *WSClient) {
	h.register <- client
}

// Broadcast sends a message to all connected WebSocket clients.
func (h *WSHub) Broadcast(msg interface{}) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

// WSClient represents a single WebSocket connection.
type WSClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan interface{}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *WSClient) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			break
		}
	}
}

// readPump reads messages from the WebSocket (ping/pong handling).
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
