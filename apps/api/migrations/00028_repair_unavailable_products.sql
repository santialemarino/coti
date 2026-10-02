-- Makes every active product that no branch carries available at each active branch of its
-- account. A product with any branch_product row keeps it: that is a real availability decision.

-- +goose Up
INSERT INTO branch_product (account_id, branch_id, product_id)
SELECT p.account_id, b.id, p.id
FROM product p
JOIN branch b ON b.account_id = p.account_id AND b.is_active = TRUE
WHERE p.is_active = TRUE
  AND NOT EXISTS (SELECT 1 FROM branch_product bp WHERE bp.product_id = p.id)
ON CONFLICT (branch_id, product_id) DO NOTHING;

-- +goose Down
-- Keep the rows: once repaired, they are indistinguishable from availability set on purpose.
SELECT 1;
