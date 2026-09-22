-- +goose Up
-- Reserve a quote-level public token and an optional frozen-version pin for a future stable URL.
-- This migration does not mint tokens or change the current per-send public resolver.
--
-- A send token is minted per channel and delivery. A stable quote URL will need its own token
-- and must pin to a version of the same account and quote after a successful send.
ALTER TABLE quote
  ADD COLUMN public_token            VARCHAR(255),
  ADD COLUMN public_pinned_version_id UUID;

CREATE UNIQUE INDEX uq_quote_public_token
  ON quote (public_token)
  WHERE public_token IS NOT NULL;

ALTER TABLE quote
  ADD CONSTRAINT fk_quote_public_pinned_version
    FOREIGN KEY (account_id, id, public_pinned_version_id)
    REFERENCES quote_version (account_id, quote_id, id);

-- +goose Down
ALTER TABLE quote
  DROP CONSTRAINT IF EXISTS fk_quote_public_pinned_version;

DROP INDEX IF EXISTS uq_quote_public_token;

ALTER TABLE quote
  DROP COLUMN IF EXISTS public_token,
  DROP COLUMN IF EXISTS public_pinned_version_id;
