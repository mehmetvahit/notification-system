package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// Repository implements domain.Repository using PostgreSQL via pgx/v5.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new PostgreSQL repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// NewPool establishes a connection pool from a DSN.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Create inserts a new notification.
func (r *Repository) Create(ctx context.Context, n *domain.Notification) error {
	varsJSON, err := json.Marshal(n.TemplateVars)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO notifications (
			id, batch_id, recipient, channel, content, status, priority,
			idempotency_key, template_id, template_vars, scheduled_at,
			retry_count, max_retries, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			NULLIF($9, ''), NULLIF($10::text, '')::uuid, $11, $12, $13,
			$14, $15
		)`

	var batchID *string
	if n.BatchID != "" {
		batchID = &n.BatchID
	}
	var templateID *string
	if n.TemplateID != "" {
		templateID = &n.TemplateID
	}
	var idempotencyKey *string
	if n.IdempotencyKey != "" {
		idempotencyKey = &n.IdempotencyKey
	}

	_, err = r.pool.Exec(ctx, query,
		n.ID, batchID, n.Recipient, string(n.Channel), n.Content,
		string(n.Status), string(n.Priority), idempotencyKey, idempotencyKey,
		templateID, n.ScheduledAt, n.RetryCount, n.MaxRetries,
		n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	_ = varsJSON
	return nil
}

// CreateBatch inserts multiple notifications in a single transaction.
func (r *Repository) CreateBatch(ctx context.Context, notifications []*domain.Notification) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, n := range notifications {
		var batchID *string
		if n.BatchID != "" {
			batchID = &n.BatchID
		}
		var templateID *string
		if n.TemplateID != "" {
			templateID = &n.TemplateID
		}
		var idempotencyKey *string
		if n.IdempotencyKey != "" {
			idempotencyKey = &n.IdempotencyKey
		}

		query := `
			INSERT INTO notifications (
				id, batch_id, recipient, channel, content, status, priority,
				idempotency_key, template_id, scheduled_at,
				retry_count, max_retries, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				NULLIF($9::text, '')::uuid, $10, $11, $12, $13, $14
			)`

		_, err = tx.Exec(ctx, query,
			n.ID, batchID, n.Recipient, string(n.Channel), n.Content,
			string(n.Status), string(n.Priority), idempotencyKey,
			templateID, n.ScheduledAt, n.RetryCount, n.MaxRetries,
			n.CreatedAt, n.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("batch insert notification %s: %w", n.ID, err)
		}
	}

	return tx.Commit(ctx)
}

// GetByID retrieves a notification by primary key.
func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	query := `
		SELECT id, COALESCE(batch_id::text, ''), recipient, channel, content,
		       status, priority, COALESCE(idempotency_key, ''),
		       COALESCE(template_id::text, ''), template_vars,
		       scheduled_at, sent_at, retry_count, max_retries,
		       COALESCE(provider_msg_id, ''), COALESCE(error_msg, ''),
		       created_at, updated_at
		FROM notifications WHERE id = $1`

	row := r.pool.QueryRow(ctx, query, id)
	n, err := scanNotification(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get notification by id: %w", err)
	}
	return n, nil
}

// GetByBatchID retrieves all notifications belonging to a batch.
func (r *Repository) GetByBatchID(ctx context.Context, batchID string) ([]*domain.Notification, error) {
	query := `
		SELECT id, COALESCE(batch_id::text, ''), recipient, channel, content,
		       status, priority, COALESCE(idempotency_key, ''),
		       COALESCE(template_id::text, ''), template_vars,
		       scheduled_at, sent_at, retry_count, max_retries,
		       COALESCE(provider_msg_id, ''), COALESCE(error_msg, ''),
		       created_at, updated_at
		FROM notifications WHERE batch_id = $1 ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	defer rows.Close()

	return collectRows(rows)
}

// Update persists changes to an existing notification.
func (r *Repository) Update(ctx context.Context, n *domain.Notification) error {
	query := `
		UPDATE notifications SET
			status = $2, retry_count = $3, provider_msg_id = NULLIF($4, ''),
			error_msg = NULLIF($5, ''), sent_at = $6, updated_at = $7
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, query,
		n.ID, string(n.Status), n.RetryCount,
		n.ProviderMsgID, n.ErrorMsg, n.SentAt, time.Now(),
	)
	return err
}

// List returns a paginated, filtered list of notifications.
func (r *Repository) List(ctx context.Context, filter domain.ListFilter) ([]*domain.Notification, int64, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	argIdx := 1

	if filter.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, string(*filter.Status))
		argIdx++
	}
	if filter.Channel != nil {
		where += fmt.Sprintf(" AND channel = $%d", argIdx)
		args = append(args, string(*filter.Channel))
		argIdx++
	}
	if filter.BatchID != nil {
		where += fmt.Sprintf(" AND batch_id = $%d", argIdx)
		args = append(args, *filter.BatchID)
		argIdx++
	}
	if filter.StartDate != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *filter.StartDate)
		argIdx++
	}
	if filter.EndDate != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *filter.EndDate)
		argIdx++
	}

	// Count query.
	countQuery := "SELECT COUNT(*) FROM notifications " + where
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	args = append(args, filter.PageSize, offset)

	dataQuery := fmt.Sprintf(`
		SELECT id, COALESCE(batch_id::text, ''), recipient, channel, content,
		       status, priority, COALESCE(idempotency_key, ''),
		       COALESCE(template_id::text, ''), template_vars,
		       scheduled_at, sent_at, retry_count, max_retries,
		       COALESCE(provider_msg_id, ''), COALESCE(error_msg, ''),
		       created_at, updated_at
		FROM notifications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications, err := collectRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return notifications, total, nil
}

// GetByIdempotencyKey finds a notification by its idempotency key.
func (r *Repository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Notification, error) {
	query := `
		SELECT id, COALESCE(batch_id::text, ''), recipient, channel, content,
		       status, priority, COALESCE(idempotency_key, ''),
		       COALESCE(template_id::text, ''), template_vars,
		       scheduled_at, sent_at, retry_count, max_retries,
		       COALESCE(provider_msg_id, ''), COALESCE(error_msg, ''),
		       created_at, updated_at
		FROM notifications WHERE idempotency_key = $1`

	row := r.pool.QueryRow(ctx, query, key)
	n, err := scanNotification(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get by idempotency key: %w", err)
	}
	return n, nil
}

// GetPendingScheduled returns all scheduled notifications due before `before`.
func (r *Repository) GetPendingScheduled(ctx context.Context, before time.Time) ([]*domain.Notification, error) {
	query := `
		SELECT id, COALESCE(batch_id::text, ''), recipient, channel, content,
		       status, priority, COALESCE(idempotency_key, ''),
		       COALESCE(template_id::text, ''), template_vars,
		       scheduled_at, sent_at, retry_count, max_retries,
		       COALESCE(provider_msg_id, ''), COALESCE(error_msg, ''),
		       created_at, updated_at
		FROM notifications
		WHERE status = 'scheduled' AND scheduled_at <= $1
		ORDER BY scheduled_at ASC`

	rows, err := r.pool.Query(ctx, query, before)
	if err != nil {
		return nil, fmt.Errorf("get pending scheduled: %w", err)
	}
	defer rows.Close()

	return collectRows(rows)
}

// GetTemplateByID retrieves a template by its primary key.
func (r *Repository) GetTemplateByID(ctx context.Context, id string) (*domain.Template, error) {
	query := `SELECT id, name, channel, content, created_at, updated_at FROM templates WHERE id = $1`
	row := r.pool.QueryRow(ctx, query, id)

	var t domain.Template
	var ch string
	err := row.Scan(&t.ID, &t.Name, &ch, &t.Content, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTemplateNotFound
		}
		return nil, fmt.Errorf("get template: %w", err)
	}
	t.Channel = domain.Channel(ch)
	return &t, nil
}

// CreateTemplate inserts a new message template.
func (r *Repository) CreateTemplate(ctx context.Context, t *domain.Template) error {
	query := `
		INSERT INTO templates (id, name, channel, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, query,
		t.ID, t.Name, string(t.Channel), t.Content, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// --- scan helpers ---

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNotification(row rowScanner) (*domain.Notification, error) {
	var n domain.Notification
	var ch, status, priority string
	var templateVarsRaw []byte

	err := row.Scan(
		&n.ID, &n.BatchID, &n.Recipient, &ch, &n.Content,
		&status, &priority, &n.IdempotencyKey,
		&n.TemplateID, &templateVarsRaw,
		&n.ScheduledAt, &n.SentAt, &n.RetryCount, &n.MaxRetries,
		&n.ProviderMsgID, &n.ErrorMsg,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	n.Channel = domain.Channel(ch)
	n.Status = domain.Status(status)
	n.Priority = domain.Priority(priority)

	if len(templateVarsRaw) > 0 {
		_ = json.Unmarshal(templateVarsRaw, &n.TemplateVars)
	}

	return &n, nil
}

func collectRows(rows pgx.Rows) ([]*domain.Notification, error) {
	var notifications []*domain.Notification
	for rows.Next() {
		var n domain.Notification
		var ch, status, priority string
		var templateVarsRaw []byte

		err := rows.Scan(
			&n.ID, &n.BatchID, &n.Recipient, &ch, &n.Content,
			&status, &priority, &n.IdempotencyKey,
			&n.TemplateID, &templateVarsRaw,
			&n.ScheduledAt, &n.SentAt, &n.RetryCount, &n.MaxRetries,
			&n.ProviderMsgID, &n.ErrorMsg,
			&n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan notification row: %w", err)
		}

		n.Channel = domain.Channel(ch)
		n.Status = domain.Status(status)
		n.Priority = domain.Priority(priority)

		if len(templateVarsRaw) > 0 {
			_ = json.Unmarshal(templateVarsRaw, &n.TemplateVars)
		}

		notifications = append(notifications, &n)
	}
	return notifications, rows.Err()
}
