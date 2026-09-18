-- +goose Up
-- cross-chat memory: user memories retained across conversations
CREATE TABLE "user_memories" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "content" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "user_memories_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
CREATE INDEX "idx_user_memories_user" ON "user_memories" ("user_id", "created_at");

-- +goose Down
DROP TABLE "user_memories";
