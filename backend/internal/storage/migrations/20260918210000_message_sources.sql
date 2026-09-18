-- +goose Up
-- add "sources" to messages: the retrieved chunks a grounded answer cited,
-- as the same rows the sources SSE event carries (empty array when the run
-- never searched).
ALTER TABLE "messages" ADD COLUMN "sources" jsonb NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE "messages" DROP COLUMN "sources";
