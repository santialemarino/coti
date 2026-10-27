-- +goose Up
-- Seller reports read a period across orders, quotes, status changes and accepted versions; these
-- indexes keep those scans inside the caller's account and branch.
CREATE INDEX idx_rfq_account_branch_received
  ON rfq(account_id, branch_id, received_at);
CREATE INDEX idx_quote_account_branch_created
  ON quote(account_id, branch_id, created_at);
CREATE INDEX idx_quote_status_change_report
  ON quote_status_change(account_id, new_status, changed_at, quote_id)
  WHERE new_status IN ('SENT', 'ACCEPTED');
CREATE INDEX idx_quote_send_report
  ON quote_send(account_id, version_id, sent_at DESC)
  WHERE tracking_status IN ('SENT', 'DELIVERED', 'VIEWED');
CREATE INDEX idx_client_action_report
  ON client_action(account_id, version_id, created_at DESC)
  WHERE type = 'ACCEPT';
CREATE INDEX idx_quote_ai_generation_account_quote_created
  ON quote_ai_generation(account_id, quote_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_quote_ai_generation_account_quote_created;
DROP INDEX IF EXISTS idx_client_action_report;
DROP INDEX IF EXISTS idx_quote_send_report;
DROP INDEX IF EXISTS idx_quote_status_change_report;
DROP INDEX IF EXISTS idx_quote_account_branch_created;
DROP INDEX IF EXISTS idx_rfq_account_branch_received;
