# Notification System

A production-quality, event-driven notification service built in Go using hexagonal (ports & adapters) architecture. Supports multi-channel delivery (SMS, Email, Push), async worker pools, scheduled notifications, message templates, idempotency, rate limiting, real-time WebSocket updates, and full observability via Prometheus metrics.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP Clients                         │
└──────────────────────────┬──────────────────────────────────┘
                           │ REST API / WebSocket
┌──────────────────────────▼──────────────────────────────────┐
│                   Adapters: HTTP (Gin)                       │
│          handler.go  │  router.go  │  middleware.go          │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│              Application Service Layer                       │
│         internal/application/notification/service.go        │
│    CreateNotification │ CreateBatch │ ProcessNotification    │
│    CancelNotification │ ListNotifications │ GetMetrics       │
└──────────┬────────────┬────────────┬────────────────────────┘
           │            │            │
    ┌──────▼──────┐ ┌───▼────┐ ┌────▼─────┐
    │  Repository │ │ Queue  │ │ Provider │  <- Domain Ports
    │  (Port)     │ │ (Port) │ │ (Port)   │
    └──────┬──────┘ └───┬────┘ └────┬─────┘
           │            │            │
    ┌──────▼──────┐ ┌───▼────┐ ┌────▼──────────┐
    │ PostgreSQL  │ │ Redis  │ │ Webhook.site  │  <- Adapters
    │  Adapter    │ │ Queue  │ │   Provider    │
    └─────────────┘ └────────┘ └───────────────┘

┌──────────────────────────────────────────────────────────────┐
│                      Worker Layer                            │
│  processor.go (N goroutines/channel) │ scheduler.go (30s)   │
└──────────────────────────────────────────────────────────────┘
```

### Design Decisions

- **Hexagonal Architecture**: Domain logic is isolated from infrastructure concerns. All I/O happens through port interfaces, making the system fully testable without real infrastructure.
- **Priority Queue**: Redis sorted sets provide FIFO ordering within each priority tier (high > normal > low) with O(log N) operations.
- **Idempotency**: Unique idempotency keys prevent duplicate delivery on network retries.
- **Exponential Backoff**: Failed deliveries are retried with `2^retryCount * baseDelay` spacing to avoid thundering herd.
- **Sliding Window Rate Limiting**: Lua scripts ensure atomic increment+TTL operations, preventing race conditions under concurrent load.
- **Template Rendering**: Go's `text/template` engine handles `{{.VarName}}` substitution for reusable message content.

## Prerequisites

- Docker + Docker Compose (for the full stack)
- Go 1.21+ (for local development)
- `golang-migrate` CLI (for manual migrations)

## Quick Start

```bash
# Clone and start all services
git clone <repo-url>
cd notification-system

# Copy environment config
cp .env.example .env

# Start with Docker Compose (builds, migrates, runs)
docker-compose up --build
```

The API will be available at `http://localhost:8080`.

## Local Development

```bash
# Install dependencies
go mod download

# Start infrastructure
docker-compose up -d postgres redis

# Run database migrations
export DATABASE_DSN="postgres://postgres:postgres@localhost:5432/notifications?sslmode=disable"
make migrate-up

# Start the service
make run
```

## API Documentation

Interactive Swagger UI: `http://localhost:8080/swagger/index.html`

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/notifications` | Create single notification |
| POST | `/api/v1/notifications/batch` | Create batch (up to 1000) |
| GET | `/api/v1/notifications` | List with filtering + pagination |
| GET | `/api/v1/notifications/:id` | Get by ID |
| DELETE | `/api/v1/notifications/:id` | Cancel notification |
| GET | `/api/v1/notifications/batch/:batchId` | Get batch |
| POST | `/api/v1/templates` | Create message template |
| GET | `/api/v1/templates/:id` | Get template |
| GET | `/api/v1/metrics` | Application metrics |
| GET | `/metrics` | Prometheus metrics |
| GET | `/health` | Health check |
| GET | `/ws` | WebSocket for real-time updates |

### curl Examples

**Create a notification:**
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "user@example.com",
    "channel": "email",
    "content": "Hello! Your order has shipped.",
    "priority": "high",
    "idempotency_key": "order-123-shipped"
  }'
```

**Create a batch:**
```bash
curl -X POST http://localhost:8080/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d '{
    "notifications": [
      {"recipient": "+15551234567", "channel": "sms", "content": "Sale ends tonight!"},
      {"recipient": "user@example.com", "channel": "email", "content": "Sale ends tonight!"}
    ]
  }'
```

**Schedule a notification:**
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "user@example.com",
    "channel": "push",
    "content": "Your weekly summary is ready.",
    "scheduled_at": "2026-04-20T09:00:00Z"
  }'
```

**Create and use a template:**
```bash
# Create template
curl -X POST http://localhost:8080/api/v1/templates \
  -H "Content-Type: application/json" \
  -d '{"name": "welcome", "channel": "email", "content": "Hello {{.Name}}, welcome to Insider!"}'

# Use template (replace <template-id> with the ID from above response)
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "new@example.com",
    "channel": "email",
    "template_id": "<template-id>",
    "template_vars": {"Name": "Alice"}
  }'
```

**List with filters:**
```bash
curl "http://localhost:8080/api/v1/notifications?status=sent&channel=email&page=1&page_size=20"
```

**Cancel a notification:**
```bash
curl -X DELETE http://localhost:8080/api/v1/notifications/<id>
```

**WebSocket real-time updates:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
ws.onmessage = (event) => {
  const notification = JSON.parse(event.data);
  console.log('Status update:', notification);
};
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP server port |
| `DATABASE_DSN` | `postgres://...` | PostgreSQL connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis server address |
| `REDIS_PASSWORD` | `` | Redis password (empty = no auth) |
| `REDIS_DB` | `0` | Redis database number |
| `WEBHOOK_URL` | `https://webhook.site/...` | Webhook delivery endpoint |
| `WEBHOOK_TIMEOUT` | `10s` | Webhook HTTP client timeout |
| `WORKERS_PER_CHANNEL` | `5` | Worker goroutines per channel |
| `MAX_RETRIES` | `3` | Maximum delivery retries |
| `RETRY_BASE_DELAY` | `1s` | Base delay for exponential backoff |
| `ENV` | `` | Set to `development` for debug logging |

## Running Tests

```bash
# All tests with race detection and coverage
make test

# View coverage report
open coverage.html

# Run specific package tests
go test -v ./internal/application/notification/...
go test -v ./internal/domain/notification/...
go test -v ./internal/adapters/http/...
```

## Channel Content Limits

| Channel | Max Length |
|---------|-----------|
| SMS | 160 characters |
| Email | 10,000 characters |
| Push | 256 characters |

## Notification Lifecycle

```
pending -> queued -> processing -> sent
                               \-> failed (retry if count < max_retries)
                                         \-> failed (permanently)
pending/queued/scheduled -> cancelled
```

## Metrics

Prometheus metrics exposed at `/metrics`:

- `notification_sent_total{channel}` - successful deliveries
- `notification_failed_total{channel}` - failed deliveries
- `notification_processing_duration_seconds{channel}` - delivery latency histogram
- `notification_queue_depth{channel,priority}` - current queue sizes
- `notification_active_workers{channel}` - active worker goroutines
- `notification_rate_limit_hits_total{channel}` - rate limit rejections
- `http_request_duration_seconds{method,path,status}` - HTTP latency
- `http_requests_total{method,path,status}` - HTTP request count

Grafana dashboard: Import the Prometheus datasource pointing to `http://prometheus:9090`.
