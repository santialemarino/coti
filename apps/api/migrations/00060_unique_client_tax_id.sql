-- +goose Up
CREATE UNIQUE INDEX uq_client_account_tax_id ON client(account_id, tax_id)
  WHERE tax_id IS NOT NULL;

-- +goose Down
DROP INDEX uq_client_account_tax_id;
