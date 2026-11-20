-- The single-use link the team hands a corralón so it can register while public sign-up is closed.
--
-- It is pre-account, like job_run: there is no tenant until the link is used, so the table carries
-- no row level security and only the owner role (registration's AdminTx, cmd/invite) touches it.

-- +goose Up

CREATE TABLE signup_invite (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token_hash  CHAR(64) NOT NULL,
  email       VARCHAR(255),
  note        VARCHAR(500),
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  account_id  UUID,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_signup_invite_token_hash UNIQUE (token_hash),
  CONSTRAINT fk_signup_invite_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT ck_signup_invite_used CHECK ((used_at IS NULL) = (account_id IS NULL))
);

REVOKE ALL ON signup_invite FROM coti_app;

-- +goose Down

DROP TABLE signup_invite;
