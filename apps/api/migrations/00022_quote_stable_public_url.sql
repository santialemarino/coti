-- +goose Up
-- Every quote gets one stable public URL for its whole life, minted when the quote reaches
-- QUOTED (materials accepted). The seller copies that link from the backoffice card immediately;
-- it reflects the quote's live content while QUOTED and becomes pinned to the frozen version
-- the moment the quote is sent AND kept for the version that was actually delivered.
--
-- Why not reuse quote_send.public_token: a send token is minted per channel and per delivery, so
-- sending to a second channel would mint a second token and the URL would change. A seller shares
-- "el link de mi cotización" with a customer before any send exists, and that link must keep
-- working (same one) after delivery.
--
-- Rules:
--   * public_token is minted once at QUOTED, never re-minted/invalidated afterwards.
--   * public_pinned_version_id is NULL while the quote is editable (QUOTED, no send yet); it
--     points at the frozen quote_version once a send freezes the content.
--   * The stable resolver serves the LIVE current version while unpinned and the FROZEN version
--     once pinnedjem (the one a customer actually saw).
ALTER TABLE quote
  ADD COLUMN public_token            VARCHAR(255),
  ADD COLUMN public_pinned_version_id UUID;

CREATE UNIQUE INDEX uq_quote_public_token
  ON quote (public_token)
  WHERE public_token IS NOT NULL;

ALTER TABLE quote
  ADD CONSTRAINT uq_quote_public_pinned_version_id
    FOREIGN KEY (account_id, public_pinned_version_id)
    REFERENCES quote_version (account_id, id);

-- +goose Down
ALTER TABLE quote
  DROP CONSTRAINT IF EXISTS uq_quote_public_pinned_version_id;

DROP INDEX IF EXISTS uq_quote_public_token;

ALTER TABLE quote
  DROP COLUMN IF EXISTS public_token,
  DROP COLUMN IF EXISTS public_pinned_version_id;
