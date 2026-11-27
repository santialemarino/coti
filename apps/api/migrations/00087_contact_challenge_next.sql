-- The portal page a contact verification returns to, so an emailed link leads back to the quote the
-- customer signed up for. Only the server reads it; it never travels in the link.

-- +goose Up

ALTER TABLE client_contact_challenge ADD COLUMN next_path VARCHAR(255);

-- +goose Down

ALTER TABLE client_contact_challenge DROP COLUMN next_path;
