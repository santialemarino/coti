-- A wrong WhatsApp proof a customer sends names no challenge, only the number it came from; it still
-- counts toward that number's lockout.

-- +goose Up
ALTER TABLE client_contact_challenge_failure ALTER COLUMN challenge_id DROP NOT NULL;

-- +goose Down
DELETE FROM client_contact_challenge_failure WHERE challenge_id IS NULL;
ALTER TABLE client_contact_challenge_failure ALTER COLUMN challenge_id SET NOT NULL;
