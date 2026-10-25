-- +goose Up
CREATE TABLE bot_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'queued',
  task text NOT NULL DEFAULT '',
  trigger_source text NOT NULL DEFAULT 'manual',
  error text,
  activity jsonb NOT NULL DEFAULT '[]',
  input_tokens int NOT NULL DEFAULT 0,
  output_tokens int NOT NULL DEFAULT 0,
  requests int NOT NULL DEFAULT 0,
  cost_micro_usd bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz
);
CREATE INDEX idx_bot_runs_bot ON bot_runs (bot_id, created_at DESC);
CREATE INDEX idx_bot_runs_active ON bot_runs (status) WHERE status IN ('queued','running','waiting_approval');

-- +goose Down
DROP INDEX idx_bot_runs_active;
DROP INDEX idx_bot_runs_bot;
DROP TABLE bot_runs;
