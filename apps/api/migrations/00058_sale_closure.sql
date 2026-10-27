-- +goose Up
-- Closing an accepted sale: who marked it paid and who said it needs no invoice, and when. Both are
-- facts on the sale, not statuses.
ALTER TABLE app_user ADD CONSTRAINT uq_app_user_account_id UNIQUE (account_id, id);

ALTER TABLE quote
  ADD COLUMN paid_at                   TIMESTAMPTZ,
  ADD COLUMN paid_by_user_id           UUID,
  ADD COLUMN invoice_waived_at         TIMESTAMPTZ,
  ADD COLUMN invoice_waived_by_user_id UUID,
  ADD CONSTRAINT ck_quote_paid CHECK ((paid_at IS NULL) = (paid_by_user_id IS NULL)),
  ADD CONSTRAINT ck_quote_invoice_waived
    CHECK ((invoice_waived_at IS NULL) = (invoice_waived_by_user_id IS NULL)),
  ADD CONSTRAINT fk_quote_paid_by FOREIGN KEY (account_id, paid_by_user_id)
    REFERENCES app_user(account_id, id),
  ADD CONSTRAINT fk_quote_invoice_waived_by FOREIGN KEY (account_id, invoice_waived_by_user_id)
    REFERENCES app_user(account_id, id);

-- Whether «Marcar cobrado» starts with «Facturar también» ticked.
ALTER TABLE account ADD COLUMN invoice_on_payment BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE account DROP COLUMN invoice_on_payment;

ALTER TABLE quote
  DROP CONSTRAINT fk_quote_invoice_waived_by,
  DROP CONSTRAINT fk_quote_paid_by,
  DROP CONSTRAINT ck_quote_invoice_waived,
  DROP CONSTRAINT ck_quote_paid,
  DROP COLUMN invoice_waived_by_user_id,
  DROP COLUMN invoice_waived_at,
  DROP COLUMN paid_by_user_id,
  DROP COLUMN paid_at;

ALTER TABLE app_user DROP CONSTRAINT uq_app_user_account_id;
