-- +goose Up
CREATE TABLE bot_routines (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  schedule text NOT NULL,
  timezone text NOT NULL DEFAULT 'UTC',
  prompt text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  last_run_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_bot_routines_bot ON bot_routines (bot_id, created_at DESC);
CREATE INDEX idx_bot_routines_enabled ON bot_routines (enabled) WHERE enabled = true;

-- +goose Down
DROP INDEX idx_bot_routines_enabled;
DROP INDEX idx_bot_routines_bot;
DROP TABLE bot_routines;
