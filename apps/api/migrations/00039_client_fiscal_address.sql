-- +goose Up
ALTER TABLE client ADD COLUMN fiscal_address VARCHAR(255);

-- +goose Down
ALTER TABLE client DROP COLUMN fiscal_address;
