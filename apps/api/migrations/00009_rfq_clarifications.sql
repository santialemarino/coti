-- Coti - reviewable clarification questions for incomplete RFQs.

-- +goose Up

CREATE TYPE rfq_clarification_issue_type AS ENUM (
  'MISSING_QUANTITY',
  'MISSING_UNIT',
  'MISSING_PRESENTATION',
  'AMBIGUOUS_DESCRIPTION',
  'AMBIGUOUS_CATALOG_MATCH'
);

CREATE TYPE rfq_clarification_status AS ENUM (
  'PROPOSED',
  'APPROVED',
  'SENT',
  'ANSWERED',
  'DISMISSED'
);

CREATE TABLE rfq_clarification (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id            UUID NOT NULL,
  rfq_id                UUID NOT NULL,
  quote_item_id         UUID,
  issue_type            rfq_clarification_issue_type NOT NULL,
  requested_description VARCHAR(512) NOT NULL,
  question              VARCHAR(512) NOT NULL,
  reason                VARCHAR(512) NOT NULL,
  status                rfq_clarification_status NOT NULL DEFAULT 'PROPOSED',
  approved_question     VARCHAR(512),
  decided_by            UUID,
  decided_at            TIMESTAMPTZ,
  sent_at               TIMESTAMPTZ,
  answer                TEXT,
  answered_at           TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE rfq_clarification
  ADD CONSTRAINT fk_rfq_clarification_account FOREIGN KEY (account_id) REFERENCES account(id),
  ADD CONSTRAINT fk_rfq_clarification_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id),
  ADD CONSTRAINT fk_rfq_clarification_item FOREIGN KEY (quote_item_id) REFERENCES quote_item(id),
  ADD CONSTRAINT fk_rfq_clarification_decider FOREIGN KEY (decided_by) REFERENCES app_user(id);

CREATE INDEX idx_rfq_clarification_pending
  ON rfq_clarification(rfq_id, status)
  WHERE status IN ('PROPOSED', 'APPROVED', 'SENT');

GRANT SELECT, INSERT, UPDATE, DELETE ON rfq_clarification TO coti_app;
ALTER TABLE rfq_clarification ENABLE ROW LEVEL SECURITY;
CREATE POLICY rfq_clarification_account_isolation ON rfq_clarification
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- +goose Down

DROP TABLE rfq_clarification;
DROP TYPE rfq_clarification_status;
DROP TYPE rfq_clarification_issue_type;
