-- Retains one provider event for each inbound message before the RFQ pipeline runs, so retries
-- can resume its reserved RFQ without creating another order.

-- +goose Up
CREATE TABLE inbound_channel_message (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id           UUID NOT NULL,
  branch_id            UUID NOT NULL,
  channel_id           UUID NOT NULL,
  rfq_id               UUID NOT NULL,
  external_message_id  VARCHAR(255) NOT NULL,
  sender_id            VARCHAR(255) NOT NULL,
  sender_label         VARCHAR(255),
  body                 TEXT NOT NULL,
  payload              JSONB NOT NULL,
  provider_received_at TIMESTAMPTZ NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_inbound_channel_message_external UNIQUE (channel_id, external_message_id),
  CONSTRAINT fk_inbound_channel_message_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_inbound_channel_message_branch FOREIGN KEY (branch_id) REFERENCES branch(id),
  CONSTRAINT fk_inbound_channel_message_channel FOREIGN KEY (channel_id) REFERENCES channel(id)
);

CREATE INDEX idx_inbound_channel_message_rfq ON inbound_channel_message (rfq_id);

ALTER TABLE inbound_channel_message ENABLE ROW LEVEL SECURITY;

CREATE POLICY inbound_channel_message_account_isolation ON inbound_channel_message
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- +goose Down
DROP TABLE IF EXISTS inbound_channel_message;
