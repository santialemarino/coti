-- A product code identifies one row per account, so "update the price by code" has a
-- single target. Partial because code is nullable: counter-only products carry none, and
-- those do not compete with each other.

-- +goose Up
CREATE UNIQUE INDEX uq_product_account_code ON product (account_id, code) WHERE code IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_product_account_code;
