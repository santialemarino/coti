-- Backfills the catalog search objects required by every supported database state.

-- +goose Up

CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose StatementBegin
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_ts_config config
    JOIN pg_namespace namespace ON namespace.oid = config.cfgnamespace
    WHERE namespace.nspname = 'public' AND config.cfgname = 'spanish_unaccent'
  ) THEN
    CREATE TEXT SEARCH CONFIGURATION public.spanish_unaccent (COPY = pg_catalog.spanish);
  END IF;
END
$$;
-- +goose StatementEnd

ALTER TEXT SEARCH CONFIGURATION public.spanish_unaccent
  ALTER MAPPING FOR hword, hword_part, word WITH unaccent, spanish_stem;

ALTER TABLE product
  ADD COLUMN IF NOT EXISTS embedding_updated_at TIMESTAMPTZ;

ALTER TABLE product
  ADD COLUMN IF NOT EXISTS search_document TSVECTOR
    GENERATED ALWAYS AS (
      to_tsvector('public.spanish_unaccent'::regconfig, canonical_name || ' ' || coalesce(description, ''))
    ) STORED;

ALTER TABLE product_synonym
  ADD COLUMN IF NOT EXISTS search_document TSVECTOR
    GENERATED ALWAYS AS (to_tsvector('public.spanish_unaccent'::regconfig, term)) STORED;

CREATE INDEX IF NOT EXISTS idx_product_search_document ON product USING GIN (search_document);
CREATE INDEX IF NOT EXISTS idx_product_synonym_search_document ON product_synonym USING GIN (search_document);

DROP TRIGGER IF EXISTS trg_product_updated ON product;
CREATE TRIGGER trg_product_updated BEFORE UPDATE ON product FOR EACH ROW
WHEN (
  OLD.code IS DISTINCT FROM NEW.code
  OR OLD.canonical_name IS DISTINCT FROM NEW.canonical_name
  OR OLD.description IS DISTINCT FROM NEW.description
  OR OLD.unit IS DISTINCT FROM NEW.unit
  OR OLD.family_id IS DISTINCT FROM NEW.family_id
  OR OLD.subgroup_id IS DISTINCT FROM NEW.subgroup_id
  OR OLD.is_active IS DISTINCT FROM NEW.is_active
)
EXECUTE FUNCTION set_updated_at();

-- +goose Down

-- These objects are owned by migration 00009 and must survive rolling this repair back.
SELECT 1;
