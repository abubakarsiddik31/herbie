-- +goose Up
-- social login: linked provider identities, single-use frontend handoff
-- codes, and nullable password hashes for OAuth-only accounts.
ALTER TABLE "users" ALTER COLUMN "password_hash" DROP NOT NULL;

CREATE TABLE "oauth_accounts" (
  "provider" text NOT NULL,
  "provider_subject" text NOT NULL,
  "user_id" uuid NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("provider", "provider_subject")
);
CREATE UNIQUE INDEX "oauth_accounts_user_provider_key" ON "oauth_accounts" ("user_id", "provider");

CREATE TABLE "oauth_codes" (
  "code_hash" text PRIMARY KEY,
  "user_id" uuid NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "expires_at" timestamptz NOT NULL
);
CREATE INDEX "idx_oauth_codes_user" ON "oauth_codes" ("user_id");

-- +goose Down
DROP TABLE "oauth_codes";
DROP TABLE "oauth_accounts";
ALTER TABLE "users" ALTER COLUMN "password_hash" SET NOT NULL;
