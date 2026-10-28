-- +goose Up
ALTER TABLE client ADD COLUMN merged_into UUID;
ALTER TABLE client ADD CONSTRAINT fk_client_merge FOREIGN KEY (account_id,merged_into) REFERENCES client(account_id,id);
ALTER TABLE client ADD CONSTRAINT ck_client_merge CHECK(merged_into IS NULL OR merged_into<>id);
CREATE TYPE client_contact_source AS ENUM ('LEGACY','CHANNEL','CUSTOMER','SELLER');
CREATE TYPE client_contact_review_status AS ENUM ('PENDING','ASSOCIATED','SEPARATE','RECORDED','MERGED');

CREATE TABLE client_contact (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 client_id UUID NOT NULL,
 kind channel_type NOT NULL CHECK (kind IN ('WHATSAPP','EMAIL')),
 value VARCHAR(255) NOT NULL,
 is_primary BOOLEAN NOT NULL DEFAULT FALSE,
 source client_contact_source NOT NULL,
 original_value VARCHAR(255),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY (account_id,client_id) REFERENCES client(account_id,id) ON DELETE CASCADE,
 UNIQUE(account_id,client_id,kind,value)
);
CREATE UNIQUE INDEX uq_client_contact_primary ON client_contact(account_id,client_id,kind) WHERE is_primary;
CREATE INDEX idx_client_contact_lookup ON client_contact(account_id,kind,value);

-- Called by every data migration: under forced RLS, or as a non-owner, its statements would match
-- zero rows.
-- +goose StatementBegin
CREATE FUNCTION assert_migration_sees_all_rows(VARIADIC tables regclass[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
 target regclass;
BEGIN
 IF (SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user) THEN
  RETURN;
 END IF;
 FOREACH target IN ARRAY tables LOOP
  IF EXISTS(SELECT 1 FROM pg_class WHERE oid=target AND relrowsecurity AND (relforcerowsecurity OR NOT pg_has_role(current_user,relowner,'USAGE'))) THEN
   RAISE EXCEPTION 'Row level security filters % for role %; a data migration on it would silently touch zero rows', target, current_user
    USING HINT='Run the migration as a superuser or a BYPASSRLS role.';
  END IF;
 END LOOP;
END;
$$;
-- +goose StatementEnd

SELECT assert_migration_sees_all_rows('client');
-- A national Argentine number (area 11, 2xx or 3xx, optional trunk 0) takes the +549 form WhatsApp
-- reports; anything else keeps its digits and 00062 leaves it unverified.
INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,original_value)
 SELECT account_id,id,'WHATSAPP',CASE
   WHEN digits ~ '^\+' THEN digits
   WHEN digits ~ '^00' THEN '+'||substr(digits,3)
   WHEN digits ~ '^54[0-9]{10,11}$' THEN '+'||digits
   WHEN digits ~ '^0?(11[0-9]{8}|[23][0-9]{9})$' THEN '+549'||regexp_replace(digits,'^0','')
   ELSE digits END,TRUE,'LEGACY',phone
 FROM (SELECT account_id,id,phone,regexp_replace(phone,'[^0-9+]','','g') AS digits FROM client WHERE phone IS NOT NULL AND btrim(phone)<>'') legacy;
INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,original_value)
 SELECT account_id,id,'EMAIL',lower(btrim(email)),TRUE,'LEGACY',email FROM client WHERE email IS NOT NULL AND btrim(email)<>'';

CREATE TABLE client_contact_observation (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 branch_id UUID NOT NULL,
 -- NULL when a migration, not an inbound message, raised the observation.
 message_id UUID,
 source_client_id UUID,
 candidate_client_id UUID,
 tax_id VARCHAR(11),
 kind channel_type NOT NULL CHECK (kind IN ('WHATSAPP','EMAIL')),
 value VARCHAR(255) NOT NULL,
 declared_name VARCHAR(255),
 original_text TEXT NOT NULL,
 status client_contact_review_status NOT NULL DEFAULT 'PENDING',
 decided_by UUID,
 decided_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY (account_id,branch_id) REFERENCES branch(account_id,id),
 FOREIGN KEY (account_id,source_client_id) REFERENCES client(account_id,id),
 FOREIGN KEY (account_id,candidate_client_id) REFERENCES client(account_id,id),
 FOREIGN KEY (account_id,decided_by) REFERENCES app_user(account_id,id),
 UNIQUE(account_id,message_id,kind,value)
);
CREATE INDEX idx_contact_observation_pending ON client_contact_observation(account_id,branch_id,created_at) WHERE status='PENDING';
ALTER TABLE client_contact ENABLE ROW LEVEL SECURITY;
CREATE POLICY client_contact_account_isolation ON client_contact USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());
ALTER TABLE client_contact_observation ENABLE ROW LEVEL SECURITY;
CREATE POLICY client_contact_observation_account_isolation ON client_contact_observation USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

-- +goose StatementBegin
CREATE FUNCTION seed_client_contacts() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.phone IS NOT NULL AND btrim(NEW.phone)<>'' THEN
  INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source) VALUES(NEW.account_id,NEW.id,'WHATSAPP',regexp_replace(NEW.phone,'[^0-9+]','','g'),TRUE,'SELLER');
 END IF;
 IF NEW.email IS NOT NULL AND btrim(NEW.email)<>'' THEN
  INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source) VALUES(NEW.account_id,NEW.id,'EMAIL',lower(btrim(NEW.email)),TRUE,'SELLER');
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER seed_client_contacts AFTER INSERT ON client FOR EACH ROW EXECUTE FUNCTION seed_client_contacts();

-- +goose Down
DROP TRIGGER seed_client_contacts ON client;
DROP FUNCTION seed_client_contacts();
DROP TABLE client_contact_observation;
DROP TABLE client_contact;
DROP FUNCTION assert_migration_sees_all_rows(regclass[]);
DROP TYPE client_contact_review_status;
DROP TYPE client_contact_source;
ALTER TABLE client DROP CONSTRAINT ck_client_merge;
ALTER TABLE client DROP CONSTRAINT fk_client_merge;
ALTER TABLE client DROP COLUMN merged_into;
