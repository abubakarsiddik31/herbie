-- +goose Up
-- A durable, editable conversation summary. summary_covers counts messages
-- from the start of the active path replaced by this summary; 0 means view
-- only (not injected into runs).
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary text NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary_updated_at timestamptz;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS summary_covers int NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_covers;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary_updated_at;
ALTER TABLE conversations DROP COLUMN IF EXISTS summary;
