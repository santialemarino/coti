-- What the unmatched-items report needs from an alternative: the order the matcher ranked the
-- candidates in, and what each one scored. Without them a line offers three product names with
-- no way to tell a near miss from a long shot.

-- +goose Up

-- The candidate's own place in the matcher's ranking, best first. Nothing else on the row records
-- it: created_at is the transaction's timestamp, shared by every row of one insert.
ALTER TABLE quote_item_alternative ADD COLUMN rank SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE quote_item_alternative ALTER COLUMN rank DROP DEFAULT;

-- Same scale as quote_item.confidence_score, and nullable for the same reason: a seller's own
-- alternative was never scored, and zero would read as one nothing came close to.
ALTER TABLE quote_item_alternative ADD COLUMN confidence_score NUMERIC(5,4);

-- Account first, matching every other tenant-scoped lookup: the request predicate and the row
-- level security policy both filter on it, so a scan of another account's rows is never needed.
CREATE INDEX idx_quote_item_alternative_account_item
  ON quote_item_alternative(account_id, quote_item_id);

-- +goose Down

DROP INDEX idx_quote_item_alternative_account_item;
ALTER TABLE quote_item_alternative DROP COLUMN confidence_score;
ALTER TABLE quote_item_alternative DROP COLUMN rank;
