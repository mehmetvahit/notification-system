package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const (
	correlationIDKey contextKey = "correlation_id"
	loggerKey        contextKey = "logger"
)

// New creates a production-ready zap logger.
func New(debug bool) (*zap.Logger, error) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}
	return cfg.Build()
}

// WithCorrelationID attaches a correlation ID to the context and returns a derived logger.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	log := FromContext(ctx).With(zap.String("correlation_id", correlationID))
	ctx = context.WithValue(ctx, correlationIDKey, correlationID)
	ctx = context.WithValue(ctx, loggerKey, log)
	return ctx
}

// CorrelationIDFromContext extracts the correlation ID string from the context.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	return ""
}

// FromContext returns the zap.Logger stored in the context, or the global logger.
func FromContext(ctx context.Context) *zap.Logger {
	if log, ok := ctx.Value(loggerKey).(*zap.Logger); ok && log != nil {
		return log
	}
	return zap.L()
}

// WithLogger stores a logger in the context.
func WithLogger(ctx context.Context, log *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// Info logs at info level using the logger from context.
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Info(msg, fields...)
}

// Error logs at error level using the logger from context.
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Error(msg, fields...)
}

// Warn logs at warn level using the logger from context.
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Warn(msg, fields...)
}

// Debug logs at debug level using the logger from context.
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	FromContext(ctx).Debug(msg, fields...)
}
