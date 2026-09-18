-- +goose Up
-- global custom instructions: user-level defaults prepended to every run's
-- system prompt (the per-conversation prompt follows, so it wins ties).
ALTER TABLE "users" ADD COLUMN "default_instructions" text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE "users" DROP COLUMN "default_instructions";
