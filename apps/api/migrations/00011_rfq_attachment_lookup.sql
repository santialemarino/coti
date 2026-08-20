-- The index the attachment list reads by. rfq_attachment carried only its primary key and a
-- partial index on processing_status, which serves the multi-format engine looking for work and
-- nothing that reads one order's files.

-- +goose Up

-- Account first, matching every other tenant-scoped lookup: the request predicate and the row
-- level security policy both filter on it, so a scan of another account's rows is never needed.
CREATE INDEX idx_rfq_attachment_account_rfq ON rfq_attachment(account_id, rfq_id);

-- +goose Down

DROP INDEX idx_rfq_attachment_account_rfq;
