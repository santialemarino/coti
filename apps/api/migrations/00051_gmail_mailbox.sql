-- +goose Up
CREATE TABLE gmail_mailbox (
  channel_id                    UUID PRIMARY KEY REFERENCES channel(id),
  account_id                    UUID NOT NULL REFERENCES account(id),
  branch_id                     UUID NOT NULL REFERENCES branch(id),
  email_address            TEXT NOT NULL,
  refresh_token_ciphertext TEXT NOT NULL,
  history_id               TEXT NOT NULL,
  connected_at             TIMESTAMPTZ NOT NULL,
	initial_sync_pending     BOOLEAN NOT NULL DEFAULT TRUE,
  last_synced_at           TIMESTAMPTZ,
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gmail_authorization_state (
  state_hash          BYTEA PRIMARY KEY,
  account_id          UUID NOT NULL REFERENCES account(id),
  branch_id           UUID NOT NULL REFERENCES branch(id),
  channel_id          UUID NOT NULL REFERENCES channel(id),
  user_id             UUID NOT NULL REFERENCES app_user(id),
  session_epoch       INTEGER NOT NULL,
  verifier_ciphertext TEXT NOT NULL,
  expires_at          TIMESTAMPTZ NOT NULL
);

ALTER TABLE gmail_mailbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY gmail_mailbox_account_isolation ON gmail_mailbox
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());
ALTER TABLE gmail_authorization_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY gmail_authorization_state_account_isolation ON gmail_authorization_state
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- +goose Down
DROP TABLE gmail_authorization_state;
DROP TABLE gmail_mailbox;
