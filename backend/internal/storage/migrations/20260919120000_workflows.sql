-- +goose Up
-- workflows: visual automation pipelines like n8n
CREATE TABLE "workflows" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "name" text NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "trigger_type" text NOT NULL DEFAULT 'manual',
  "webhook_slug" text NULL,
  "webhook_secret" text NOT NULL DEFAULT '',
  "nodes" jsonb NOT NULL DEFAULT '[]'::jsonb,
  "edges" jsonb NOT NULL DEFAULT '[]'::jsonb,
  "expose_as_tool" boolean NOT NULL DEFAULT false,
  "tool_name" text NOT NULL DEFAULT '',
  "tool_description" text NOT NULL DEFAULT '',
  "is_active" boolean NOT NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "workflows_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "workflows_webhook_slug_key" UNIQUE ("webhook_slug")
);
CREATE INDEX "idx_workflows_user" ON "workflows" ("user_id", "updated_at" DESC);
CREATE INDEX "idx_workflows_active_tool" ON "workflows" ("user_id", "is_active") WHERE "expose_as_tool" = true;

CREATE TABLE "workflow_runs" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "workflow_id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "status" text NOT NULL DEFAULT 'pending',
  "trigger_source" text NOT NULL DEFAULT 'manual',
  "input_data" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "output_data" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "node_results" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "error" text NULL,
  "duration_ms" bigint NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "finished_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "workflow_runs_workflow_id_fkey" FOREIGN KEY ("workflow_id") REFERENCES "workflows" ("id") ON DELETE CASCADE,
  CONSTRAINT "workflow_runs_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
CREATE INDEX "idx_workflow_runs_workflow" ON "workflow_runs" ("workflow_id", "created_at" DESC);
CREATE INDEX "idx_workflow_runs_user" ON "workflow_runs" ("user_id", "created_at" DESC);

CREATE TABLE "workflow_credentials" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "name" text NOT NULL,
  "type" text NOT NULL,
  "data" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "workflow_credentials_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE,
  CONSTRAINT "workflow_credentials_user_name_key" UNIQUE ("user_id", "name")
);
CREATE INDEX "idx_workflow_credentials_user" ON "workflow_credentials" ("user_id", "name");

-- +goose Down
DROP TABLE IF EXISTS "workflow_credentials";
DROP TABLE IF EXISTS "workflow_runs";
DROP TABLE IF EXISTS "workflows";
