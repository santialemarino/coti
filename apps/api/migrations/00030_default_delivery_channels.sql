-- Gives every branch the channels new branches open with: manual entry, and the WhatsApp and email
-- routes a quote is sent through. A branch that already has a channel of the type, active or not,
-- keeps it alone.

-- +goose Up
INSERT INTO channel (account_id, branch_id, type)
SELECT b.account_id, b.id, defaults.type
FROM branch b
CROSS JOIN (VALUES ('MANUAL_ENTRY'::channel_type), ('WHATSAPP'::channel_type), ('EMAIL'::channel_type))
  AS defaults (type)
WHERE NOT EXISTS (
  SELECT 1 FROM channel c
  WHERE c.account_id = b.account_id AND c.branch_id = b.id AND c.type = defaults.type
)
ON CONFLICT DO NOTHING;

-- +goose Down
-- Keep the rows: sends reference them, and nothing marks which ones this migration opened.
SELECT 1;
