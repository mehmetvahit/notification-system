package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/insider/notification-system/pkg/logger"
	"github.com/insider/notification-system/pkg/metrics"
)

const correlationIDHeader = "X-Correlation-ID"

// CorrelationID middleware attaches a correlation ID to every request.
// It reads the X-Correlation-ID header if present, or generates a new UUID.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.GetHeader(correlationIDHeader)
		if correlationID == "" {
			correlationID = uuid.NewString()
		}
		c.Header(correlationIDHeader, correlationID)
		ctx := logger.WithCorrelationID(c.Request.Context(), correlationID)
		c.Request = c.Request.WithContext(ctx)
		c.Set("correlation_id", correlationID)
		c.Next()
	}
}

// RequestLogger logs structured request/response details using zap.
func RequestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		elapsed := time.Since(start)
		statusCode := c.Writer.Status()
		correlationID, _ := c.Get("correlation_id")

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", statusCode),
			zap.Duration("latency", elapsed),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Any("correlation_id", correlationID),
		}

		if len(c.Errors) > 0 {
			log.Error("request completed with errors", append(fields, zap.String("errors", c.Errors.String()))...)
		} else if statusCode >= http.StatusInternalServerError {
			log.Error("request completed", fields...)
		} else if statusCode >= http.StatusBadRequest {
			log.Warn("request completed", fields...)
		} else {
			log.Info("request completed", fields...)
		}
	}
}

// Recovery middleware recovers from panics and returns a 500 response.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// CORS middleware adds permissive CORS headers for development.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Correlation-ID")
		c.Header("Access-Control-Expose-Headers", "X-Correlation-ID")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// PrometheusMiddleware records HTTP request metrics.
func PrometheusMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		m.HTTPRequestDuration.WithLabelValues(c.Request.Method, c.FullPath(), status).Observe(elapsed)
		m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), status).Inc()
	}
}
