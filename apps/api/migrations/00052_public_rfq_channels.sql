-- +goose Up
INSERT INTO channel (account_id, branch_id, type)
SELECT account_id, id, 'WEBAPP'
FROM branch
ON CONFLICT DO NOTHING;

-- +goose Down
-- Public intake channels remain to preserve request history.
SELECT 1;
