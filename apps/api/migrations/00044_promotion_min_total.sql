-- +goose Up
-- The net an ON_TOTAL promotion needs before it applies; null = no threshold.
ALTER TABLE promotion ADD COLUMN min_total NUMERIC(14,2);
ALTER TABLE promotion ADD CONSTRAINT ck_promotion_min_total CHECK (
  min_total IS NULL OR (condition_type = 'ON_TOTAL'::promotion_condition_type AND min_total > 0)
);

-- +goose Down
ALTER TABLE promotion DROP CONSTRAINT ck_promotion_min_total;
ALTER TABLE promotion DROP COLUMN min_total;
