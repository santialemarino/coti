-- +goose Up
-- The customer identity that entered a contact, so a profile it has not yet proved shows it only its own.
ALTER TABLE client_contact ADD COLUMN declared_by UUID;
ALTER TABLE client_contact ADD CONSTRAINT fk_client_contact_declared_by FOREIGN KEY (account_id,declared_by) REFERENCES customer_identity(account_id,id);
UPDATE client_contact c SET declared_by=i.id
 FROM client profile JOIN customer_identity i ON i.account_id=profile.account_id AND i.tax_id=profile.tax_id AND i.disabled_at IS NULL
 WHERE profile.account_id=c.account_id AND profile.id=c.client_id AND c.source='CUSTOMER' AND c.created_at>=i.created_at;

-- +goose Down
ALTER TABLE client_contact DROP CONSTRAINT fk_client_contact_declared_by;
ALTER TABLE client_contact DROP COLUMN declared_by;
