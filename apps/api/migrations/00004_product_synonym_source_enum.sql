-- product_synonym.source becomes a native enum, and a term becomes unique per product.
--
-- The value set comes from who writes the column: MANUAL is loaded by a person, LEARNED is
-- proposed by the matching pipeline, IMPORTED arrives with a bulk load. Starting with all
-- three matters because ALTER TYPE ... ADD VALUE cannot be used in the transaction that
-- added it, and goose wraps each file in one.

-- +goose Up

CREATE TYPE product_synonym_source AS ENUM ('MANUAL', 'LEARNED', 'IMPORTED');

-- Anything that is not MANUAL or LEARNED arrived through a load, so IMPORTED is its place.
ALTER TABLE product_synonym
  ALTER COLUMN source TYPE product_synonym_source
  USING (
    CASE upper(source)
      WHEN 'MANUAL' THEN 'MANUAL'
      WHEN 'LEARNED' THEN 'LEARNED'
      ELSE 'IMPORTED'
    END
  )::product_synonym_source;

-- The common origin is a person loading terms in the backoffice.
ALTER TABLE product_synonym ALTER COLUMN source SET DEFAULT 'MANUAL';

-- =============================================================================
-- One term per product
-- =============================================================================
--
-- The rule already lived in the service; here it moves to the schema, case-insensitively
-- because "Portland" and "portland" are the same term to a matcher. The index is also the
-- ON CONFLICT target the synonym insert and the seed both name.
--
-- The dedupe is not optional: the index cannot be created on a database that carries
-- duplicates.
DELETE FROM product_synonym a
USING product_synonym b
WHERE a.account_id = b.account_id
  AND a.product_id = b.product_id
  AND lower(a.term) = lower(b.term)
  AND (a.created_at, a.id) > (b.created_at, b.id);

CREATE UNIQUE INDEX uq_product_synonym_term
  ON product_synonym (account_id, product_id, lower(term));

-- +goose Down

DROP INDEX uq_product_synonym_term;

ALTER TABLE product_synonym ALTER COLUMN source DROP DEFAULT;

ALTER TABLE product_synonym
  ALTER COLUMN source TYPE VARCHAR(64)
  USING source::text;

DROP TYPE product_synonym_source;
