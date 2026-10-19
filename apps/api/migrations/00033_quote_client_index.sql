-- The client profile reads every quote of one client on each load; this index answers the
-- account-scoped lookup without a sequential scan over the whole quote table.

-- +goose Up
CREATE INDEX idx_quote_client ON quote (account_id, client_id);

-- +goose Down
DROP INDEX idx_quote_client;
