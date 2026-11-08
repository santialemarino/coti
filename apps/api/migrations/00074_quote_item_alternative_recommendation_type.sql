-- +goose Up
-- A matcher candidate is not a commercial recommendation. Preserve the catalog relationship that
-- produced an ECONOMY or PREMIUM offer so the seller can distinguish it from a near-match.
ALTER TABLE quote_item_alternative
  ADD COLUMN recommendation_type product_alternative_type,
  ADD CONSTRAINT ck_qia_recommendation_type
    CHECK (recommendation_type IS NULL OR type = 'PRODUCT');

-- +goose Down
ALTER TABLE quote_item_alternative DROP CONSTRAINT ck_qia_recommendation_type;
ALTER TABLE quote_item_alternative DROP COLUMN recommendation_type;
