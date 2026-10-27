-- +goose Up
ALTER TABLE branch ADD CONSTRAINT uq_branch_account_id UNIQUE (account_id, id);
CREATE TABLE arca_setup (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL UNIQUE REFERENCES account(id) ON DELETE CASCADE,
  tax_id VARCHAR(11) NOT NULL,
  csr TEXT NOT NULL,
  sealed_key TEXT NOT NULL,
  certificate TEXT NOT NULL DEFAULT '',
  sealed_ticket TEXT NOT NULL DEFAULT '',
  certificate_expires_at TIMESTAMPTZ,
  UNIQUE (account_id, id)
);

CREATE TABLE branch_arca_setup (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL REFERENCES account(id) ON DELETE CASCADE,
  branch_id UUID NOT NULL,
  setup_id UUID NOT NULL,
  point_of_sale INTEGER NOT NULL CHECK (point_of_sale BETWEEN 1 AND 99998),
  verified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (account_id, branch_id),
  UNIQUE (account_id, point_of_sale),
  FOREIGN KEY (account_id, setup_id) REFERENCES arca_setup(account_id, id) ON DELETE CASCADE,
  FOREIGN KEY (account_id, branch_id) REFERENCES branch(account_id, id) ON DELETE CASCADE
);

ALTER TABLE arca_setup ENABLE ROW LEVEL SECURITY;
ALTER TABLE arca_setup FORCE ROW LEVEL SECURITY;
CREATE POLICY arca_setup_tenant ON arca_setup
  USING (account_id = NULLIF(current_setting('app.current_account_id', true), '')::uuid)
  WITH CHECK (account_id = NULLIF(current_setting('app.current_account_id', true), '')::uuid);
ALTER TABLE branch_arca_setup ENABLE ROW LEVEL SECURITY;
ALTER TABLE branch_arca_setup FORCE ROW LEVEL SECURITY;
CREATE POLICY branch_arca_setup_tenant ON branch_arca_setup
  USING (account_id = NULLIF(current_setting('app.current_account_id', true), '')::uuid)
  WITH CHECK (account_id = NULLIF(current_setting('app.current_account_id', true), '')::uuid);

-- +goose Down
DROP TABLE branch_arca_setup;
DROP TABLE arca_setup;
ALTER TABLE branch DROP CONSTRAINT uq_branch_account_id;
