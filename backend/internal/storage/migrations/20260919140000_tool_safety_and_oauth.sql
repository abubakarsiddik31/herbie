-- +goose Up
-- tool safety, encrypted credentials, oauth tool metadata, and audit logs

ALTER TABLE "workflows"
  ADD COLUMN IF NOT EXISTS "tool_require_approval" boolean NOT NULL DEFAULT true;

ALTER TABLE "workflow_credentials"
  ADD COLUMN IF NOT EXISTS "provider" text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS "scopes" text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS "expires_at" timestamptz NULL;

CREATE TABLE IF NOT EXISTS "tool_audit_logs" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "caller_type" text NOT NULL,
  "caller_id" text NOT NULL DEFAULT '',
  "tool_name" text NOT NULL,
  "action" text NOT NULL DEFAULT 'execute',
  "input_summary" text NOT NULL DEFAULT '',
  "output_summary" text NOT NULL DEFAULT '',
  "status" text NOT NULL DEFAULT 'success',
  "error" text NULL,
  "duration_ms" bigint NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "tool_audit_logs_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS "idx_tool_audit_logs_user" ON "tool_audit_logs" ("user_id", "created_at" DESC);

-- +goose Down
DROP TABLE IF EXISTS "tool_audit_logs";
ALTER TABLE "workflow_credentials"
  DROP COLUMN IF EXISTS "expires_at",
  DROP COLUMN IF EXISTS "scopes",
  DROP COLUMN IF EXISTS "provider";
ALTER TABLE "workflows"
  DROP COLUMN IF EXISTS "tool_require_approval";
