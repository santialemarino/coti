-- +goose Up
ALTER TABLE channel ADD CONSTRAINT uq_channel_account_branch UNIQUE(account_id,branch_id,id);
ALTER TABLE client_contact ADD COLUMN verified_at TIMESTAMPTZ;
ALTER TABLE client_contact ALTER COLUMN verified_at SET DEFAULT now();
ALTER TABLE client_contact ADD COLUMN retired_at TIMESTAMPTZ;
UPDATE client_contact SET verified_at=created_at;
UPDATE client_contact SET retired_at=now() WHERE client_id IN (SELECT id FROM client WHERE merged_into IS NOT NULL);
CREATE UNIQUE INDEX uq_verified_client_contact ON client_contact(account_id,kind,value) WHERE verified_at IS NOT NULL AND retired_at IS NULL;

CREATE TABLE client_contact_challenge (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 customer_id UUID NOT NULL,
 client_id UUID NOT NULL,
 kind channel_type NOT NULL CHECK(kind IN ('WHATSAPP','EMAIL')),
 value VARCHAR(255) NOT NULL,
 token_hash VARCHAR(64) NOT NULL,
 attempts INTEGER NOT NULL DEFAULT 0,
 expires_at TIMESTAMPTZ NOT NULL,
 consumed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(account_id,customer_id) REFERENCES customer_identity(account_id,id),
 FOREIGN KEY(account_id,client_id) REFERENCES client(account_id,id)
);
CREATE INDEX idx_contact_challenge_customer ON client_contact_challenge(account_id,customer_id,created_at);
ALTER TABLE client_contact_challenge ENABLE ROW LEVEL SECURITY;
CREATE POLICY contact_challenge_account_isolation ON client_contact_challenge USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

CREATE TYPE client_contact_outbox_purpose AS ENUM ('REQUEST_NAME');
CREATE TYPE client_contact_outbox_status AS ENUM ('PENDING','SENDING','SENT','FAILED','COMPLETED');

CREATE TABLE client_contact_outbox (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 branch_id UUID NOT NULL,
 channel_id UUID NOT NULL,
 kind channel_type NOT NULL CHECK(kind IN ('WHATSAPP','EMAIL')),
 value VARCHAR(255) NOT NULL,
 purpose client_contact_outbox_purpose NOT NULL,
 status client_contact_outbox_status NOT NULL DEFAULT 'PENDING',
 provider_reference TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(account_id,branch_id) REFERENCES branch(account_id,id),
 FOREIGN KEY(account_id,branch_id,channel_id) REFERENCES channel(account_id,branch_id,id),
 UNIQUE(account_id,kind,value,purpose)
);
ALTER TABLE client_contact_outbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY contact_outbox_account_isolation ON client_contact_outbox USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

-- +goose StatementBegin
CREATE FUNCTION protect_client_tax_id() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.tax_id IS NOT NULL AND NEW.tax_id IS DISTINCT FROM OLD.tax_id THEN
  RAISE EXCEPTION 'Client fiscal identifiers are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER protect_client_tax_id BEFORE UPDATE OF tax_id ON client FOR EACH ROW EXECUTE FUNCTION protect_client_tax_id();

-- +goose Down
DROP TRIGGER protect_client_tax_id ON client;
DROP FUNCTION protect_client_tax_id();
DROP TABLE client_contact_outbox;
DROP TYPE client_contact_outbox_status;
DROP TYPE client_contact_outbox_purpose;
ALTER TABLE channel DROP CONSTRAINT uq_channel_account_branch;
DROP TABLE client_contact_challenge;
ALTER TABLE client_contact DROP COLUMN verified_at;
ALTER TABLE client_contact DROP COLUMN retired_at;
