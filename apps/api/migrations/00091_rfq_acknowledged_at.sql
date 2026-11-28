-- When the client was told their order arrived. Claimed once, so a retried delivery or a second
-- message joining the same order never sends the acknowledgement again.

-- +goose Up
ALTER TABLE rfq ADD COLUMN acknowledged_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE rfq DROP COLUMN acknowledged_at;
