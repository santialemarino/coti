-- A draft line remembers the sent line it was copied from, so the change-request comparison pairs
-- each line with the one it replaced instead of with whatever sits at the same position.

-- +goose Up
ALTER TABLE quote_item ADD COLUMN source_item_id UUID;
ALTER TABLE quote_item ADD CONSTRAINT fk_quote_item_source
  FOREIGN KEY (source_item_id) REFERENCES quote_item(id) ON DELETE SET NULL;

-- Open drafts get their pairing from the version before them, by the line's wording.
UPDATE quote_item d
SET source_item_id = (
  SELECT f.id
  FROM quote_version pv
  JOIN quote_item f ON f.account_id = pv.account_id AND f.version_id = pv.id
  WHERE pv.account_id = dv.account_id AND pv.quote_id = dv.quote_id
    AND pv.version_number = dv.version_number - 1
    AND f.requested_description = d.requested_description
  ORDER BY (f.product_id IS NOT DISTINCT FROM d.product_id) DESC, f.created_at, f.id
  LIMIT 1)
FROM quote_version dv
WHERE dv.account_id = d.account_id AND dv.id = d.version_id AND dv.is_immutable = FALSE;

-- +goose Down
ALTER TABLE quote_item DROP CONSTRAINT fk_quote_item_source;
ALTER TABLE quote_item DROP COLUMN source_item_id;
