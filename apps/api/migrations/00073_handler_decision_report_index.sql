-- +goose Up
-- The report reads an account's assistant decisions by period; the version index cannot serve that.
CREATE INDEX idx_handler_decision_account_created ON handler_decision(account_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_handler_decision_account_created;
