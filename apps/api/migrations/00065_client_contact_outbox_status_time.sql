-- +goose Up
SELECT assert_migration_sees_all_rows('client_contact_outbox');
ALTER TABLE client_contact_outbox ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT now();
-- The column default would date every existing prompt to the deploy and reopen its reply window.
UPDATE client_contact_outbox SET status_changed_at=created_at;

-- +goose Down
ALTER TABLE client_contact_outbox DROP COLUMN status_changed_at;
