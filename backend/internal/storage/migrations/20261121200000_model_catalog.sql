-- +goose Up
-- Admin-managed model catalog, seeded from the process catalog. Rows here are
-- the source of truth after boot; enabled=false hides a model even when its
-- provider key is configured.
CREATE TABLE "model_catalog" (
  "id" text NOT NULL,
  "provider" text NOT NULL,
  "label" text NOT NULL,
  "input_per_m" double precision NOT NULL DEFAULT 0,
  "output_per_m" double precision NOT NULL DEFAULT 0,
  "enabled" boolean NOT NULL DEFAULT true,
  "capabilities" jsonb NOT NULL DEFAULT '{}',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "model_catalog_provider_check"
    CHECK ("provider" IN ('gemini', 'openai', 'anthropic'))
);

INSERT INTO "model_catalog" (id, provider, label, input_per_m, output_per_m) VALUES
  ('gemini-3.5-flash', 'gemini', 'Gemini 3.5 Flash', 0.30, 2.50),
  ('gemini-3.5-pro', 'gemini', 'Gemini 3.5 Pro', 1.25, 10.00),
  ('gemma-3-27b-it', 'gemini', 'Gemma 3 27B', 0.20, 1.50),
  ('gemma-3-12b-it', 'gemini', 'Gemma 3 12B', 0.10, 0.80),
  ('gpt-5', 'openai', 'GPT-5', 1.25, 10.00),
  ('gpt-4.1', 'openai', 'GPT-4.1', 2.00, 8.00),
  ('claude-sonnet-4-5', 'anthropic', 'Claude Sonnet 4.5', 3.00, 15.00),
  ('claude-haiku-4-5', 'anthropic', 'Claude Haiku 4.5', 1.00, 5.00),
  ('gemini-2.5-flash', 'gemini', 'Gemini 2.5 Flash', 0.30, 2.50),
  ('gemini-2.5-flash-lite', 'gemini', 'Gemini 2.5 Flash Lite', 0.10, 0.40),
  ('gemini-2.5-pro', 'gemini', 'Gemini 2.5 Pro', 1.25, 10.00);

-- +goose Down
DROP TABLE IF EXISTS "model_catalog";
