-- +goose Up
-- Client-generated idempotency key for retried sends. Unique per
-- conversation; NULL for messages that never carried one.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS client_message_id text;
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_client_id
  ON messages (conversation_id, client_message_id)
  WHERE client_message_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_messages_client_id;
ALTER TABLE messages DROP COLUMN IF EXISTS client_message_id;
