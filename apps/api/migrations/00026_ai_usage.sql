-- Record what every AI provider call consumed, per account and operation, as an append-only ledger.

-- +goose Up

CREATE TYPE ai_operation AS ENUM (
  'RFQ_EXTRACTION',
  'AUDIO_TRANSCRIPTION',
  'INTERPRETATION_LOOKUP',
  'CATALOG_SEARCH',
  'CATALOG_MATCH_REVIEW',
  'CATALOG_EMBEDDING',
  'CORRECTION_LEARNING'
);

CREATE TABLE ai_usage (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id         UUID NOT NULL,
  branch_id          UUID,
  rfq_id             UUID,
  operation          ai_operation NOT NULL,
  provider           VARCHAR(64) NOT NULL,
  model              VARCHAR(255) NOT NULL,
  succeeded          BOOLEAN NOT NULL,
  attempts           SMALLINT NOT NULL CHECK (attempts >= 0),
  elapsed_ms         INTEGER NOT NULL CHECK (elapsed_ms >= 0),
  input_tokens       INTEGER NOT NULL CHECK (input_tokens >= 0),
  output_tokens      INTEGER NOT NULL CHECK (output_tokens >= 0),
  cache_read_tokens  INTEGER NOT NULL CHECK (cache_read_tokens >= 0),
  cache_write_tokens INTEGER NOT NULL CHECK (cache_write_tokens >= 0),
  audio_seconds      NUMERIC(10,2) NOT NULL CHECK (audio_seconds >= 0),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_ai_usage_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_ai_usage_branch FOREIGN KEY (branch_id) REFERENCES branch(id),
  -- The spend happened whatever becomes of the order it served.
  CONSTRAINT fk_ai_usage_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id) ON DELETE SET NULL
);

CREATE INDEX idx_ai_usage_account_created ON ai_usage(account_id, created_at);
CREATE INDEX idx_ai_usage_account_rfq ON ai_usage(account_id, rfq_id) WHERE rfq_id IS NOT NULL;

ALTER TABLE ai_usage ENABLE ROW LEVEL SECURITY;
CREATE POLICY ai_usage_account_isolation ON ai_usage
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

GRANT SELECT, INSERT ON ai_usage TO coti_app;
REVOKE UPDATE, DELETE ON ai_usage FROM coti_app;

-- +goose Down

DROP TABLE ai_usage;
DROP TYPE ai_operation;
