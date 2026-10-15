-- A closed batch is claimed, handed to the handler, and retried on failure up to a limit; a claim
-- whose processor died is taken back once processing_started_at is old enough.

-- +goose Up
ALTER TABLE message_batch
  ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN last_error TEXT,
  ADD COLUMN processing_started_at TIMESTAMPTZ,
  ADD CONSTRAINT ck_message_batch_attempts CHECK (attempts >= 0);

CREATE INDEX idx_message_batch_processing_started
  ON message_batch (processing_started_at) WHERE status = 'PROCESSING';

-- +goose Down
DROP INDEX idx_message_batch_processing_started;
ALTER TABLE message_batch
  DROP CONSTRAINT ck_message_batch_attempts,
  DROP COLUMN processing_started_at,
  DROP COLUMN last_error,
  DROP COLUMN attempts;
