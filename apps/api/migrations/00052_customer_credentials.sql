-- +goose Up
CREATE TABLE customer_identity (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL REFERENCES account(id),
  tax_id VARCHAR(11) NOT NULL CHECK (tax_id ~ '^[0-9]{11}$'),
  password_hash TEXT,
  google_subject VARCHAR(255),
  recovery_email VARCHAR(255),
  recovery_email_verified_at TIMESTAMPTZ,
  name VARCHAR(255),
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  locked_until TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_customer_identity_scope UNIQUE (account_id, id),
  CONSTRAINT ck_customer_identity_credential CHECK (password_hash IS NOT NULL OR google_subject IS NOT NULL)
);
CREATE UNIQUE INDEX uq_customer_identity_active_tax_id ON customer_identity(account_id, tax_id)
  WHERE disabled_at IS NULL;

CREATE TABLE customer_session (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL REFERENCES account(id),
  customer_id UUID NOT NULL,
  token_hash VARCHAR(64) NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (account_id, customer_id) REFERENCES customer_identity(account_id, id)
);
CREATE INDEX idx_customer_session_subject ON customer_session(account_id, customer_id);

ALTER TABLE customer_identity ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_identity_account_isolation ON customer_identity
  USING (account_id = app_current_account_id()) WITH CHECK (account_id = app_current_account_id());
ALTER TABLE customer_session ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_session_account_isolation ON customer_session
  USING (account_id = app_current_account_id()) WITH CHECK (account_id = app_current_account_id());

CREATE UNIQUE INDEX uq_customer_google_subject ON customer_identity(account_id,google_subject) WHERE disabled_at IS NULL AND google_subject IS NOT NULL;
CREATE TYPE customer_auth_purpose AS ENUM ('GOOGLE_SIGNUP','VERIFY_EMAIL','RESET_PASSWORD');
CREATE TABLE customer_auth_challenge (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 customer_id UUID,
 purpose customer_auth_purpose NOT NULL,
 token_hash VARCHAR(64) NOT NULL UNIQUE,
 payload JSONB NOT NULL DEFAULT '{}',
 expires_at TIMESTAMPTZ NOT NULL,
 consumed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY (account_id,customer_id) REFERENCES customer_identity(account_id,id)
);
CREATE INDEX idx_customer_challenge_subject ON customer_auth_challenge(account_id,customer_id,purpose);
ALTER TABLE customer_auth_challenge ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_auth_challenge_account_isolation ON customer_auth_challenge USING (account_id=app_current_account_id()) WITH CHECK (account_id=app_current_account_id());

-- +goose Down
DROP TABLE customer_auth_challenge;
DROP TYPE customer_auth_purpose;
DROP TABLE customer_session;
DROP TABLE customer_identity;
