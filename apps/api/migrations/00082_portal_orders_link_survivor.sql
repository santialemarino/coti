-- +goose Up
-- Portal orders whose customer profile was merged were left without a client; they belong to the
-- profile it was merged into.
UPDATE rfq r SET client_id=COALESCE(c.merged_into,c.id) FROM inbound_channel_message m
JOIN customer_identity i ON i.account_id=m.account_id AND i.id::text=m.payload->>'customer_id'
JOIN client c ON c.account_id=i.account_id AND c.tax_id=i.tax_id
JOIN channel ch ON ch.account_id=m.account_id AND ch.branch_id=m.branch_id AND ch.id=m.channel_id AND ch.type='WEBAPP'
WHERE r.account_id=m.account_id AND r.branch_id=m.branch_id AND r.id=m.rfq_id AND r.client_id IS NULL;
UPDATE quote q SET client_id=r.client_id FROM rfq r
WHERE q.account_id=r.account_id AND q.branch_id=r.branch_id AND q.rfq_id=r.id AND q.client_id IS NULL AND r.client_id IS NOT NULL;

-- +goose Down
-- The links are correct data; nothing to undo.
SELECT 1;
