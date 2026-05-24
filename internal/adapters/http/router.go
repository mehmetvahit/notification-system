package http

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	"github.com/insider/notification-system/pkg/metrics"
)

// NewRouter creates and configures the Gin router with all routes and middleware.
func NewRouter(handler *Handler, log *zap.Logger, m *metrics.Metrics) *gin.Engine {
	router := gin.New()

	// Global middleware.
	router.Use(Recovery(log))
	router.Use(CORS())
	router.Use(CorrelationID())
	router.Use(RequestLogger(log))
	router.Use(PrometheusMiddleware(m))

	// Health check (no auth required).
	router.GET("/health", handler.HealthCheck)

	// Prometheus metrics endpoint.
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// WebSocket endpoint.
	router.GET("/ws", handler.WebSocketHandler)

	// Swagger documentation.
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1 routes.
	v1 := router.Group("/api/v1")
	{
		notifications := v1.Group("/notifications")
		{
			notifications.POST("", handler.CreateNotification)
			notifications.POST("/batch", handler.CreateBatch)
			notifications.GET("", handler.ListNotifications)
			notifications.GET("/:id", handler.GetNotification)
			notifications.DELETE("/:id", handler.CancelNotification)
			notifications.GET("/batch/:batchId", handler.GetBatch)
		}

		templates := v1.Group("/templates")
		{
			templates.POST("", handler.CreateTemplate)
			templates.GET("/:id", handler.GetTemplate)
		}

		v1.GET("/metrics", handler.GetMetrics)

		v1.POST("/broadcast", handler.BroadcastMessage)
	}

	return router
}
