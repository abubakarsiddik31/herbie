-- +goose Up
CREATE TABLE bots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  emoji text NOT NULL DEFAULT '🤖',
  color text NOT NULL DEFAULT 'sage',
  instructions text NOT NULL DEFAULT '',
  model text NOT NULL DEFAULT '',
  temperature real,
  rag_enabled boolean NOT NULL DEFAULT true,
  is_archived boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_bots_user ON bots (user_id, updated_at DESC);

ALTER TABLE conversations ADD COLUMN bot_id uuid REFERENCES bots(id) ON DELETE SET NULL;
CREATE INDEX idx_conversations_bot ON conversations (bot_id);

CREATE TABLE bot_memories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_bot_memories_bot ON bot_memories (bot_id, created_at DESC);

-- +goose Down
DROP TABLE bot_memories;
DROP INDEX idx_conversations_bot;
ALTER TABLE conversations DROP COLUMN bot_id;
DROP INDEX idx_bots_user;
DROP TABLE bots;
