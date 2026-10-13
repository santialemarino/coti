-- +goose Up
CREATE TYPE arca_environment AS ENUM ('HOMOLOGATION', 'PRODUCTION');

-- Existing rows were all made in homologation. A constant default fills them without an UPDATE, so
-- the issued-invoice trigger never fires; dropping it makes every new row name its environment.
ALTER TABLE arca_setup ADD COLUMN environment arca_environment NOT NULL DEFAULT 'HOMOLOGATION';
ALTER TABLE arca_setup ALTER COLUMN environment DROP DEFAULT;
ALTER TABLE invoice ADD COLUMN environment arca_environment NOT NULL DEFAULT 'HOMOLOGATION';
ALTER TABLE invoice ALTER COLUMN environment DROP DEFAULT;

-- ARCA numbers each environment on its own, so the same number is legitimately issued in both.
DROP INDEX uq_invoice_number;
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, environment, issuer_cuit, point_of_sale, invoice_type, number)
  WHERE number IS NOT NULL;
DROP INDEX uq_invoice_pending_series;
CREATE UNIQUE INDEX uq_invoice_pending_series ON invoice(account_id, environment, issuer_cuit, point_of_sale, invoice_type)
  WHERE status = 'PENDING';

-- +goose Down
DROP INDEX uq_invoice_pending_series;
CREATE UNIQUE INDEX uq_invoice_pending_series ON invoice(account_id, issuer_cuit, point_of_sale, invoice_type) WHERE status = 'PENDING';
DROP INDEX uq_invoice_number;
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, issuer_cuit, point_of_sale, invoice_type, number) WHERE number IS NOT NULL;
ALTER TABLE invoice DROP COLUMN environment;
ALTER TABLE arca_setup DROP COLUMN environment;
DROP TYPE arca_environment;
