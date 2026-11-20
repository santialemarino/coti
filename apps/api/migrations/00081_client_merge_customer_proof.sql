-- +goose Up
ALTER TABLE client_merge_audit
 ADD COLUMN requires_customer_proof BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN customer_confirmed_at TIMESTAMPTZ,
 ADD CONSTRAINT uq_client_merge_audit_account_id UNIQUE (account_id, id),
 ADD CONSTRAINT ck_client_merge_audit_confirmation
   CHECK (customer_confirmed_at IS NULL OR requires_customer_proof);

-- Orders a seller merged into a customer's account, kept out of the portal until the customer proves
-- one of the merged profile's contacts.
CREATE TABLE client_merge_rfq (
 account_id UUID NOT NULL REFERENCES account(id),
 merge_audit_id UUID NOT NULL,
 rfq_id UUID NOT NULL,
 PRIMARY KEY (account_id, merge_audit_id, rfq_id),
 FOREIGN KEY (account_id, merge_audit_id) REFERENCES client_merge_audit(account_id, id),
 FOREIGN KEY (account_id, rfq_id) REFERENCES rfq(account_id, id)
);
CREATE INDEX idx_client_merge_rfq_rfq ON client_merge_rfq(account_id, rfq_id);
ALTER TABLE client_merge_rfq ENABLE ROW LEVEL SECURITY;
CREATE POLICY client_merge_rfq_account_isolation ON client_merge_rfq
 USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

-- +goose Down
DROP TABLE client_merge_rfq;
ALTER TABLE client_merge_audit
 DROP CONSTRAINT ck_client_merge_audit_confirmation,
 DROP CONSTRAINT uq_client_merge_audit_account_id,
 DROP COLUMN customer_confirmed_at,
 DROP COLUMN requires_customer_proof;
