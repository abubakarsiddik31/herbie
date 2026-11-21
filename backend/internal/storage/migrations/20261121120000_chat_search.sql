-- +goose Up
-- Full-text message search. The vector indexes only the first 100,000
-- characters of content, so a 1,000,000-character prompt cannot produce an
-- unbounded GIN entry; the trigger keeps INSERT/UPDATE cost proportional to
-- the stored text and avoids a generated-column rewrite of messages.
-- The backfill UPDATE rewrites every row once, and CREATE INDEX takes a SHARE
-- lock on messages while it builds: on a large install run this migration in
-- a maintenance window.
ALTER TABLE "messages" ADD COLUMN "search_vector" tsvector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION "messages_search_vector"() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW."search_vector" := to_tsvector('simple', left(coalesce(NEW."content", ''), 100000));
  RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER "messages_search_vector_trigger"
  BEFORE INSERT OR UPDATE OF "content" ON "messages"
  FOR EACH ROW EXECUTE FUNCTION "messages_search_vector"();

UPDATE "messages"
SET "search_vector" = to_tsvector('simple', left(coalesce("content", ''), 100000));

ALTER TABLE "messages" ALTER COLUMN "search_vector" SET NOT NULL;

CREATE INDEX "idx_messages_search"
  ON "messages" USING GIN ("search_vector");

-- +goose Down
DROP INDEX IF EXISTS "idx_messages_search";
ALTER TABLE "messages" ALTER COLUMN "search_vector" DROP NOT NULL;
DROP TRIGGER IF EXISTS "messages_search_vector_trigger" ON "messages";
DROP FUNCTION IF EXISTS "messages_search_vector"();
ALTER TABLE "messages" DROP COLUMN IF EXISTS "search_vector";
