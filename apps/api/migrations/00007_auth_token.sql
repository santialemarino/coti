-- Single-use tokens a user presents instead of a session: the password-recovery link and
-- the address-verification link.

-- +goose Up

-- EMAIL_VERIFICATION is here from the start because a value cannot be used in the
-- transaction that adds it, and goose wraps each migration in one.
CREATE TYPE auth_token_type AS ENUM ('PASSWORD_RESET', 'EMAIL_VERIFICATION');

-- Only the hex SHA-256 of the raw token is stored; the raw value is high-entropy, so a fast
-- hash is enough. consumed_at is what makes the link single-use, and the row survives its use.
CREATE TABLE auth_token (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id  UUID NOT NULL,
  user_id     UUID NOT NULL,
  type        auth_token_type NOT NULL,
  token_hash  CHAR(64) NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_auth_token_hash UNIQUE (token_hash)
);

ALTER TABLE auth_token ADD CONSTRAINT fk_auth_token_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE auth_token ADD CONSTRAINT fk_auth_token_user FOREIGN KEY (user_id) REFERENCES app_user(id);

-- Asking for a new link invalidates the outstanding ones, which is the only hot read.
CREATE INDEX idx_auth_token_user_type ON auth_token(user_id, type) WHERE consumed_at IS NULL;

-- A table created here has row level security disabled by default, which would let the
-- restricted role read every account's rows with no error at all.
ALTER TABLE auth_token ENABLE ROW LEVEL SECURITY;
CREATE POLICY auth_token_account_isolation ON auth_token
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- Null until the address is confirmed. Added now for the same reason as the enum value.
ALTER TABLE app_user ADD COLUMN email_verified_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE app_user DROP COLUMN email_verified_at;

DROP TABLE auth_token;
DROP TYPE auth_token_type;
