-- A versioned, resumable onboarding flow whose stable step keys can grow without renumbering
-- progress already recorded for an account.

-- +goose Up

CREATE TYPE onboarding_status AS ENUM ('IN_PROGRESS', 'COMPLETED', 'DISMISSED');
CREATE TYPE onboarding_step_status AS ENUM ('COMPLETED', 'SKIPPED');

CREATE TABLE account_onboarding (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  flow_version   INTEGER NOT NULL DEFAULT 1 CHECK (flow_version > 0),
  status         onboarding_status NOT NULL DEFAULT 'IN_PROGRESS',
  current_step   VARCHAR(64) NOT NULL DEFAULT 'WELCOME',
  completed_at   TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_account_onboarding_account UNIQUE (account_id),
  CONSTRAINT fk_account_onboarding_account FOREIGN KEY (account_id) REFERENCES account(id)
);

CREATE TABLE onboarding_step_progress (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  onboarding_id  UUID NOT NULL,
  step_key       VARCHAR(64) NOT NULL,
  status         onboarding_step_status NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_onboarding_step UNIQUE (onboarding_id, step_key),
  CONSTRAINT fk_onboarding_step_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_onboarding_step_onboarding FOREIGN KEY (onboarding_id) REFERENCES account_onboarding(id)
);

-- Accounts that predate this flow keep their current entry experience. New registrations create
-- an IN_PROGRESS row explicitly inside the registration transaction.
INSERT INTO account_onboarding (account_id, status, current_step, completed_at)
SELECT id, 'DISMISSED', 'WELCOME', NULL FROM account;

CREATE INDEX idx_onboarding_step_account ON onboarding_step_progress(account_id);

CREATE TRIGGER trg_account_onboarding_updated BEFORE UPDATE ON account_onboarding
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_onboarding_step_updated BEFORE UPDATE ON onboarding_step_progress
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE account_onboarding ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_onboarding_account_isolation ON account_onboarding
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE onboarding_step_progress ENABLE ROW LEVEL SECURITY;
CREATE POLICY onboarding_step_progress_account_isolation ON onboarding_step_progress
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON account_onboarding, onboarding_step_progress TO coti_app;

-- +goose Down

DROP TABLE onboarding_step_progress;
DROP TABLE account_onboarding;
DROP TYPE onboarding_step_status;
DROP TYPE onboarding_status;
