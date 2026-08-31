-- +goose Up
-- modify "refresh_tokens" table
ALTER TABLE "refresh_tokens" DROP CONSTRAINT "refresh_tokens_token_hash_key";
-- create index "refresh_tokens_token_hash_key" to table: "refresh_tokens"
CREATE UNIQUE INDEX "refresh_tokens_token_hash_key" ON "refresh_tokens" ("token_hash");
-- modify "users" table
ALTER TABLE "users" DROP CONSTRAINT "users_email_key";
-- create index "users_email_key" to table: "users"
CREATE UNIQUE INDEX "users_email_key" ON "users" ("email");
-- create "pending_tool_calls" table
CREATE TABLE "pending_tool_calls" (
  "call_id" text NOT NULL,
  "conversation_id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "tool_name" text NOT NULL,
  "args" jsonb NOT NULL,
  "reason" text NOT NULL DEFAULT '',
  "status" text NOT NULL DEFAULT 'pending',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("call_id"),
  CONSTRAINT "pending_tool_calls_conversation_id_fkey" FOREIGN KEY ("conversation_id") REFERENCES "conversations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "pending_tool_calls_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "idx_pending_tool_calls_conversation" to table: "pending_tool_calls"
CREATE INDEX "idx_pending_tool_calls_conversation" ON "pending_tool_calls" ("conversation_id", "status");
-- create "tools" table
CREATE TABLE "tools" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "name" text NOT NULL,
  "description" text NOT NULL,
  "method" text NOT NULL,
  "url_template" text NOT NULL,
  "params" jsonb NOT NULL DEFAULT '[]',
  "body_template" text NOT NULL DEFAULT '',
  "headers" jsonb NOT NULL DEFAULT '{}',
  "require_approval" boolean NOT NULL DEFAULT false,
  "enabled" boolean NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "tools_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "tools_user_id_name_key" to table: "tools"
CREATE UNIQUE INDEX "tools_user_id_name_key" ON "tools" ("user_id", "name");

-- +goose Down
-- reverse: create index "tools_user_id_name_key" to table: "tools"
DROP INDEX "tools_user_id_name_key";
-- reverse: create "tools" table
DROP TABLE "tools";
-- reverse: create index "idx_pending_tool_calls_conversation" to table: "pending_tool_calls"
DROP INDEX "idx_pending_tool_calls_conversation";
-- reverse: create "pending_tool_calls" table
DROP TABLE "pending_tool_calls";
-- reverse: create index "users_email_key" to table: "users"
DROP INDEX "users_email_key";
-- reverse: modify "users" table
ALTER TABLE "users" ADD CONSTRAINT "users_email_key" UNIQUE ("email");
-- reverse: create index "refresh_tokens_token_hash_key" to table: "refresh_tokens"
DROP INDEX "refresh_tokens_token_hash_key";
-- reverse: modify "refresh_tokens" table
ALTER TABLE "refresh_tokens" ADD CONSTRAINT "refresh_tokens_token_hash_key" UNIQUE ("token_hash");
