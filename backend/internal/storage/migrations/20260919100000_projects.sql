-- +goose Up
-- projects: workspaces with dedicated files and project-scoped chats
CREATE TABLE "projects" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "name" text NOT NULL,
  "description" text NOT NULL DEFAULT '',
  "instructions" text NOT NULL DEFAULT '',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "projects_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
CREATE INDEX "idx_projects_user" ON "projects" ("user_id", "updated_at" DESC);

ALTER TABLE "documents" ADD COLUMN IF NOT EXISTS "project_id" uuid NULL REFERENCES "projects" ("id") ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS "idx_documents_project" ON "documents" ("project_id");

ALTER TABLE "conversations" ADD COLUMN IF NOT EXISTS "project_id" uuid NULL REFERENCES "projects" ("id") ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS "idx_conversations_project" ON "conversations" ("project_id");

-- +goose Down
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "project_id";
ALTER TABLE "documents" DROP COLUMN IF EXISTS "project_id";
DROP TABLE IF EXISTS "projects";
