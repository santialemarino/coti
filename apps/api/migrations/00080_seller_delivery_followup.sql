-- +goose Up
CREATE TYPE quote_delivery_purpose AS ENUM ('QUOTE', 'FOLLOWUP');
ALTER TABLE account ADD COLUMN followup_template TEXT CHECK (followup_template IS NULL OR length(followup_template) BETWEEN 1 AND 2000);
ALTER TABLE rfq ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE quote_send ADD COLUMN purpose quote_delivery_purpose NOT NULL DEFAULT 'QUOTE';
ALTER TABLE quote_send ADD COLUMN message TEXT;
ALTER TABLE quote_send DROP CONSTRAINT uq_quote_send_idempotency_channel;
ALTER TABLE quote_send ADD CONSTRAINT uq_quote_send_idempotency_channel UNIQUE (account_id, idempotency_key, channel_id, destination);

UPDATE rfq r SET client_id=c.id FROM inbound_channel_message m
JOIN customer_identity i ON i.account_id=m.account_id AND i.id::text=m.payload->>'customer_id'
JOIN client c ON c.account_id=i.account_id AND c.tax_id=i.tax_id AND c.merged_into IS NULL
JOIN channel ch ON ch.account_id=m.account_id AND ch.branch_id=m.branch_id AND ch.id=m.channel_id AND ch.type='WEBAPP'
WHERE r.account_id=m.account_id AND r.branch_id=m.branch_id AND r.id=m.rfq_id AND r.client_id IS NULL;
UPDATE quote q SET client_id=r.client_id FROM rfq r
WHERE q.account_id=r.account_id AND q.branch_id=r.branch_id AND q.rfq_id=r.id AND q.client_id IS NULL AND r.client_id IS NOT NULL;

-- +goose Down
ALTER TABLE quote_send DROP CONSTRAINT uq_quote_send_idempotency_channel;
ALTER TABLE quote_send ADD CONSTRAINT uq_quote_send_idempotency_channel UNIQUE (account_id,idempotency_key,channel_id);
ALTER TABLE quote_send DROP COLUMN message, DROP COLUMN purpose;
ALTER TABLE rfq DROP COLUMN archived_at;
ALTER TABLE account DROP COLUMN followup_template;
DROP TYPE quote_delivery_purpose;
