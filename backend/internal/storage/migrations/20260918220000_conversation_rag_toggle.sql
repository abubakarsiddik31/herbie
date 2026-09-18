-- +goose Up
-- add "rag_enabled" to conversations: per-conversation document search
-- toggle (defaults on; existing conversations keep retrieval).
ALTER TABLE "conversations" ADD COLUMN "rag_enabled" boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE "conversations" DROP COLUMN "rag_enabled";
