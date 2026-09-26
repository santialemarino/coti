-- +goose Up
INSERT INTO tag (account_id, name)
SELECT account.id, defaults.name
FROM account
CROSS JOIN (VALUES ('Recurrente'), ('Obra grande')) AS defaults(name)
ON CONFLICT (account_id, lower(name)) DO NOTHING;

-- +goose Down
-- Keep the rows: an existing account may have created a same-named tag before this migration,
-- and there is no provenance column that would make deleting only seeded rows safe.
SELECT 1;
