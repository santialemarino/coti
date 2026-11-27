-- What an acceptance read from a customer's reply needs: the account's switch, the mark that it
-- waits for the seller's confirmation, a way to revoke the customer action it recorded, and, to
-- accept the sent version of a quote with an open draft, a way to discard that draft's preview.

-- +goose Up

ALTER TABLE account ADD COLUMN whatsapp_auto_accept BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE quote ADD COLUMN acceptance_unconfirmed_at TIMESTAMPTZ;
ALTER TABLE quote ADD CONSTRAINT ck_quote_acceptance_unconfirmed
  CHECK (acceptance_unconfirmed_at IS NULL OR current_status = 'ACCEPTED');
CREATE INDEX idx_quote_acceptance_unconfirmed ON quote (branch_id)
  WHERE acceptance_unconfirmed_at IS NOT NULL;

ALTER TABLE client_action ADD COLUMN revoked_at TIMESTAMPTZ;
ALTER TABLE client_action ADD COLUMN message_batch_id UUID;
ALTER TABLE client_action ADD CONSTRAINT fk_client_action_batch
  FOREIGN KEY (message_batch_id) REFERENCES message_batch(id);

-- A frozen version's representation stays immutable; only a draft's preview may be deleted.
GRANT DELETE ON quote_representation TO coti_app;
CREATE POLICY quote_representation_draft_delete ON quote_representation AS RESTRICTIVE FOR DELETE
  USING (EXISTS (SELECT 1 FROM quote_version version
    WHERE version.account_id = quote_representation.account_id
      AND version.id = quote_representation.version_id AND NOT version.is_immutable));

-- +goose Down

DROP POLICY quote_representation_draft_delete ON quote_representation;
REVOKE DELETE ON quote_representation FROM coti_app;

ALTER TABLE client_action DROP CONSTRAINT fk_client_action_batch;
ALTER TABLE client_action DROP COLUMN message_batch_id;
ALTER TABLE client_action DROP COLUMN revoked_at;

DROP INDEX idx_quote_acceptance_unconfirmed;
ALTER TABLE quote DROP CONSTRAINT ck_quote_acceptance_unconfirmed;
ALTER TABLE quote DROP COLUMN acceptance_unconfirmed_at;

ALTER TABLE account DROP COLUMN whatsapp_auto_accept;
