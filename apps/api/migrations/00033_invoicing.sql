-- Electronic invoicing with ARCA: the fiscal data an invoice needs and the invoices themselves.

-- +goose Up

CREATE TYPE iva_condition AS ENUM ('REGISTERED', 'MONOTRIBUTO', 'EXEMPT', 'FINAL_CONSUMER');
CREATE TYPE vat_rate AS ENUM ('VAT_0', 'VAT_2_5', 'VAT_5', 'VAT_10_5', 'VAT_21', 'VAT_27', 'EXEMPT');
CREATE TYPE invoice_type AS ENUM ('A', 'B', 'C');
CREATE TYPE invoice_status AS ENUM ('PENDING', 'ISSUED', 'REJECTED');
CREATE TYPE receiver_doc_type AS ENUM ('CUIT', 'DNI', 'NONE');

ALTER TABLE account
  ADD COLUMN iva_condition iva_condition,
  -- Whether the catalog's prices already carry IVA; the invoice splits them either way.
  ADD COLUMN prices_include_vat BOOLEAN NOT NULL DEFAULT FALSE;

-- ARCA numbers invoices per point of sale, and a point of sale belongs to an address.
ALTER TABLE branch ADD COLUMN point_of_sale INTEGER CHECK (point_of_sale BETWEEN 1 AND 99998);

ALTER TABLE product ADD COLUMN vat_rate vat_rate NOT NULL DEFAULT 'VAT_21';

ALTER TABLE client
  ADD COLUMN legal_name VARCHAR(255),
  -- Digits only: an 11-digit CUIT or a 7–8 digit DNI.
  ADD COLUMN tax_id VARCHAR(11) CHECK (tax_id ~ '^[0-9]{7,11}$'),
  ADD COLUMN iva_condition iva_condition;

-- One certificate per account. The key is sealed by the API before it reaches the database.
CREATE TABLE arca_credential (
  account_id         UUID PRIMARY KEY,
  certificate_pem    TEXT NOT NULL,
  private_key_sealed TEXT NOT NULL,
  cuit               VARCHAR(11) NOT NULL,
  subject            VARCHAR(512) NOT NULL,
  expires_at         TIMESTAMPTZ NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_arca_credential_account FOREIGN KEY (account_id) REFERENCES account(id)
);

-- One invoice per accepted quote version. A rejected attempt stays as a record and frees the
-- version for another try; a pending one holds it while ARCA is being asked.
CREATE TABLE invoice (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id             UUID NOT NULL,
  branch_id              UUID NOT NULL,
  quote_id               UUID NOT NULL,
  quote_version_id       UUID NOT NULL,
  status                 invoice_status NOT NULL DEFAULT 'PENDING',
  invoice_type           invoice_type NOT NULL,
  point_of_sale          INTEGER NOT NULL,
  number                 BIGINT,
  issued_on              DATE NOT NULL,
  cae                    VARCHAR(14),
  cae_expires_on         DATE,
  issuer_cuit            VARCHAR(11) NOT NULL,
  receiver_name          VARCHAR(255) NOT NULL,
  receiver_doc_type      receiver_doc_type NOT NULL,
  receiver_doc_number    VARCHAR(11),
  receiver_iva_condition iva_condition NOT NULL,
  net_amount             NUMERIC(14,2) NOT NULL,
  exempt_amount          NUMERIC(14,2) NOT NULL,
  vat_amount             NUMERIC(14,2) NOT NULL,
  total                  NUMERIC(14,2) NOT NULL,
  currency               CHAR(3) NOT NULL,
  -- [{rate, base, amount}] as decimal strings, as authorized.
  vat_breakdown          JSONB NOT NULL DEFAULT '[]',
  issues                 JSONB NOT NULL DEFAULT '[]',
  arca_request           TEXT,
  arca_response          TEXT,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_invoice_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_invoice_branch FOREIGN KEY (branch_id) REFERENCES branch(id),
  CONSTRAINT fk_invoice_quote FOREIGN KEY (quote_id) REFERENCES quote(id),
  CONSTRAINT fk_invoice_quote_version FOREIGN KEY (quote_version_id) REFERENCES quote_version(id),
  CONSTRAINT chk_invoice_issued CHECK (
    status <> 'ISSUED' OR (number IS NOT NULL AND cae IS NOT NULL AND cae_expires_on IS NOT NULL)
  )
);

CREATE UNIQUE INDEX uq_invoice_active_version ON invoice(quote_version_id) WHERE status <> 'REJECTED';
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, point_of_sale, invoice_type, number)
  WHERE number IS NOT NULL;
CREATE INDEX idx_invoice_account_quote ON invoice(account_id, quote_id);

CREATE TRIGGER trg_arca_credential_updated BEFORE UPDATE ON arca_credential
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_invoice_updated BEFORE UPDATE ON invoice
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE arca_credential ENABLE ROW LEVEL SECURITY;
CREATE POLICY arca_credential_account_isolation ON arca_credential
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

ALTER TABLE invoice ENABLE ROW LEVEL SECURITY;
CREATE POLICY invoice_account_isolation ON invoice
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- An authorized invoice is a fiscal record: it is corrected with a credit note, never deleted.
GRANT SELECT, INSERT, UPDATE, DELETE ON arca_credential TO coti_app;
GRANT SELECT, INSERT, UPDATE ON invoice TO coti_app;
REVOKE DELETE ON invoice FROM coti_app;

-- +goose Down

DROP TABLE invoice;
DROP TABLE arca_credential;
ALTER TABLE client DROP COLUMN iva_condition, DROP COLUMN tax_id, DROP COLUMN legal_name;
ALTER TABLE product DROP COLUMN vat_rate;
ALTER TABLE branch DROP COLUMN point_of_sale;
ALTER TABLE account DROP COLUMN prices_include_vat, DROP COLUMN iva_condition;
DROP TYPE receiver_doc_type;
DROP TYPE invoice_status;
DROP TYPE invoice_type;
DROP TYPE vat_rate;
DROP TYPE iva_condition;
