-- An admin-created user can be invited instead of handed a password: the invite link lets them
-- choose their own, and is single-use and expiring like the other two links.

-- +goose Up
ALTER TYPE auth_token_type ADD VALUE IF NOT EXISTS 'INVITE';

-- +goose Down
-- Postgres cannot drop an enum value, so the type is rebuilt without it. An invite still
-- outstanding is lost with it; the admin can hand that user a password instead.
DELETE FROM auth_token WHERE type = 'INVITE';

ALTER TYPE auth_token_type RENAME TO auth_token_type_old;

CREATE TYPE auth_token_type AS ENUM ('PASSWORD_RESET', 'EMAIL_VERIFICATION');

ALTER TABLE auth_token
  ALTER COLUMN type TYPE auth_token_type USING type::text::auth_token_type;

DROP TYPE auth_token_type_old;
