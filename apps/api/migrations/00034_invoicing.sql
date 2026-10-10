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
  ADD COLUMN prices_include_vat BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN invoice_profile JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(invoice_profile) = 'object');

ALTER TABLE product ADD COLUMN vat_rate vat_rate NOT NULL DEFAULT 'VAT_21';

ALTER TABLE client
  ADD COLUMN legal_name VARCHAR(255),
  -- Digits only: an 11-digit CUIT or a 7–8 digit DNI.
  ADD COLUMN tax_id VARCHAR(11) CHECK (tax_id ~ '^[0-9]{7,11}$'),
  ADD COLUMN iva_condition iva_condition;

-- One invoice per sale. A rejected attempt stays as a record and frees the quote for another try;
-- a pending one holds it while ARCA is being asked.
ALTER TABLE quote ADD CONSTRAINT uq_invoice_quote_tenant UNIQUE (account_id, branch_id, id);
ALTER TABLE quote_version ADD CONSTRAINT uq_invoice_version_tenant UNIQUE (account_id, quote_id, id);
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
  -- The number last requested from ARCA, written before the request: how an unknown outcome is checked.
  claimed_number         BIGINT,
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
  snapshot               JSONB NOT NULL DEFAULT '{}',
  issuer_profile         JSONB NOT NULL DEFAULT '{}',
  issuer_iva_condition   iva_condition NOT NULL DEFAULT 'REGISTERED',
  -- [{rate, base, amount}] as decimal strings, as authorized.
  vat_breakdown          JSONB NOT NULL DEFAULT '[]',
  issues                 JSONB NOT NULL DEFAULT '[]',
  arca_request           TEXT,
  arca_response          TEXT,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_invoice_account FOREIGN KEY (account_id) REFERENCES account(id),
  CONSTRAINT fk_invoice_branch FOREIGN KEY (account_id, branch_id) REFERENCES branch(account_id, id),
  CONSTRAINT fk_invoice_quote FOREIGN KEY (account_id, branch_id, quote_id) REFERENCES quote(account_id, branch_id, id),
  CONSTRAINT fk_invoice_quote_version FOREIGN KEY (account_id, quote_id, quote_version_id) REFERENCES quote_version(account_id, quote_id, id),
  CONSTRAINT chk_invoice_issued CHECK (
    status <> 'ISSUED' OR (number IS NOT NULL AND cae IS NOT NULL AND cae_expires_on IS NOT NULL)
  )
);

CREATE UNIQUE INDEX uq_invoice_active_quote ON invoice(quote_id) WHERE status <> 'REJECTED';
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, point_of_sale, invoice_type, number)
  WHERE number IS NOT NULL;
CREATE UNIQUE INDEX uq_invoice_pending_series ON invoice(account_id, issuer_cuit, point_of_sale, invoice_type) WHERE status = 'PENDING';
ALTER TABLE invoice FORCE ROW LEVEL SECURITY;
CREATE INDEX idx_invoice_account_quote ON invoice(account_id, quote_id);

CREATE TRIGGER trg_invoice_updated BEFORE UPDATE ON invoice
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE invoice ENABLE ROW LEVEL SECURITY;
CREATE POLICY invoice_account_isolation ON invoice
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- An authorized invoice is a fiscal record: it is corrected with a credit note, never deleted.
GRANT SELECT, INSERT, UPDATE ON invoice TO coti_app;
REVOKE DELETE ON invoice FROM coti_app;

-- +goose Down

DROP TABLE invoice;
ALTER TABLE quote_version DROP CONSTRAINT uq_invoice_version_tenant;
ALTER TABLE quote DROP CONSTRAINT uq_invoice_quote_tenant;
ALTER TABLE client DROP COLUMN iva_condition, DROP COLUMN tax_id, DROP COLUMN legal_name;
ALTER TABLE product DROP COLUMN vat_rate;
ALTER TABLE account DROP COLUMN invoice_profile, DROP COLUMN prices_include_vat, DROP COLUMN iva_condition;
DROP TYPE receiver_doc_type;
DROP TYPE invoice_status;
DROP TYPE invoice_type;
DROP TYPE vat_rate;
DROP TYPE iva_condition;
