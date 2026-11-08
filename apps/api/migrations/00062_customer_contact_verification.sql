-- +goose Up
ALTER TABLE channel ADD CONSTRAINT uq_channel_account_branch UNIQUE(account_id,branch_id,id);
ALTER TABLE client_contact ADD COLUMN verified_at TIMESTAMPTZ;
ALTER TABLE client_contact ALTER COLUMN verified_at SET DEFAULT now();
ALTER TABLE client_contact ADD COLUMN retired_at TIMESTAMPTZ;
SELECT assert_migration_sees_all_rows('client','client_contact','client_contact_observation','rfq','quote','branch');
UPDATE client_contact SET retired_at=now() WHERE client_id IN (SELECT id FROM client WHERE merged_into IS NOT NULL);
-- Only values shaped as NormalizeClientContact requires are trusted. Each live value keeps one owner: the
-- profile with a CUIT, else the newest; every other copy stays unverified and goes to seller review.
WITH valid AS (
 SELECT c.id,c.account_id,c.client_id,c.kind,c.value,c.original_value,c.created_at,c.retired_at,cl.tax_id,cl.name,
  row_number() OVER owners AS owner_rank,first_value(c.client_id) OVER owners AS owner_id
 FROM client_contact c JOIN client cl ON cl.account_id=c.account_id AND cl.id=c.client_id
 WHERE CASE c.kind WHEN 'WHATSAPP' THEN c.value ~ '^\+[1-9][0-9]{6,14}$' ELSE c.value ~ '^[a-z0-9!#$%&''*+/=?^_`{|}~-]+(\.[a-z0-9!#$%&''*+/=?^_`{|}~-]+)*@[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$' AND c.value !~ '[[:cntrl:]]' END
 WINDOW owners AS (PARTITION BY c.account_id,c.kind,c.value,c.retired_at IS NULL ORDER BY cl.tax_id IS NOT NULL DESC,cl.created_at DESC,cl.id DESC)
), verified AS (
 UPDATE client_contact c SET verified_at=v.created_at FROM valid v WHERE c.id=v.id AND (v.owner_rank=1 OR v.retired_at IS NOT NULL)
)
INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,declared_name,original_text,status)
SELECT account_id,branch_id,client_id,owner_id,kind,value,name,original_text,'PENDING' FROM (
 SELECT v.*,COALESCE(v.original_value,v.value) AS original_text,COALESCE(
  (SELECT branch_id FROM rfq WHERE account_id=v.account_id AND client_id=v.client_id ORDER BY created_at DESC,id DESC LIMIT 1),
  (SELECT branch_id FROM quote WHERE account_id=v.account_id AND client_id=v.client_id ORDER BY created_at DESC,id DESC LIMIT 1),
  (SELECT id FROM branch WHERE account_id=v.account_id ORDER BY created_at,id LIMIT 1)) AS branch_id
 FROM valid v WHERE v.owner_rank>1 AND v.retired_at IS NULL
) duplicate WHERE branch_id IS NOT NULL;
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
DELETE FROM client_contact_observation WHERE message_id IS NULL;
ALTER TABLE client_contact DROP COLUMN verified_at;
ALTER TABLE client_contact DROP COLUMN retired_at;
