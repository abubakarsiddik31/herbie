-- +goose Up
-- modify "conversations" table
ALTER TABLE "conversations" ADD COLUMN "model" text NOT NULL DEFAULT '', ADD COLUMN "temperature" real NULL, ADD COLUMN "system_prompt" text NOT NULL DEFAULT '';
-- modify "messages" table
ALTER TABLE "messages" ADD COLUMN "model" text NOT NULL DEFAULT '';

-- +goose Down
-- reverse: modify "messages" table
ALTER TABLE "messages" DROP COLUMN "model";
-- reverse: modify "conversations" table
ALTER TABLE "conversations" DROP COLUMN "system_prompt", DROP COLUMN "temperature", DROP COLUMN "model";
