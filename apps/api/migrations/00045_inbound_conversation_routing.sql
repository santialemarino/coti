-- An inbound provider message either starts an order or continues a sent quote's conversation, so
-- the event reserves exactly one of the two.

-- +goose Up
ALTER TABLE quote_message
  ADD COLUMN other_live_quotes INTEGER NOT NULL DEFAULT 0,
  ADD CONSTRAINT ck_quote_message_other_live_quotes CHECK (other_live_quotes >= 0),
  ADD CONSTRAINT uq_quote_message_account_id UNIQUE (account_id, id);

CREATE UNIQUE INDEX uq_quote_message_client_action
  ON quote_message (client_action_id) WHERE client_action_id IS NOT NULL;

ALTER TABLE inbound_channel_message
  ALTER COLUMN rfq_id DROP NOT NULL,
  ADD COLUMN quote_message_id UUID,
  ADD CONSTRAINT ck_inbound_channel_message_target
    CHECK ((rfq_id IS NULL) <> (quote_message_id IS NULL)),
  ADD CONSTRAINT fk_inbound_channel_message_quote_message FOREIGN KEY (account_id, quote_message_id)
    REFERENCES quote_message (account_id, id);

CREATE UNIQUE INDEX uq_inbound_channel_message_quote_message
  ON inbound_channel_message (quote_message_id) WHERE quote_message_id IS NOT NULL;

CREATE INDEX idx_quote_send_channel_destination ON quote_send (channel_id, destination);

-- +goose Down
DROP INDEX idx_quote_send_channel_destination;
DROP INDEX uq_inbound_channel_message_quote_message;
DELETE FROM inbound_channel_message WHERE rfq_id IS NULL;
ALTER TABLE inbound_channel_message
  DROP CONSTRAINT fk_inbound_channel_message_quote_message,
  DROP CONSTRAINT ck_inbound_channel_message_target,
  DROP COLUMN quote_message_id,
  ALTER COLUMN rfq_id SET NOT NULL;
DROP INDEX uq_quote_message_client_action;
ALTER TABLE quote_message
  DROP CONSTRAINT uq_quote_message_account_id,
  DROP CONSTRAINT ck_quote_message_other_live_quotes,
  DROP COLUMN other_live_quotes;
