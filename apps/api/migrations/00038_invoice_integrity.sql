-- +goose Up
ALTER TABLE invoice ADD CONSTRAINT chk_invoice_amounts CHECK (net_amount >= 0 AND exempt_amount >= 0 AND vat_amount >= 0 AND total > 0 AND total = net_amount + exempt_amount + vat_amount);
ALTER TABLE invoice ADD CONSTRAINT chk_invoice_identifiers CHECK (point_of_sale BETWEEN 1 AND 99999 AND issuer_cuit ~ '^[0-9]{11}$' AND (number IS NULL OR number > 0) AND (claimed_number IS NULL OR claimed_number > 0) AND (cae IS NULL OR cae ~ '^[0-9]{14}$'));
DROP INDEX uq_invoice_number;
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, issuer_cuit, point_of_sale, invoice_type, number) WHERE number IS NOT NULL;
-- +goose StatementBegin
CREATE FUNCTION protect_issued_invoice() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status = 'ISSUED' THEN RAISE EXCEPTION 'Authorized invoices are immutable'; END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER trg_invoice_immutable BEFORE UPDATE ON invoice FOR EACH ROW EXECUTE FUNCTION protect_issued_invoice();

-- +goose Down
DROP TRIGGER trg_invoice_immutable ON invoice;
DROP FUNCTION protect_issued_invoice();
ALTER TABLE invoice DROP CONSTRAINT chk_invoice_amounts, DROP CONSTRAINT chk_invoice_identifiers;
DROP INDEX uq_invoice_number;
CREATE UNIQUE INDEX uq_invoice_number ON invoice(account_id, point_of_sale, invoice_type, number) WHERE number IS NOT NULL;
