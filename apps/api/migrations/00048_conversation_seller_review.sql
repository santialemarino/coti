-- The seller's review of the conversation handler: each decision keeps the draft as it was around
-- the AI's change, so the seller's own edits can be told apart, and a failed batch says why it
-- failed, so only one a retry can fix is offered again.

-- +goose Up
CREATE TYPE message_batch_failure_reason AS ENUM ('UNPROCESSABLE', 'ATTEMPTS_EXHAUSTED');

ALTER TABLE message_batch ADD COLUMN failure_reason message_batch_failure_reason;

UPDATE message_batch
SET failure_reason = CASE
  WHEN last_error LIKE 'conversation batch cannot be processed%' THEN 'UNPROCESSABLE'
  ELSE 'ATTEMPTS_EXHAUSTED' END::message_batch_failure_reason
WHERE status = 'FAILED';

ALTER TABLE message_batch ADD CONSTRAINT ck_message_batch_failure_reason
  CHECK ((status = 'FAILED') = (failure_reason IS NOT NULL));

ALTER TABLE handler_decision
  ADD COLUMN draft_before JSONB,
  ADD COLUMN draft_after JSONB;

-- +goose Down
ALTER TABLE handler_decision
  DROP COLUMN draft_after,
  DROP COLUMN draft_before;
ALTER TABLE message_batch
  DROP CONSTRAINT ck_message_batch_failure_reason,
  DROP COLUMN failure_reason;
DROP TYPE message_batch_failure_reason;
