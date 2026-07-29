-- El código de producto identifica de forma unívoca dentro de la cuenta, así que la
-- carga de precios "por código" tiene un único destino posible. Sin esto, actualizar
-- por código es no determinístico cuando hay dos productos con el mismo.
--
-- Índice parcial porque code es nullable: un producto de mostrador puede no tener
-- código, y esos no compiten entre sí.

-- +goose Up
CREATE UNIQUE INDEX uq_product_account_code ON product (account_id, code) WHERE code IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_product_account_code;
