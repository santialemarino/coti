-- The portal page a contact verification returns to, so an emailed link leads back to the quote the
-- customer signed up for, and when the proof succeeded: a newer request also consumes a proof, so only
-- proved_at says the customer actually proved the destination through it.

-- +goose Up

ALTER TABLE client_contact_challenge ADD COLUMN next_path VARCHAR(255);
ALTER TABLE client_contact_challenge ADD COLUMN proved_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE client_contact_challenge DROP COLUMN proved_at;
ALTER TABLE client_contact_challenge DROP COLUMN next_path;
