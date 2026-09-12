-- +goose Up
-- create "documents" table
CREATE TABLE "documents" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "object_key" text NOT NULL,
  "filename" text NOT NULL,
  "mime" text NOT NULL,
  "size_bytes" bigint NOT NULL,
  "status" text NOT NULL DEFAULT 'processing',
  "error" text NULL,
  "chunk_count" integer NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "documents_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- create index "idx_documents_user" to table: "documents"
CREATE INDEX "idx_documents_user" ON "documents" ("user_id", "created_at");
-- modify "usage_events" table
ALTER TABLE "usage_events" ADD COLUMN "document_id" uuid NULL, ADD CONSTRAINT "usage_events_document_id_fkey" FOREIGN KEY ("document_id") REFERENCES "documents" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;

-- +goose Down
-- reverse: modify "usage_events" table
ALTER TABLE "usage_events" DROP CONSTRAINT "usage_events_document_id_fkey", DROP COLUMN "document_id";
-- reverse: create index "idx_documents_user" to table: "documents"
DROP INDEX "idx_documents_user";
-- reverse: create "documents" table
DROP TABLE "documents";
