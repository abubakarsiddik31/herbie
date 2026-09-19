-- +goose Up
-- Model Context Protocol (MCP) servers registration
CREATE TABLE IF NOT EXISTS "mcp_servers" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "name" text NOT NULL,
  "url" text NOT NULL,
  "transport" text NOT NULL DEFAULT 'http',
  "enabled" boolean NOT NULL DEFAULT true,
  "headers" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "mcp_servers_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "mcp_servers_user_id_name_key" UNIQUE ("user_id", "name")
);

CREATE INDEX IF NOT EXISTS "idx_mcp_servers_user_enabled" ON "mcp_servers" ("user_id", "enabled");

-- +goose Down
DROP TABLE IF EXISTS "mcp_servers";
