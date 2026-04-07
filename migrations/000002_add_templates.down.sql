ALTER TABLE notifications DROP CONSTRAINT IF EXISTS fk_notifications_template;
DROP TRIGGER IF EXISTS update_templates_updated_at ON templates;
DROP TABLE IF EXISTS templates;
