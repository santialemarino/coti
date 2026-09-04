-- Exactly one open price period per branch and product, enforced by the schema.
--
-- The rule already lived in the service, serialized by a SELECT ... FOR UPDATE on the parent
-- product row. The lock is what makes two concurrent repricings both succeed correctly; this
-- index is what turns a path that forgets the lock into a loud error instead of a silently
-- duplicated open period.

-- +goose Up

-- The dedupe is not optional: the index cannot be created on a database that carries
-- duplicates. Closing each older period at the next one's valid_from is what a reprice does,
-- so the surviving history is the one the service would have written. It is a no-op with no
-- duplicates present, because lead() is then NULL for every row.
WITH ranked AS (
  SELECT id,
         lead(valid_from) OVER (PARTITION BY branch_id, product_id ORDER BY valid_from, id) AS next_valid_from
  FROM product_price
  WHERE valid_to IS NULL
)
UPDATE product_price p
SET valid_to = r.next_valid_from
FROM ranked r
WHERE p.id = r.id
  AND r.next_valid_from IS NOT NULL;

CREATE UNIQUE INDEX uq_product_price_open_period
  ON product_price (branch_id, product_id) WHERE valid_to IS NULL;

-- +goose Down

DROP INDEX uq_product_price_open_period;
