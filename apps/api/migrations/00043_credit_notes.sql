-- Credit notes: a voucher that cancels one issued invoice in full. It shares the invoice's lifecycle,
-- recovery and immutability, so it is a row of the same table with its own kind.

-- +goose Up
CREATE TYPE voucher_kind AS ENUM ('INVOICE', 'CREDIT_NOTE');

-- A constant default fills the existing rows without an UPDATE, so the issued-invoice trigger never fires.
ALTER TABLE invoice ADD COLUMN kind voucher_kind NOT NULL DEFAULT 'INVOICE';
ALTER TABLE invoice ALTER COLUMN kind DROP DEFAULT;
ALTER TABLE invoice
  -- On a credit note: the invoice it cancels.
  ADD COLUMN credited_invoice_id UUID,
  -- On an invoice issued after a credit note: the credited invoice it bills the sale again after.
  ADD COLUMN replaces_invoice_id UUID;

ALTER TABLE invoice ADD CONSTRAINT uq_invoice_link_tenant UNIQUE (account_id, quote_id, environment, id);
ALTER TABLE invoice
  ADD CONSTRAINT fk_invoice_credited FOREIGN KEY (account_id, quote_id, environment, credited_invoice_id)
    REFERENCES invoice(account_id, quote_id, environment, id),
  ADD CONSTRAINT fk_invoice_replaces FOREIGN KEY (account_id, quote_id, environment, replaces_invoice_id)
    REFERENCES invoice(account_id, quote_id, environment, id),
  ADD CONSTRAINT chk_invoice_links CHECK (
    (kind = 'CREDIT_NOTE' AND credited_invoice_id IS NOT NULL AND replaces_invoice_id IS NULL)
    OR (kind = 'INVOICE' AND credited_invoice_id IS NULL)
  );

-- One live invoice per sale and environment: each new one names the credited invoice it follows.
DROP INDEX uq_invoice_active_quote;
CREATE UNIQUE INDEX uq_invoice_active_quote ON invoice(quote_id, environment, replaces_invoice_id) NULLS NOT DISTINCT
  WHERE kind = 'INVOICE' AND status <> 'REJECTED';
-- One credit note per invoice: a pending one holds it, an issued one cancels it for good.
CREATE UNIQUE INDEX uq_invoice_credit_note ON invoice(credited_invoice_id, environment)
  WHERE kind = 'CREDIT_NOTE' AND status <> 'REJECTED';
-- ARCA numbers credit notes in a series of their own.
DROP INDEX uq_invoice_number;
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, environment, issuer_cuit, point_of_sale, kind, invoice_type, number)
  WHERE number IS NOT NULL;
DROP INDEX uq_invoice_pending_series;
CREATE UNIQUE INDEX uq_invoice_pending_series ON invoice(account_id, environment, issuer_cuit, point_of_sale, kind, invoice_type)
  WHERE status = 'PENDING';

-- +goose StatementBegin
CREATE FUNCTION check_invoice_links() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.credited_invoice_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM invoice WHERE id = NEW.credited_invoice_id AND kind = 'INVOICE' AND status = 'ISSUED'
  ) THEN
    RAISE EXCEPTION 'A credit note cancels an issued invoice';
  END IF;
  IF NEW.replaces_invoice_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM invoice WHERE credited_invoice_id = NEW.replaces_invoice_id AND kind = 'CREDIT_NOTE' AND status = 'ISSUED'
  ) THEN
    RAISE EXCEPTION 'An invoice replaces only an invoice an issued credit note cancelled';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER trg_invoice_links BEFORE INSERT OR UPDATE OF kind, credited_invoice_id, replaces_invoice_id ON invoice
  FOR EACH ROW EXECUTE FUNCTION check_invoice_links();

-- +goose Down
DROP TRIGGER trg_invoice_links ON invoice;
DROP FUNCTION check_invoice_links();
DROP INDEX uq_invoice_pending_series;
DROP INDEX uq_invoice_number;
DROP INDEX uq_invoice_credit_note;
DROP INDEX uq_invoice_active_quote;
ALTER TABLE invoice DROP CONSTRAINT chk_invoice_links, DROP CONSTRAINT fk_invoice_replaces, DROP CONSTRAINT fk_invoice_credited;
ALTER TABLE invoice DROP CONSTRAINT uq_invoice_link_tenant;
ALTER TABLE invoice DROP COLUMN replaces_invoice_id, DROP COLUMN credited_invoice_id, DROP COLUMN kind;
DROP TYPE voucher_kind;
CREATE UNIQUE INDEX uq_invoice_active_quote ON invoice(quote_id, environment) WHERE status <> 'REJECTED';
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, environment, issuer_cuit, point_of_sale, invoice_type, number)
  WHERE number IS NOT NULL;
CREATE UNIQUE INDEX uq_invoice_pending_series ON invoice(account_id, environment, issuer_cuit, point_of_sale, invoice_type)
  WHERE status = 'PENDING';
