-- +goose Up
-- read-only public links: one active share per conversation, looked up by
-- token hash. Rows die with their conversation.
CREATE TABLE "conversation_shares" (
  "conversation_id" uuid PRIMARY KEY REFERENCES "conversations" ("id") ON DELETE CASCADE,
  "token_hash" text NOT NULL UNIQUE,
  "created_at" timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE "conversation_shares";
