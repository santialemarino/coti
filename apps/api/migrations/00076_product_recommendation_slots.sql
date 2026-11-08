-- +goose Up

-- Recommendations are commercial slots, not an open-ended tag list. Preserve the newest
-- configured relation before making each ECONOMY and PREMIUM slot unique.
WITH ranked AS (
  SELECT id,
         row_number() OVER (
           PARTITION BY account_id, base_product_id, type
           ORDER BY created_at DESC, id DESC
         ) AS position
  FROM product_alternative
  WHERE type IN ('ECONOMY', 'PREMIUM')
)
DELETE FROM product_alternative alternative
USING ranked
WHERE alternative.id = ranked.id AND ranked.position > 1;

CREATE UNIQUE INDEX uq_product_alternative_recommendation
  ON product_alternative (account_id, base_product_id, type)
  WHERE type IN ('ECONOMY', 'PREMIUM');

-- +goose Down

DROP INDEX uq_product_alternative_recommendation;
