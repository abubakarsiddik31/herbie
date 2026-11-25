-- +goose Up
-- Existing accounts predate verification: treat them as verified so turning
-- EMAIL_VERIFICATION_REQUIRED on never locks out deployed users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;
UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL;

-- A session is one refresh-token rotation family. Legacy rows become their
-- own single-token session; new rows inherit the ID across rotations.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS session_id uuid;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS user_agent text NOT NULL DEFAULT '';
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS ip text NOT NULL DEFAULT '';
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS last_used_at timestamptz NOT NULL DEFAULT now();
UPDATE refresh_tokens SET session_id = id WHERE session_id IS NULL;
ALTER TABLE refresh_tokens ALTER COLUMN session_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_session ON refresh_tokens (user_id, session_id);

CREATE TABLE auth_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    purpose    text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX auth_tokens_user_purpose_key ON auth_tokens (user_id, purpose);
CREATE INDEX idx_auth_tokens_user_purpose ON auth_tokens (user_id, purpose);
CREATE INDEX idx_auth_tokens_expiry ON auth_tokens (expires_at);

-- +goose Down
DROP TABLE IF EXISTS auth_tokens;
DROP INDEX IF EXISTS idx_refresh_tokens_session;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS last_used_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS ip;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS user_agent;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS session_id;
ALTER TABLE users DROP COLUMN IF EXISTS email_verified_at;
