CREATE TABLE templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    channel notification_channel NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_templates_updated_at
    BEFORE UPDATE ON templates
    FOR EACH ROW
    EXECUTE PROCEDURE update_updated_at_column();

ALTER TABLE notifications ADD CONSTRAINT fk_notifications_template
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE SET NULL;
