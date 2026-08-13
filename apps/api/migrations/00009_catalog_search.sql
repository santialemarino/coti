-- The lexical half of the catalog search, and the marker saying when a product was last embedded.
--
-- The approximate vector index is deliberately not here: built on an empty table it is degenerate,
-- so it is a start-up step run once the catalog is loaded and embedded (pnpm db:vector-index).

-- +goose Up

-- Informal RFQ text drops accents constantly, so "hormigon" has to reach "hormigón". The stock
-- spanish configuration keeps them, which would make that a miss.
CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE TEXT SEARCH CONFIGURATION spanish_unaccent (COPY = spanish);
ALTER TEXT SEARCH CONFIGURATION spanish_unaccent
  ALTER MAPPING FOR hword, hword_part, word WITH unaccent, spanish_stem;

ALTER TABLE product
  ADD COLUMN embedding_updated_at TIMESTAMPTZ,
  ADD COLUMN search_document TSVECTOR
    GENERATED ALWAYS AS (
      to_tsvector('spanish_unaccent'::regconfig, canonical_name || ' ' || coalesce(description, ''))
    ) STORED;

ALTER TABLE product_synonym
  ADD COLUMN search_document TSVECTOR
    GENERATED ALWAYS AS (to_tsvector('spanish_unaccent'::regconfig, term)) STORED;

CREATE INDEX idx_product_search_document ON product USING GIN (search_document);
CREATE INDEX idx_product_synonym_search_document ON product_synonym USING GIN (search_document);

-- +goose Down

DROP INDEX idx_product_synonym_search_document;
DROP INDEX idx_product_search_document;
ALTER TABLE product_synonym DROP COLUMN search_document;
ALTER TABLE product DROP COLUMN search_document, DROP COLUMN embedding_updated_at;
DROP TEXT SEARCH CONFIGURATION spanish_unaccent;
