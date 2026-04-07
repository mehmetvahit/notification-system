package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus instruments for the notification service.
type Metrics struct {
	NotificationsSent              *prometheus.CounterVec
	NotificationsFailed            *prometheus.CounterVec
	NotificationProcessingDuration *prometheus.HistogramVec
	QueueDepth                     *prometheus.GaugeVec
	ActiveWorkers                  *prometheus.GaugeVec
	RateLimitHits                  *prometheus.CounterVec
	HTTPRequestDuration            *prometheus.HistogramVec
	HTTPRequestsTotal              *prometheus.CounterVec
}

// New registers and returns all Prometheus metrics.
func New() *Metrics {
	return &Metrics{
		NotificationsSent: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "notification",
				Name:      "sent_total",
				Help:      "Total number of notifications successfully sent, partitioned by channel.",
			},
			[]string{"channel"},
		),
		NotificationsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "notification",
				Name:      "failed_total",
				Help:      "Total number of notifications that failed delivery, partitioned by channel.",
			},
			[]string{"channel"},
		),
		NotificationProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "notification",
				Name:      "processing_duration_seconds",
				Help:      "Time taken to process (send) a notification, partitioned by channel.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"channel"},
		),
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "notification",
				Name:      "queue_depth",
				Help:      "Current number of notifications in each queue partition.",
			},
			[]string{"channel", "priority"},
		),
		ActiveWorkers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "notification",
				Name:      "active_workers",
				Help:      "Number of currently active worker goroutines per channel.",
			},
			[]string{"channel"},
		),
		RateLimitHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "notification",
				Name:      "rate_limit_hits_total",
				Help:      "Total number of rate-limit rejections per channel.",
			},
			[]string{"channel"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request latency histogram.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests.",
			},
			[]string{"method", "path", "status"},
		),
	}
}
