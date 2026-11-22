-- +goose Up
-- Capability flags drive which generation controls the UI shows. Gemma and
-- Flash Lite are not reasoning models on their current endpoints.
UPDATE "model_catalog" SET "capabilities" = '{"reasoning":true}'::jsonb
 WHERE "id" IN ('gemini-3.5-flash','gemini-3.5-pro','gpt-5','gpt-4.1','claude-sonnet-4-5','claude-haiku-4-5','gemini-2.5-flash','gemini-2.5-pro');
UPDATE "model_catalog" SET "capabilities" = '{"reasoning":false}'::jsonb
 WHERE "id" IN ('gemma-3-27b-it','gemma-3-12b-it','gemini-2.5-flash-lite');

-- +goose Down
UPDATE "model_catalog" SET "capabilities" = '{}'::jsonb;
