CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE notification_channel AS ENUM ('sms', 'email', 'push');
CREATE TYPE notification_status AS ENUM ('pending', 'queued', 'processing', 'sent', 'failed', 'cancelled', 'scheduled');
CREATE TYPE notification_priority AS ENUM ('high', 'normal', 'low');

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    batch_id UUID,
    recipient VARCHAR(255) NOT NULL,
    channel notification_channel NOT NULL,
    content TEXT NOT NULL,
    status notification_status NOT NULL DEFAULT 'pending',
    priority notification_priority NOT NULL DEFAULT 'normal',
    idempotency_key VARCHAR(255) UNIQUE,
    template_id UUID,
    template_vars JSONB,
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    provider_msg_id VARCHAR(255),
    error_msg TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_batch_id ON notifications(batch_id);
CREATE INDEX idx_notifications_status ON notifications(status);
CREATE INDEX idx_notifications_channel ON notifications(channel);
CREATE INDEX idx_notifications_created_at ON notifications(created_at);
CREATE INDEX idx_notifications_scheduled_at ON notifications(scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX idx_notifications_idempotency_key ON notifications(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_notifications_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW
    EXECUTE PROCEDURE update_updated_at_column();
