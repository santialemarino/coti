-- +goose Up
ALTER TABLE quote ADD COLUMN number BIGINT;

WITH numbered AS (
  SELECT id, row_number() OVER (PARTITION BY account_id ORDER BY created_at, id) AS number
  FROM quote
)
UPDATE quote
SET number = numbered.number
FROM numbered
WHERE quote.id = numbered.id;

ALTER TABLE quote
  ALTER COLUMN number SET NOT NULL,
  ADD CONSTRAINT ck_quote_number CHECK (number > 0),
  ADD CONSTRAINT uq_quote_account_number UNIQUE (account_id, number),
  ADD CONSTRAINT uq_quote_tenant_branch_id UNIQUE (account_id, branch_id, id);

CREATE TABLE quote_number_counter (
  account_id  UUID PRIMARY KEY,
  last_number BIGINT NOT NULL CHECK (last_number > 0),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO quote_number_counter (account_id, last_number)
SELECT account_id, max(number)
FROM quote
GROUP BY account_id;

ALTER TABLE quote_number_counter
  ADD CONSTRAINT fk_quote_number_counter_account FOREIGN KEY (account_id) REFERENCES account(id);

ALTER TABLE quote_version
  ADD COLUMN currency VARCHAR(8) NOT NULL DEFAULT 'ARS',
  ADD COLUMN frozen_at TIMESTAMPTZ;

UPDATE quote_version SET frozen_at = created_at WHERE is_immutable = TRUE;

ALTER TABLE quote_version
  ADD CONSTRAINT uq_quote_version_tenant_quote_id UNIQUE (account_id, quote_id, id),
  ADD CONSTRAINT ck_quote_version_currency CHECK (currency ~ '^[A-Z]{3}$'),
  ADD CONSTRAINT ck_quote_version_frozen_at CHECK (
    (is_immutable = TRUE AND frozen_at IS NOT NULL)
    OR (is_immutable = FALSE AND frozen_at IS NULL)
  );

CREATE TABLE quote_representation (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id         UUID NOT NULL,
  branch_id          UUID NOT NULL,
  quote_id           UUID NOT NULL,
  version_id         UUID NOT NULL,
  schema_version     SMALLINT NOT NULL DEFAULT 1,
  payload            JSONB NOT NULL,
  message            TEXT NOT NULL,
  pdf_storage_key    TEXT NOT NULL,
  pdf_content_type   VARCHAR(64) NOT NULL,
  pdf_size_bytes     BIGINT NOT NULL,
  pdf_sha256         CHAR(64) NOT NULL,
  logo_fallback_used BOOLEAN NOT NULL DEFAULT FALSE,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_representation_version UNIQUE (account_id, version_id),
  CONSTRAINT ck_quote_representation_schema CHECK (schema_version = 1),
  CONSTRAINT ck_quote_representation_payload CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT ck_quote_representation_message CHECK (length(message) > 0),
  CONSTRAINT ck_quote_representation_pdf_type CHECK (pdf_content_type = 'application/pdf'),
  CONSTRAINT ck_quote_representation_pdf_size CHECK (pdf_size_bytes > 0),
  CONSTRAINT ck_quote_representation_pdf_sha256 CHECK (pdf_sha256 ~ '^[0-9a-f]{64}$')
);

ALTER TABLE quote_representation
  ADD CONSTRAINT fk_quote_representation_account FOREIGN KEY (account_id) REFERENCES account(id),
  ADD CONSTRAINT fk_quote_representation_branch FOREIGN KEY (branch_id) REFERENCES branch(id),
  ADD CONSTRAINT fk_quote_representation_quote FOREIGN KEY (account_id, branch_id, quote_id) REFERENCES quote(account_id, branch_id, id),
  ADD CONSTRAINT fk_quote_representation_version FOREIGN KEY (account_id, quote_id, version_id) REFERENCES quote_version(account_id, quote_id, id);

CREATE INDEX idx_quote_representation_quote ON quote_representation(account_id, branch_id, quote_id);

ALTER TABLE quote_number_counter ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_number_counter_account_isolation ON quote_number_counter
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE quote_representation ENABLE ROW LEVEL SECURITY;
CREATE POLICY quote_representation_account_isolation ON quote_representation
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON quote_number_counter TO coti_app;
REVOKE UPDATE, DELETE ON quote_representation FROM coti_app;
GRANT SELECT, INSERT ON quote_representation TO coti_app;

-- +goose Down
DROP TABLE IF EXISTS quote_representation;

ALTER TABLE quote_version
  DROP CONSTRAINT IF EXISTS uq_quote_version_tenant_quote_id,
  DROP CONSTRAINT IF EXISTS ck_quote_version_frozen_at,
  DROP CONSTRAINT IF EXISTS ck_quote_version_currency,
  DROP COLUMN IF EXISTS frozen_at,
  DROP COLUMN IF EXISTS currency;

DROP TABLE IF EXISTS quote_number_counter;

ALTER TABLE quote
  DROP CONSTRAINT IF EXISTS uq_quote_tenant_branch_id,
  DROP CONSTRAINT IF EXISTS uq_quote_account_number,
  DROP CONSTRAINT IF EXISTS ck_quote_number,
  DROP COLUMN IF EXISTS number;
