-- Natural keys for the three tables whose only unique index was a random UUID, so
-- ON CONFLICT DO NOTHING had nothing to collide with and every seed run re-inserted them.

-- +goose Up

-- The rows already duplicated have to go before a unique index can exist. The oldest of each
-- group survives; client_tag points at tags that do not, so its links move first.
-- +goose StatementBegin
WITH ranked AS (
  SELECT id,
         first_value(id) OVER (
           PARTITION BY account_id, lower(name) ORDER BY created_at, id
         ) AS keeper
  FROM tag
)
DELETE FROM client_tag ct
USING ranked r
WHERE ct.tag_id = r.id
  AND r.id <> r.keeper
  AND EXISTS (SELECT 1 FROM client_tag k WHERE k.client_id = ct.client_id AND k.tag_id = r.keeper);
-- +goose StatementEnd

-- +goose StatementBegin
WITH ranked AS (
  SELECT id,
         first_value(id) OVER (
           PARTITION BY account_id, lower(name) ORDER BY created_at, id
         ) AS keeper
  FROM tag
)
UPDATE client_tag ct
SET tag_id = r.keeper
FROM ranked r
WHERE ct.tag_id = r.id AND r.id <> r.keeper;
-- +goose StatementEnd

-- +goose StatementBegin
WITH ranked AS (
  SELECT id,
         first_value(id) OVER (
           PARTITION BY account_id, lower(name) ORDER BY created_at, id
         ) AS keeper
  FROM tag
)
DELETE FROM tag t USING ranked r WHERE t.id = r.id AND r.id <> r.keeper;
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM promotion_tier t
USING (
  SELECT id, row_number() OVER (
           PARTITION BY promotion_id, from_quantity ORDER BY created_at, id
         ) AS n
  FROM promotion_tier
) d
WHERE t.id = d.id AND d.n > 1;
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM promotion_condition_item t
USING (
  SELECT id, row_number() OVER (
           PARTITION BY promotion_id, product_id, family_id, subgroup_id ORDER BY created_at, id
         ) AS n
  FROM promotion_condition_item
) d
WHERE t.id = d.id AND d.n > 1;
-- +goose StatementEnd

-- Case-insensitively, the way app_user.email and product_synonym.term already are.
CREATE UNIQUE INDEX uq_tag_account_name ON tag (account_id, lower(name));

ALTER TABLE promotion_tier
  ADD CONSTRAINT uq_promotion_tier_from_quantity UNIQUE (promotion_id, from_quantity);

-- A condition item names one target and three of its four key columns are null on any given row,
-- so the index has to compare nulls as equal or it bounds nothing.
ALTER TABLE promotion_condition_item
  ADD CONSTRAINT uq_promotion_condition_item_target
  UNIQUE NULLS NOT DISTINCT (promotion_id, product_id, family_id, subgroup_id);

-- +goose Down

-- The duplicates the Up removed are not restored.
ALTER TABLE promotion_condition_item DROP CONSTRAINT uq_promotion_condition_item_target;
ALTER TABLE promotion_tier DROP CONSTRAINT uq_promotion_tier_from_quantity;
DROP INDEX uq_tag_account_name;
