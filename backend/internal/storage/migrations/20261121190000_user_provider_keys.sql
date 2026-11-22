-- +goose Up
-- Per-user provider API keys (BYOK), encrypted by the credential vault before
-- they reach this table. The server keys remain the fallback.
CREATE TABLE "user_provider_keys" (
  "user_id" uuid NOT NULL,
  "provider" text NOT NULL,
  "encrypted_key" jsonb NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("user_id", "provider"),
  CONSTRAINT "user_provider_keys_provider_check"
    CHECK ("provider" IN ('gemini', 'openai', 'anthropic')),
  CONSTRAINT "user_provider_keys_user_id_fkey"
    FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS "user_provider_keys";
