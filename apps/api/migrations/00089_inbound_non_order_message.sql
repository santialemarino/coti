-- A message that is not an order — a greeting, a question, an automated email — is kept for the
-- seller without opening an order. It carries the kind it was read as and points at neither an rfq
-- nor a quote conversation; a seller putting it away sets dismissed_at.

-- +goose Up
CREATE TYPE inbound_message_kind AS ENUM ('GREETING', 'QUESTION', 'OTHER', 'AUTOMATED');

ALTER TABLE inbound_channel_message
  ADD COLUMN kind inbound_message_kind,
  ADD COLUMN dismissed_at TIMESTAMPTZ,
  DROP CONSTRAINT ck_inbound_channel_message_target,
  ADD CONSTRAINT ck_inbound_channel_message_target CHECK (
    (rfq_id IS NOT NULL AND quote_message_id IS NULL AND kind IS NULL)
    OR (rfq_id IS NULL AND quote_message_id IS NOT NULL AND kind IS NULL)
    OR (rfq_id IS NULL AND quote_message_id IS NULL AND kind IS NOT NULL)),
  ADD CONSTRAINT ck_inbound_channel_message_dismissed
    CHECK (dismissed_at IS NULL OR kind IS NOT NULL);

CREATE INDEX idx_inbound_channel_message_open_non_order
  ON inbound_channel_message (account_id, branch_id, provider_received_at DESC)
  WHERE kind IS NOT NULL AND dismissed_at IS NULL;

-- +goose Down
DROP INDEX idx_inbound_channel_message_open_non_order;
DELETE FROM client_contact_observation
 WHERE message_id IN (SELECT id FROM inbound_channel_message WHERE kind IS NOT NULL);
DELETE FROM inbound_channel_message WHERE kind IS NOT NULL;
ALTER TABLE inbound_channel_message
  DROP CONSTRAINT ck_inbound_channel_message_dismissed,
  DROP CONSTRAINT ck_inbound_channel_message_target,
  ADD CONSTRAINT ck_inbound_channel_message_target
    CHECK ((rfq_id IS NULL) <> (quote_message_id IS NULL)),
  DROP COLUMN dismissed_at,
  DROP COLUMN kind;
DROP TYPE inbound_message_kind;
