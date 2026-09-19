-- +goose Up
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "role" text NOT NULL DEFAULT 'user';
CREATE INDEX IF NOT EXISTS "idx_users_role" ON "users" ("role");

-- +goose Down
DROP INDEX IF EXISTS "idx_users_role";
ALTER TABLE "users" DROP COLUMN IF EXISTS "role";
