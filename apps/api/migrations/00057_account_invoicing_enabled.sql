-- +goose Up
-- Whether the corralón invoices with Coti. Accounts already on the way to ARCA start on.
ALTER TABLE account ADD COLUMN invoicing_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE account a SET invoicing_enabled = TRUE
WHERE EXISTS (SELECT 1 FROM arca_setup s WHERE s.account_id = a.id)
   OR EXISTS (SELECT 1 FROM invoice i WHERE i.account_id = a.id);

-- +goose Down
ALTER TABLE account DROP COLUMN invoicing_enabled;
