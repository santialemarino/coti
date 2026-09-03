-- Materialize bounded, account-local correction patterns from seller-approved quotes.

-- +goose Up

CREATE TABLE quote_correction_memory (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id        UUID NOT NULL,
  kind              VARCHAR(32) NOT NULL CHECK (kind IN ('INTERPRETATION', 'CATALOG')),
  source_text       TEXT NOT NULL,
  normalized_source TEXT NOT NULL,
  embedding         VECTOR(1536),
  status            VARCHAR(16) NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING', 'READY')),
  corrected_items   JSONB,
  product_id        UUID,
  support_count     INTEGER NOT NULL DEFAULT 0 CHECK (support_count >= 0),
  use_count         INTEGER NOT NULL DEFAULT 0 CHECK (use_count >= 0),
  last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at      TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error        TEXT,
  CONSTRAINT ck_quote_correction_memory_payload CHECK (
    (kind = 'INTERPRETATION' AND corrected_items IS NOT NULL
      AND jsonb_typeof(corrected_items) = 'array' AND product_id IS NULL)
    OR
    (kind = 'CATALOG' AND corrected_items IS NULL AND product_id IS NOT NULL)
  ),
  CONSTRAINT ck_quote_correction_memory_ready CHECK (
    (status = 'PENDING' AND embedding IS NULL) OR
    (status = 'READY' AND embedding IS NOT NULL AND last_error IS NULL)
  ),
  CONSTRAINT fk_quote_correction_memory_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_quote_correction_memory_product FOREIGN KEY (product_id) REFERENCES product(id)
    ON DELETE CASCADE
);

CREATE TABLE quote_correction_memory_source (
  account_id    UUID NOT NULL,
  evaluation_id UUID NOT NULL,
  memory_id     UUID NOT NULL,
  source_key    VARCHAR(128) NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (evaluation_id, source_key),
  CONSTRAINT fk_quote_correction_memory_source_account FOREIGN KEY (account_id)
    REFERENCES account(id),
  CONSTRAINT fk_quote_correction_memory_source_evaluation FOREIGN KEY (evaluation_id)
    REFERENCES quote_quality_evaluation(id) ON DELETE CASCADE,
  CONSTRAINT fk_quote_correction_memory_source_memory FOREIGN KEY (memory_id)
    REFERENCES quote_correction_memory(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_quote_correction_catalog_pattern
  ON quote_correction_memory(account_id, normalized_source, product_id)
  WHERE kind = 'CATALOG';
CREATE UNIQUE INDEX uq_quote_correction_interpretation_pattern
  ON quote_correction_memory(account_id, normalized_source)
  WHERE kind = 'INTERPRETATION';
CREATE INDEX idx_quote_correction_memory_account_kind
  ON quote_correction_memory(account_id, kind);
CREATE INDEX idx_quote_correction_memory_pending
  ON quote_correction_memory(created_at, id) WHERE status = 'PENDING';
CREATE INDEX idx_quote_correction_memory_source_account
  ON quote_correction_memory_source(account_id, memory_id);

ALTER TABLE quote_correction_memory ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_correction_memory_account_isolation ON quote_correction_memory
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE quote_correction_memory_source ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_correction_memory_source_account_isolation ON quote_correction_memory_source
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON quote_correction_memory,
  quote_correction_memory_source TO coti_app;

-- +goose Down

DROP TABLE quote_correction_memory_source;
DROP TABLE quote_correction_memory;
