-- +goose Up
-- Preserve the curated dropdown order; created_at ties made id order the
-- default model after the catalog moved into Postgres.
ALTER TABLE "model_catalog" ADD COLUMN "sort_order" integer NOT NULL DEFAULT 0;
UPDATE "model_catalog" SET "sort_order" = v.ord
FROM (VALUES
  ('gemini-3.5-flash', 1),
  ('gemini-3.5-pro', 2),
  ('gemma-3-27b-it', 3),
  ('gemma-3-12b-it', 4),
  ('gpt-5', 5),
  ('gpt-4.1', 6),
  ('claude-sonnet-4-5', 7),
  ('claude-haiku-4-5', 8),
  ('gemini-2.5-flash', 9),
  ('gemini-2.5-flash-lite', 10),
  ('gemini-2.5-pro', 11)
) AS v(id, ord) WHERE "model_catalog"."id" = v.id;

-- +goose Down
ALTER TABLE "model_catalog" DROP COLUMN IF EXISTS "sort_order";
