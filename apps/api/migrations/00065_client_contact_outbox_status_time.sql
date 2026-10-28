-- +goose Up
ALTER TABLE client_contact_outbox ADD COLUMN status_changed_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE client_contact_outbox DROP COLUMN status_changed_at;
