-- +goose Up
-- Provider-agnostic generation controls. NULL/zero/empty means "provider
-- default" so existing rows keep today's behavior. Conversations carry the
-- per-chat values; bots their own; users and projects provide fallbacks.
ALTER TABLE "conversations" ADD COLUMN "top_p" double precision;
ALTER TABLE "conversations" ADD COLUMN "max_output_tokens" integer NOT NULL DEFAULT 0;
ALTER TABLE "conversations" ADD COLUMN "reasoning" text NOT NULL DEFAULT '';

ALTER TABLE "bots" ADD COLUMN "top_p" double precision;
ALTER TABLE "bots" ADD COLUMN "max_output_tokens" integer NOT NULL DEFAULT 0;
ALTER TABLE "bots" ADD COLUMN "reasoning" text NOT NULL DEFAULT '';

ALTER TABLE "users" ADD COLUMN "default_top_p" double precision;
ALTER TABLE "users" ADD COLUMN "default_max_output_tokens" integer NOT NULL DEFAULT 0;
ALTER TABLE "users" ADD COLUMN "default_reasoning" text NOT NULL DEFAULT '';

ALTER TABLE "projects" ADD COLUMN "default_top_p" double precision;
ALTER TABLE "projects" ADD COLUMN "default_max_output_tokens" integer NOT NULL DEFAULT 0;
ALTER TABLE "projects" ADD COLUMN "default_reasoning" text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE "projects" DROP COLUMN IF EXISTS "default_reasoning";
ALTER TABLE "projects" DROP COLUMN IF EXISTS "default_max_output_tokens";
ALTER TABLE "projects" DROP COLUMN IF EXISTS "default_top_p";

ALTER TABLE "users" DROP COLUMN IF EXISTS "default_reasoning";
ALTER TABLE "users" DROP COLUMN IF EXISTS "default_max_output_tokens";
ALTER TABLE "users" DROP COLUMN IF EXISTS "default_top_p";

ALTER TABLE "bots" DROP COLUMN IF EXISTS "reasoning";
ALTER TABLE "bots" DROP COLUMN IF EXISTS "max_output_tokens";
ALTER TABLE "bots" DROP COLUMN IF EXISTS "top_p";

ALTER TABLE "conversations" DROP COLUMN IF EXISTS "reasoning";
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "max_output_tokens";
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "top_p";
