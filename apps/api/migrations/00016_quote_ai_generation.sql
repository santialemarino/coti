-- Preserve the original AI proposal independently of the quote version the seller edits.

-- +goose Up

CREATE TABLE quote_ai_generation (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id         UUID NOT NULL,
  quote_id           UUID NOT NULL,
  quote_version_id   UUID NOT NULL,
  provider           VARCHAR(64) NOT NULL,
  model              VARCHAR(255) NOT NULL,
  prompt_version     VARCHAR(64) NOT NULL,
  schema_version     VARCHAR(64) NOT NULL,
  input_tokens       INTEGER NOT NULL CHECK (input_tokens >= 0),
  output_tokens      INTEGER NOT NULL CHECK (output_tokens >= 0),
  cache_read_tokens  INTEGER NOT NULL CHECK (cache_read_tokens >= 0),
  cache_write_tokens INTEGER NOT NULL CHECK (cache_write_tokens >= 0),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_ai_generation_version UNIQUE (quote_version_id),
  CONSTRAINT fk_quote_ai_generation_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_quote_ai_generation_quote FOREIGN KEY (quote_id) REFERENCES quote(id),
  CONSTRAINT fk_quote_ai_generation_version FOREIGN KEY (quote_version_id) REFERENCES quote_version(id)
);

CREATE TABLE quote_ai_generation_item (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id            UUID NOT NULL,
  generation_id         UUID NOT NULL,
  position              SMALLINT NOT NULL CHECK (position >= 0),
  source_quote_item_id  UUID NOT NULL,
  product_id            UUID,
  requested_description VARCHAR(512) NOT NULL,
  quantity              NUMERIC(14,2) NOT NULL,
  unit                  VARCHAR(64),
  quantity_source       VARCHAR(16) NOT NULL
                        CHECK (quantity_source IN ('EXPLICIT', 'DERIVED', 'UNRESOLVED')),
  quantity_rationale    VARCHAR(512) NOT NULL,
  match_status          item_match_status NOT NULL,
  confidence_score      NUMERIC(5,4),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_ai_generation_item_position UNIQUE (generation_id, position),
  CONSTRAINT fk_quote_ai_generation_item_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_quote_ai_generation_item_generation FOREIGN KEY (generation_id) REFERENCES quote_ai_generation(id),
  CONSTRAINT fk_quote_ai_generation_item_product FOREIGN KEY (product_id) REFERENCES product(id)
);

CREATE TABLE quote_quality_evaluation (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id             UUID NOT NULL,
  generation_id          UUID NOT NULL,
  final_quote_version_id UUID NOT NULL,
  evaluator_version      VARCHAR(64) NOT NULL,
  whole_quote_correct    BOOLEAN NOT NULL,
  same_item_count        BOOLEAN NOT NULL,
  all_items_equivalent   BOOLEAN NOT NULL,
  all_items_matched      BOOLEAN NOT NULL,
  all_items_priced       BOOLEAN NOT NULL,
  all_subtotals_valid    BOOLEAN NOT NULL,
  total_valid            BOOLEAN NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_quality_evaluation
    UNIQUE (generation_id, final_quote_version_id, evaluator_version),
  CONSTRAINT ck_quote_quality_whole_correct CHECK (
    whole_quote_correct = (
      same_item_count AND all_items_equivalent AND all_items_matched AND all_items_priced
      AND all_subtotals_valid AND total_valid
    )
  ),
  CONSTRAINT fk_quote_quality_evaluation_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_quote_quality_evaluation_generation FOREIGN KEY (generation_id) REFERENCES quote_ai_generation(id),
  CONSTRAINT fk_quote_quality_evaluation_version FOREIGN KEY (final_quote_version_id) REFERENCES quote_version(id)
);

CREATE TABLE quote_quality_difference (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id         UUID NOT NULL,
  evaluation_id      UUID NOT NULL,
  kind               VARCHAR(32) NOT NULL CHECK (kind IN (
                       'ITEM_REMOVED', 'ITEM_ADDED', 'FIELD_CHANGED', 'UNRESOLVED_MATCH',
                       'MISSING_PRICE', 'INVALID_SUBTOTAL', 'INVALID_TOTAL'
                     )),
  generation_item_id UUID,
  final_quote_item_id UUID,
  field              VARCHAR(64),
  expected_value     TEXT,
  actual_value       TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_quote_quality_difference_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_quote_quality_difference_evaluation FOREIGN KEY (evaluation_id) REFERENCES quote_quality_evaluation(id),
  CONSTRAINT fk_quote_quality_difference_generation_item FOREIGN KEY (generation_item_id) REFERENCES quote_ai_generation_item(id) ON DELETE SET NULL,
  CONSTRAINT fk_quote_quality_difference_final_item FOREIGN KEY (final_quote_item_id) REFERENCES quote_item(id) ON DELETE SET NULL
);

CREATE INDEX idx_quote_ai_generation_account_quote
  ON quote_ai_generation(account_id, quote_id);
CREATE INDEX idx_quote_ai_generation_item_account_generation
  ON quote_ai_generation_item(account_id, generation_id);
CREATE INDEX idx_quote_quality_evaluation_account_generation
  ON quote_quality_evaluation(account_id, generation_id);
CREATE INDEX idx_quote_quality_difference_account_evaluation
  ON quote_quality_difference(account_id, evaluation_id);

ALTER TABLE quote_ai_generation ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_ai_generation_account_isolation ON quote_ai_generation
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE quote_ai_generation_item ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_ai_generation_item_account_isolation ON quote_ai_generation_item
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE quote_quality_evaluation ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_quality_evaluation_account_isolation ON quote_quality_evaluation
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE quote_quality_difference ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_quality_difference_account_isolation ON quote_quality_difference
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

GRANT SELECT, INSERT ON quote_ai_generation, quote_ai_generation_item,
  quote_quality_evaluation, quote_quality_difference TO coti_app;
REVOKE UPDATE, DELETE ON quote_ai_generation, quote_ai_generation_item,
  quote_quality_evaluation, quote_quality_difference FROM coti_app;

-- +goose Down

DROP TABLE quote_quality_difference;
DROP TABLE quote_quality_evaluation;
DROP TABLE quote_ai_generation_item;
DROP TABLE quote_ai_generation;
