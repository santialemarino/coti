-- Raw inbound channel messages make webhook retries idempotent before RFQ processing starts.

-- +goose Up

CREATE TABLE inbound_channel_message (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id          UUID NOT NULL,
  branch_id           UUID NOT NULL,
  channel_id          UUID NOT NULL,
  rfq_id              UUID,
  external_message_id VARCHAR(255) NOT NULL,
  external_sender_id  VARCHAR(255) NOT NULL,
  body                TEXT NOT NULL,
  raw_payload         JSONB NOT NULL,
  received_at         TIMESTAMPTZ NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE inbound_channel_message ADD CONSTRAINT fk_inbound_channel_message_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE inbound_channel_message ADD CONSTRAINT fk_inbound_channel_message_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE inbound_channel_message ADD CONSTRAINT fk_inbound_channel_message_channel FOREIGN KEY (channel_id) REFERENCES channel(id);
ALTER TABLE inbound_channel_message ADD CONSTRAINT fk_inbound_channel_message_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id);

CREATE UNIQUE INDEX uq_inbound_channel_message_external
  ON inbound_channel_message (channel_id, external_message_id);
CREATE INDEX idx_inbound_channel_message_rfq
  ON inbound_channel_message (rfq_id) WHERE rfq_id IS NOT NULL;

ALTER TABLE inbound_channel_message ENABLE ROW LEVEL SECURITY;
CREATE POLICY inbound_channel_message_account_isolation ON inbound_channel_message
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- +goose Down

DROP TABLE inbound_channel_message;
