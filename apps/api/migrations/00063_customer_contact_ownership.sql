-- +goose Up
ALTER TABLE rfq ADD CONSTRAINT uq_rfq_account_id UNIQUE(account_id,id);
ALTER TABLE client_contact ADD CONSTRAINT uq_client_contact_account_id UNIQUE(account_id,id);
CREATE TABLE customer_contact_ownership (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 customer_id UUID NOT NULL,
 contact_id UUID NOT NULL,
 includes_profile_history BOOLEAN NOT NULL,
 verified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(account_id,customer_id) REFERENCES customer_identity(account_id,id) ON DELETE CASCADE,
 FOREIGN KEY(account_id,contact_id) REFERENCES client_contact(account_id,id) ON DELETE CASCADE,
 UNIQUE(account_id,customer_id,contact_id)
);
ALTER TABLE customer_contact_ownership ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_contact_ownership_account_isolation ON customer_contact_ownership
 USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

CREATE TABLE customer_contact_history (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 customer_id UUID NOT NULL,
 rfq_id UUID NOT NULL,
 FOREIGN KEY(account_id,rfq_id) REFERENCES rfq(account_id,id) ON DELETE CASCADE,
 kind channel_type NOT NULL CHECK(kind IN ('EMAIL','WHATSAPP')),
 value VARCHAR(255) NOT NULL,
 FOREIGN KEY(account_id,customer_id) REFERENCES customer_identity(account_id,id) ON DELETE CASCADE,
 UNIQUE(account_id,customer_id,rfq_id,kind,value)
);
ALTER TABLE customer_contact_history ENABLE ROW LEVEL SECURITY;
CREATE POLICY customer_contact_history_account_isolation ON customer_contact_history
 USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

-- +goose Down
DROP TABLE customer_contact_history;
DROP TABLE customer_contact_ownership;
ALTER TABLE client_contact DROP CONSTRAINT uq_client_contact_account_id;
ALTER TABLE rfq DROP CONSTRAINT uq_rfq_account_id;
