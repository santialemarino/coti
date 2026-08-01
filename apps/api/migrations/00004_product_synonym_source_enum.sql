-- Coti — product_synonym.source pasa a enum nativo.
--
-- Era el único conjunto cerrado del schema guardado como VARCHAR: la base aceptaba
-- cualquier cosa y el conjunto válido vivía solo en Go, así que un segundo escritor —el
-- pipeline de aprendizaje, una carga masiva, alguien en psql— podía meter texto libre y
-- ningún lector podía confiar en la columna.
--
-- Se hace ahora porque es lo más barato que va a ser: ocho filas en desarrollo y ninguna
-- base productiva. Con datos del piloto son miles de términos aprendidos y el mapeo de
-- los valores viejos deja de ser trivial.
--
-- El conjunto sale de quién escribe la columna, no de los dos valores que nombraba el
-- ticket: MANUAL lo carga una persona, LEARNED lo propone el matching, IMPORTED entra por
-- carga masiva (US-01) y es donde caen las filas del seed. Agregar un valor después es
-- ALTER TYPE ... ADD VALUE, que no se puede usar en la misma transacción que lo agregó
-- —y goose envuelve cada archivo en una—, así que arrancar con los tres conocidos evita
-- tener que partir una migración en dos más adelante.

-- +goose Up

CREATE TYPE product_synonym_source AS ENUM ('MANUAL', 'LEARNED', 'IMPORTED');

-- El USING mapea lo que ya está cargado. Todo lo que no sea MANUAL o LEARNED entró por
-- una carga —hoy, las ocho filas 'seed'—, así que IMPORTED es su lugar, y el camino de
-- upgrade converge con lo que el seed nuevo escribe en una base limpia.
ALTER TABLE product_synonym
  ALTER COLUMN source TYPE product_synonym_source
  USING (
    CASE upper(source)
      WHEN 'MANUAL' THEN 'MANUAL'
      WHEN 'LEARNED' THEN 'LEARNED'
      ELSE 'IMPORTED'
    END
  )::product_synonym_source;

-- El origen más común es la carga a mano desde el backoffice, y con default la API no
-- tiene que mandarlo en cada insert.
ALTER TABLE product_synonym ALTER COLUMN source SET DEFAULT 'MANUAL';

-- =============================================================================
-- Un término por producto
-- =============================================================================
--
-- product_synonym era la única tabla sembrada cuyo ON CONFLICT DO NOTHING no tenía con
-- qué chocar: su único índice único es la PK, que es un uuid aleatorio, así que cada
-- `pnpm db:seed` volvía a insertar los ocho sinónimos. Nadie lo vio porque el seed
-- normalmente corre una vez.
--
-- La regla de negocio ya la aplicaba el service con un NOT EXISTS. Acá pasa al schema,
-- que es donde vive el resto: sin distinguir mayúsculas, porque "Portland" y "portland"
-- son el mismo término para un matcher.
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
