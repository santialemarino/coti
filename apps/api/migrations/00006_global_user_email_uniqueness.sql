-- +goose Up

-- Login resolves a user by email alone, so an address has to identify exactly one of them.
-- uq_app_user_email is per account, which left two accounts free to share an address and made
-- the login lookup non-deterministic: one of the two users could never reach their account.

-- Nothing shipped can produce a duplicate — registration refuses a taken address and the
-- account-scoped constraint covered the rest — but this also runs on local volumes edited by
-- hand, where the alternative is a bare "could not create unique index" with no hint at the
-- cause. The oldest row keeps the address; the rest are parked rather than deleted, so an
-- administrator sees what happened instead of a user silently losing their login.
UPDATE app_user AS u
SET email = 'duplicate.' || u.id || '.' || u.email,
    is_active = FALSE
FROM (
  SELECT id,
         row_number() OVER (PARTITION BY lower(email) ORDER BY created_at, id) AS position
  FROM app_user
) AS ranked
WHERE ranked.id = u.id
  AND ranked.position > 1;

-- Functional, so case-insensitivity stops depending on the service lowercasing on write.
CREATE UNIQUE INDEX uq_app_user_email_global ON app_user (lower(email));

-- +goose Down

DROP INDEX IF EXISTS uq_app_user_email_global;
