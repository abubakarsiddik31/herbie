-- +goose Up
-- modify "pending_tool_calls" table
ALTER TABLE "pending_tool_calls" DROP CONSTRAINT "pending_tool_calls_pkey", ADD COLUMN "id" uuid NOT NULL DEFAULT gen_random_uuid(), ADD PRIMARY KEY ("id");

-- +goose Down
-- reverse: modify "pending_tool_calls" table
ALTER TABLE "pending_tool_calls" DROP CONSTRAINT "pending_tool_calls_pkey", DROP COLUMN "id", ADD PRIMARY KEY ("call_id");
