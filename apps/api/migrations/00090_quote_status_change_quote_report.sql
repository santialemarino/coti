-- +goose Up
-- Per-quote event lookups also need quote_id before the time range in generic plans.
CREATE INDEX idx_quote_status_change_quote_report
  ON quote_status_change(account_id, quote_id, new_status, changed_at)
  WHERE new_status IN ('SENT', 'ACCEPTED');

-- +goose Down
DROP INDEX IF EXISTS idx_quote_status_change_quote_report;
