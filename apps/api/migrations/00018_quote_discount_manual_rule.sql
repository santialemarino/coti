-- A seller-typed (MANUAL_SELLER) discount carries the rule it was entered with, not only the
-- money amount the deterministic engine computed from it. action_type reuses the promotion
-- enum: FIXED_AMOUNT for money, PERCENTAGE for a rate. amount stays the computed money — the
-- audit trail never holds a rule instead of a result. Automated rows leave the columns at
-- their defaults and keep being recomputed by the sweep from their promotion rule.

-- +goose Up

ALTER TABLE quote_discount
  ADD COLUMN action_type promotion_action_type NOT NULL DEFAULT 'FIXED_AMOUNT',
  ADD COLUMN action_value NUMERIC(14,2);

-- Every seller-typed discount names the value it was typed with (money for FIXED_AMOUNT,
-- percentage for PERCENTAGE); automated applications carry no typed value.
ALTER TABLE quote_discount
  ADD CONSTRAINT ck_quote_discount_manual_action CHECK (
    origin <> 'MANUAL_SELLER'::discount_origin
    OR (action_value > 0)
  );

-- A percentage rate is bounded to (0, 100] so the computed amount can never exceed the base
-- it is applied to.
ALTER TABLE quote_discount
  ADD CONSTRAINT ck_quote_discount_percentage_bounds CHECK (
    action_type <> 'PERCENTAGE'::promotion_action_type
    OR (action_value > 0 AND action_value <= 100)
  );

-- +goose Down

ALTER TABLE quote_discount
  DROP CONSTRAINT ck_quote_discount_percentage_bounds,
  DROP CONSTRAINT ck_quote_discount_manual_action,
  DROP COLUMN action_type,
  DROP COLUMN action_value;