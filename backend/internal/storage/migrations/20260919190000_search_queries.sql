-- +goose Up
CREATE TABLE IF NOT EXISTS "search_queries" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "conversation_id" uuid NULL,
  "query" text NOT NULL,
  "kind" text NOT NULL DEFAULT 'web_search',
  "provider" text NOT NULL DEFAULT '',
  "results_count" integer NOT NULL DEFAULT 0,
  "duration_ms" bigint NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "search_queries_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "search_queries_conversation_id_fkey" FOREIGN KEY ("conversation_id") REFERENCES "conversations" ("id") ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS "idx_search_queries_user_time" ON "search_queries" ("user_id", "created_at" DESC);
CREATE INDEX IF NOT EXISTS "idx_search_queries_user_kind" ON "search_queries" ("user_id", "kind", "created_at" DESC);

-- +goose Down
DROP TABLE IF EXISTS "search_queries";
