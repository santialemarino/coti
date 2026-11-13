-- +goose Up
SELECT assert_migration_sees_all_rows('quote_item_alternative');

-- A line holds at most one economy and one premium option. Which of two to keep is a commercial
-- call, so duplicates stop the migration instead of being resolved here.
-- +goose StatementBegin
DO $$
DECLARE
  duplicated INTEGER;
BEGIN
  SELECT count(*) INTO duplicated FROM (
    SELECT 1 FROM quote_item_alternative
    WHERE recommendation_type IN ('ECONOMY', 'PREMIUM')
    GROUP BY quote_item_id, recommendation_type
    HAVING count(*) > 1
  ) slots;
  IF duplicated > 0 THEN
    RAISE EXCEPTION '% quote item slots hold more than one ECONOMY or PREMIUM option', duplicated
      USING HINT = 'Keep one option per quote_item_id and recommendation_type, then rerun the migration.';
  END IF;
END;
$$;
-- +goose StatementEnd

CREATE UNIQUE INDEX uq_quote_item_alternative_recommendation
  ON quote_item_alternative (quote_item_id, recommendation_type)
  WHERE recommendation_type IN ('ECONOMY', 'PREMIUM');

-- +goose Down
DROP INDEX uq_quote_item_alternative_recommendation;
