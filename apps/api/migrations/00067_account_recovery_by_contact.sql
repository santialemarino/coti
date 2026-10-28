-- +goose Up
SELECT assert_migration_sees_all_rows('customer_identity','customer_auth_challenge','client_contact_challenge');

-- Recovery goes to a verified contact, so the separate recovery address and its link challenges go.
DELETE FROM customer_auth_challenge WHERE purpose IN ('VERIFY_EMAIL','RESET_PASSWORD');
ALTER TABLE customer_identity DROP COLUMN recovery_email;
ALTER TABLE customer_identity DROP COLUMN recovery_email_verified_at;
ALTER TYPE customer_auth_purpose RENAME TO customer_auth_purpose_old;
CREATE TYPE customer_auth_purpose AS ENUM ('GOOGLE_SIGNUP');
ALTER TABLE customer_auth_challenge ALTER COLUMN purpose TYPE customer_auth_purpose USING purpose::text::customer_auth_purpose;
DROP TYPE customer_auth_purpose_old;

CREATE TYPE client_contact_challenge_purpose AS ENUM ('VERIFY','RECOVER');
ALTER TABLE client_contact_challenge ADD COLUMN purpose client_contact_challenge_purpose NOT NULL DEFAULT 'VERIFY';
ALTER TABLE client_contact_challenge ALTER COLUMN purpose DROP DEFAULT;

-- The main contact the customer chose, at most one per profile; without a choice the first WhatsApp it
-- proved is the main one, else the first contact. Always a live, verified contact.
ALTER TABLE client_contact ADD COLUMN is_main BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE client_contact ADD CONSTRAINT ck_client_contact_main CHECK (NOT is_main OR (verified_at IS NOT NULL AND retired_at IS NULL));
CREATE UNIQUE INDEX uq_client_contact_main ON client_contact(account_id,client_id) WHERE is_main;

-- +goose Down
DROP INDEX uq_client_contact_main;
ALTER TABLE client_contact DROP CONSTRAINT ck_client_contact_main;
ALTER TABLE client_contact DROP COLUMN is_main;
DELETE FROM client_contact_challenge WHERE purpose='RECOVER';
ALTER TABLE client_contact_challenge DROP COLUMN purpose;
DROP TYPE client_contact_challenge_purpose;
ALTER TYPE customer_auth_purpose RENAME TO customer_auth_purpose_old;
CREATE TYPE customer_auth_purpose AS ENUM ('GOOGLE_SIGNUP','VERIFY_EMAIL','RESET_PASSWORD');
ALTER TABLE customer_auth_challenge ALTER COLUMN purpose TYPE customer_auth_purpose USING purpose::text::customer_auth_purpose;
DROP TYPE customer_auth_purpose_old;
ALTER TABLE customer_identity ADD COLUMN recovery_email VARCHAR(255);
ALTER TABLE customer_identity ADD COLUMN recovery_email_verified_at TIMESTAMPTZ;
