-- +goose Up

-- An approved alternative is a real selectable offer. Its floor must be frozen with its price so
-- a client selection cannot use a newer catalog price or discount limit.
ALTER TABLE quote_item_alternative ADD COLUMN min_price_snapshot NUMERIC(14,2);

-- This internal payload freezes the rules used to settle a client-selected alternative. It is
-- deliberately separate from the customer-safe representation payload.
ALTER TABLE quote_representation ADD COLUMN selection_pricing_snapshot JSONB;
ALTER TABLE quote_representation ADD CONSTRAINT ck_quote_representation_selection_snapshot
  CHECK (selection_pricing_snapshot IS NULL OR jsonb_typeof(selection_pricing_snapshot) = 'object');

-- +goose Down

ALTER TABLE quote_representation DROP CONSTRAINT ck_quote_representation_selection_snapshot;
ALTER TABLE quote_representation DROP COLUMN selection_pricing_snapshot;
ALTER TABLE quote_item_alternative DROP COLUMN min_price_snapshot;
