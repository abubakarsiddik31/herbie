-- +goose Up
-- Chat organization: pin/archive flags on conversations plus explicit title
-- provenance so the first-message heuristic can never overwrite a user
-- rename. Existing titles are manual by definition (ADD's default stamps
-- them); new rows classify as heuristic.
ALTER TABLE "conversations" ADD COLUMN "pinned_at" timestamptz;
ALTER TABLE "conversations" ADD COLUMN "archived_at" timestamptz;
ALTER TABLE "conversations" ADD COLUMN "title_source" text NOT NULL DEFAULT 'manual';
ALTER TABLE "conversations" ALTER COLUMN "title_source" SET DEFAULT 'heuristic';

CREATE INDEX "idx_conversations_pinned"
  ON "conversations" ("user_id", "pinned_at" DESC)
  WHERE "pinned_at" IS NOT NULL;
CREATE INDEX "idx_conversations_archived"
  ON "conversations" ("user_id", "archived_at" DESC)
  WHERE "archived_at" IS NOT NULL;

-- Saved prompts are the user's reusable composer snippets. Ownership rides
-- user_id in every query; there is no sharing in v1.
CREATE TABLE "saved_prompts" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "title" text NOT NULL,
  "body" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "saved_prompts_user_id_fkey"
    FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);
CREATE INDEX "idx_saved_prompts_user" ON "saved_prompts" ("user_id", "updated_at" DESC);

-- +goose Down
DROP TABLE IF EXISTS "saved_prompts";
DROP INDEX IF EXISTS "idx_conversations_archived";
DROP INDEX IF EXISTS "idx_conversations_pinned";
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "title_source";
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "archived_at";
ALTER TABLE "conversations" DROP COLUMN IF EXISTS "pinned_at";
