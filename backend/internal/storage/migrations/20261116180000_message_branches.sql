-- +goose Up
-- Message tree: non-destructive edit/regenerate branches. parent_id chains
-- every stored row (prompt -> tool calls -> results -> answer);
-- conversations.active_leaf_id selects the visible path. Existing rows
-- backfill as one linear chain per conversation in (created_at, id) order —
-- the same order every read path uses today, so microsecond ties broken by
-- random UUIDs keep today's display order (accepted, not a regression).
-- Hand-written: schema.hcl drifted behind the migrations (mcp_servers,
-- project_id); an atlas diff would emit unrelated drops.

ALTER TABLE "messages" ADD COLUMN "parent_id" uuid;
ALTER TABLE "conversations" ADD COLUMN "active_leaf_id" uuid;
ALTER TABLE "pending_tool_calls" ADD COLUMN "message_id" uuid;

WITH "ordered" AS (
  SELECT "id",
         lag("id") OVER (PARTITION BY "conversation_id" ORDER BY "created_at", "id") AS "prev"
  FROM "messages"
)
UPDATE "messages" m SET "parent_id" = o."prev"
FROM "ordered" o WHERE m."id" = o."id";

WITH "last" AS (
  SELECT DISTINCT ON ("conversation_id") "conversation_id", "id"
  FROM "messages"
  ORDER BY "conversation_id", "created_at" DESC, "id" DESC
)
UPDATE "conversations" c SET "active_leaf_id" = l."id"
FROM "last" l WHERE c."id" = l."conversation_id";

-- Parked approvals anchor at the paused run's tail: the conversation's active
-- leaf is the last row that run persisted. A mixed batch executes some calls
-- before pausing, so anchoring at the assistant call row would drop those
-- results from the resume history. Sends are blocked while an approval is
-- pending, so no later unrelated row can be the leaf.
UPDATE "pending_tool_calls" p SET "message_id" = c."active_leaf_id"
FROM "conversations" c
WHERE c."id" = p."conversation_id" AND p."status" = 'pending'
  AND c."active_leaf_id" IS NOT NULL;

-- FKs after backfill so the chain validation never sees a half-built tree.
ALTER TABLE "messages"
  ADD CONSTRAINT "messages_parent_id_fkey"
  FOREIGN KEY ("parent_id") REFERENCES "messages" ("id") ON DELETE CASCADE;
CREATE INDEX "idx_messages_parent" ON "messages" ("parent_id");

ALTER TABLE "conversations"
  ADD CONSTRAINT "conversations_active_leaf_id_fkey"
  FOREIGN KEY ("active_leaf_id") REFERENCES "messages" ("id") ON DELETE SET NULL;

ALTER TABLE "pending_tool_calls"
  ADD CONSTRAINT "pending_tool_calls_message_id_fkey"
  FOREIGN KEY ("message_id") REFERENCES "messages" ("id") ON DELETE CASCADE;

-- +goose Down
-- Rollback is lossy by design: variants off the active path are pruned so
-- the surviving linear order stays a valid conversation, then the tree
-- columns drop. Restore from backup to keep branches.
ALTER TABLE "pending_tool_calls" DROP CONSTRAINT IF EXISTS "pending_tool_calls_message_id_fkey";
ALTER TABLE "pending_tool_calls" DROP COLUMN IF EXISTS "message_id";
ALTER TABLE "conversations" DROP CONSTRAINT IF EXISTS "conversations_active_leaf_id_fkey";

WITH RECURSIVE "leaf" AS (
  SELECT c."id" AS "conv_id",
         COALESCE(c."active_leaf_id", (
           SELECT m."id" FROM "messages" m
           WHERE m."conversation_id" = c."id"
           ORDER BY m."created_at" DESC, m."id" DESC LIMIT 1
         )) AS "leaf_id"
  FROM "conversations" c
),
"path" AS (
  SELECT m."id", m."parent_id"
  FROM "messages" m JOIN "leaf" l ON m."id" = l."leaf_id"
  UNION
  SELECT p."id", p."parent_id"
  FROM "messages" p JOIN "path" ON p."id" = "path"."parent_id"
)
DELETE FROM "messages" m
WHERE NOT EXISTS (SELECT 1 FROM "path" WHERE "path"."id" = m."id");

ALTER TABLE "conversations" DROP COLUMN IF EXISTS "active_leaf_id";
ALTER TABLE "messages" DROP CONSTRAINT IF EXISTS "messages_parent_id_fkey";
DROP INDEX IF EXISTS "idx_messages_parent";
ALTER TABLE "messages" DROP COLUMN IF EXISTS "parent_id";
