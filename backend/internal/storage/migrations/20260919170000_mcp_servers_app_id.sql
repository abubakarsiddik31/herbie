-- +goose Up
ALTER TABLE "mcp_servers" ADD COLUMN IF NOT EXISTS "app_id" text;
CREATE INDEX IF NOT EXISTS "idx_mcp_servers_user_app" ON "mcp_servers" ("user_id", "app_id");

-- +goose Down
DROP INDEX IF EXISTS "idx_mcp_servers_user_app";
ALTER TABLE "mcp_servers" DROP COLUMN IF EXISTS "app_id";
