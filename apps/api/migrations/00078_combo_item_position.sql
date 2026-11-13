-- +goose Up
SELECT assert_migration_sees_all_rows('combo_item', 'product');

-- A combo lists its items in the order the administrator entered them. Rows written in one statement
-- share created_at, so existing combos keep the order they were shown in: by name.
ALTER TABLE combo_item ADD COLUMN position SMALLINT CHECK (position >= 0);
UPDATE combo_item ci SET position = ordered.position
FROM (
  SELECT item.id, row_number() OVER (
    PARTITION BY item.combo_id ORDER BY item.created_at, p.canonical_name, item.id) - 1 AS position
  FROM combo_item item
  JOIN product p ON p.account_id = item.account_id AND p.id = item.product_id
) ordered
WHERE ci.id = ordered.id;
ALTER TABLE combo_item ALTER COLUMN position SET NOT NULL;
ALTER TABLE combo_item ADD CONSTRAINT uq_combo_item_position UNIQUE (combo_id, position);

-- +goose Down
ALTER TABLE combo_item DROP COLUMN position;
