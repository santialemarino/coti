-- +goose Up
ALTER TABLE quote_send ADD COLUMN client_association_hash VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX idx_client_account_tax_id ON client(account_id,tax_id) WHERE tax_id IS NOT NULL;

-- +goose Down
DROP INDEX idx_client_account_tax_id;
ALTER TABLE quote_send DROP COLUMN client_association_hash;
