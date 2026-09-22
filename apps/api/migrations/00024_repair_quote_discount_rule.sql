-- Reconciles databases that recorded migration 00018 without its discount rule columns.

-- +goose Up

ALTER TABLE quote_discount
  ADD COLUMN IF NOT EXISTS action_type promotion_action_type NOT NULL DEFAULT 'FIXED_AMOUNT',
  ADD COLUMN IF NOT EXISTS action_value NUMERIC(14,2);

UPDATE quote_discount
SET action_type = 'FIXED_AMOUNT'
WHERE action_type IS NULL;

UPDATE quote_discount
SET action_type = 'FIXED_AMOUNT', action_value = amount
WHERE origin = 'MANUAL_SELLER' AND action_value IS NULL;

ALTER TABLE quote_discount
  ALTER COLUMN action_type SET DEFAULT 'FIXED_AMOUNT',
  ALTER COLUMN action_type SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'quote_discount'::regclass
      AND conname = 'ck_quote_discount_manual_action'
  ) THEN
    ALTER TABLE quote_discount
      ADD CONSTRAINT ck_quote_discount_manual_action CHECK (
        origin <> 'MANUAL_SELLER'::discount_origin OR action_value > 0
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'quote_discount'::regclass
      AND conname = 'ck_quote_discount_percentage_bounds'
  ) THEN
    ALTER TABLE quote_discount
      ADD CONSTRAINT ck_quote_discount_percentage_bounds CHECK (
        action_type <> 'PERCENTAGE'::promotion_action_type
        OR (action_value > 0 AND action_value <= 100)
      );
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- Version 00023 already requires these columns and constraints.
SELECT 1;
