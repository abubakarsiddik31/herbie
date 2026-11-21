-- +goose Up
-- Reply-to quotes and per-message feedback.
--
-- quote_text is a server snapshot of the quoted row's raw content, capped at
-- the handler's limit so later edits/deletes cannot change what the model saw.
-- reply_to_id keeps the link for UI navigation; deleting the quoted row clears
-- the pointer but leaves the snapshot. messages.content stays the raw,
-- user-visible text; the model-facing composition happens in the handler and
-- is persisted in messages.data for history replay.
ALTER TABLE "messages" ADD COLUMN "reply_to_id" uuid;
ALTER TABLE "messages" ADD COLUMN "quote_text" text;

ALTER TABLE "messages"
  ADD CONSTRAINT "messages_reply_to_id_fkey"
  FOREIGN KEY ("reply_to_id") REFERENCES "messages" ("id") ON DELETE SET NULL;
CREATE INDEX "idx_messages_reply_to"
  ON "messages" ("reply_to_id")
  WHERE "reply_to_id" IS NOT NULL;

-- One rating per user and message; assistant rows only (enforced by the
-- handler). Ownership rides user_id in every query; shared transcripts never
-- read this table.
CREATE TABLE "message_feedback" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "message_id" uuid NOT NULL,
  "rating" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "message_feedback_rating_check" CHECK ("rating" IN ('up', 'down')),
  CONSTRAINT "message_feedback_user_id_fkey"
    FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "message_feedback_message_id_fkey"
    FOREIGN KEY ("message_id") REFERENCES "messages" ("id") ON DELETE CASCADE
);
CREATE UNIQUE INDEX "message_feedback_user_message_key"
  ON "message_feedback" ("user_id", "message_id");
CREATE INDEX "idx_message_feedback_message"
  ON "message_feedback" ("message_id");

-- +goose Down
DROP TABLE IF EXISTS "message_feedback";
DROP INDEX IF EXISTS "idx_messages_reply_to";
ALTER TABLE "messages" DROP CONSTRAINT IF EXISTS "messages_reply_to_id_fkey";
ALTER TABLE "messages" DROP COLUMN IF EXISTS "quote_text";
ALTER TABLE "messages" DROP COLUMN IF EXISTS "reply_to_id";
