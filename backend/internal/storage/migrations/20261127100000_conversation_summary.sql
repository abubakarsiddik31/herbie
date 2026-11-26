-- +goose Up
-- A durable, editable conversation summary. summary_through_message_id is the
-- last active-path message it replaces; NULL means view-only (not injected).
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary text NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary_updated_at timestamptz;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary_through_message_id uuid;

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_through_message_id;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_updated_at;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary;
