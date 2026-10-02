-- +goose Up
ALTER TABLE quote_send
  ADD COLUMN idempotency_key UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN destination VARCHAR(255),
  ADD COLUMN provider_reference VARCHAR(255),
  ADD COLUMN validity_days INTEGER NOT NULL DEFAULT 7,
  ADD CONSTRAINT ck_quote_send_validity_days CHECK (validity_days BETWEEN 1 AND 365),
  ADD CONSTRAINT uq_quote_send_idempotency_channel
    UNIQUE (account_id, idempotency_key, channel_id);

CREATE UNIQUE INDEX uq_quote_send_channel_provider_reference
  ON quote_send (channel_id, provider_reference)
  WHERE provider_reference IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_quote_send_channel_provider_reference;

ALTER TABLE quote_send
  DROP CONSTRAINT IF EXISTS uq_quote_send_idempotency_channel,
  DROP CONSTRAINT IF EXISTS ck_quote_send_validity_days,
  DROP COLUMN IF EXISTS validity_days,
  DROP COLUMN IF EXISTS provider_reference,
  DROP COLUMN IF EXISTS destination,
  DROP COLUMN IF EXISTS idempotency_key;
