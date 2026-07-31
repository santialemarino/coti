-- Coti — ajustes de esquema de las decisiones de modelado cerradas.
--
-- Ocho cambios en una sola migración. Todos son aditivos, relajantes o mueven una
-- columna, y ninguno necesita salir de la transacción: o entra el conjunto o no entra
-- nada. El código único de producto por cuenta, primer punto de la ronda, ya está en
-- 00002_add_product_code_unique.sql.

-- +goose Up

-- =============================================================================
-- quote.rfq_id NOT NULL
-- =============================================================================
--
-- uq_quote_rfq ya existía, pero con rfq_id nullable la unicidad se escapaba: Postgres
-- no considera dos NULL iguales, así que N cotizaciones sin RFQ convivían sin violar la
-- restricción. La regla 1-a-1 rfq→quote recién queda enforceada cuando el NULL es
-- imposible. Toda cotización nace de un RFQ, incluida la carga manual.
ALTER TABLE quote ALTER COLUMN rfq_id SET NOT NULL;

-- =============================================================================
-- Timestamps de proceso
-- =============================================================================
--
-- Convención: una tabla de proceso lleva un timestamp por cada transición que importa,
-- en vez de un updated_at genérico que no dice cuál fue el cambio. Nullable porque la
-- transición todavía no pasó; la escribe quien la ejecuta.
ALTER TABLE notification ADD COLUMN sent_at TIMESTAMPTZ;
ALTER TABLE rfq_attachment ADD COLUMN processed_at TIMESTAMPTZ;

-- =============================================================================
-- promotion.name
-- =============================================================================
--
-- description es el texto largo y opcional; name es la etiqueta corta con la que el
-- vendedor identifica la promo en pantalla. En tres pasos porque agregar una columna
-- NOT NULL sin default falla si la tabla tiene filas: las promos existentes toman su
-- descripción como nombre.
ALTER TABLE promotion ADD COLUMN name VARCHAR(128);
UPDATE promotion SET name = COALESCE(NULLIF(left(description, 128), ''), 'Promoción');
ALTER TABLE promotion ALTER COLUMN name SET NOT NULL;

-- =============================================================================
-- Canal de carga manual
-- =============================================================================
--
-- COUNTER ya era "carga presencial desde el backoffice", que es exactamente la carga
-- manual: se renombra en vez de agregar un valor nuevo. El rename es solo metadata —
-- las filas existentes siguen apuntando a la misma entrada de pg_enum, así que las
-- sucursales que ya tenían canal de mostrador quedan con su canal de carga manual sin
-- migrar datos. Agregar el valor, en cambio, no serviría acá: Postgres no permite usar
-- un valor de enum recién agregado en la misma transacción que lo agregó.
--
-- MANUAL_ENTRY describe cómo entró el pedido, no dónde estaba el cliente: un pedido
-- telefónico o un mensaje que llegó a un canal que no integramos también son carga
-- manual.
ALTER TYPE channel_type RENAME VALUE 'COUNTER' TO 'MANUAL_ENTRY';

-- =============================================================================
-- channel: identificador propio y unicidad relajada
-- =============================================================================
--
-- Una sucursal puede tener dos WhatsApp (dos números) o dos casillas de mail, así que
-- la unicidad pasa a incluir qué instancia del canal es. identifier es el número, la
-- casilla o lo que identifique la instancia; queda NULL para los canales que no tienen
-- más de una posible (webapp, carga manual).
ALTER TABLE channel ADD COLUMN identifier VARCHAR(255);

ALTER TABLE channel DROP CONSTRAINT uq_channel_branch_type;
ALTER TABLE channel ADD CONSTRAINT uq_channel_branch_type_identifier
  UNIQUE (branch_id, type, identifier);

-- La restricción compuesta sola no alcanza: con identifier NULL no compara, así que
-- dejaría entrar N canales de carga manual en la misma sucursal — la misma clase de
-- agujero que se cierra arriba con quote.rfq_id. El índice parcial cubre justamente el
-- caso sin identificador, igual que uq_product_account_code cubre el código nullable.
CREATE UNIQUE INDEX uq_channel_branch_type_no_identifier
  ON channel (branch_id, type) WHERE identifier IS NULL;

-- Toda sucursal necesita su canal de carga manual: sin él el vendedor no tiene por
-- dónde originar un pedido de mostrador, y un channel_id nullable en rfq reintroduciría
-- el agujero que cierra el NOT NULL de arriba. Las sucursales que venían de COUNTER ya
-- lo tienen por el rename.
INSERT INTO channel (account_id, branch_id, type)
SELECT b.account_id, b.id, 'MANUAL_ENTRY'
FROM branch b
WHERE NOT EXISTS (
  SELECT 1 FROM channel c WHERE c.branch_id = b.id AND c.type = 'MANUAL_ENTRY'
);

-- En la carga manual el cliente es opcional, y cuando no hay ficha el vendedor igual anota
-- para quién es el pedido. Esa etiqueta describe este pedido y no una persona con la que
-- después se vaya a matchear, así que vive en el pedido: crear una fila de client con solo
-- el nombre haría que "cliente opcional" fuera falso y llenaría el listado de clientes de
-- ventas de mostrador de una sola vez.
ALTER TABLE rfq ADD COLUMN client_label VARCHAR(255);

-- =============================================================================
-- Origen del cliente sale de channel_type
-- =============================================================================
--
-- channel_type y client.origin_channel contestaban preguntas distintas con el mismo
-- tipo: uno es el mecanismo de un canal que integramos, el otro es de dónde salió el
-- cliente. Separarlos es lo que deja sacar PHONE del primero sin perder "vino por
-- teléfono" en el segundo, y deja crecer la procedencia (referido, publicidad) sin
-- inventar canales que nadie integra.
CREATE TYPE client_origin AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'PHONE', 'WALK_IN');

ALTER TABLE client ALTER COLUMN origin_channel TYPE client_origin
  USING CASE origin_channel::text
          WHEN 'MANUAL_ENTRY' THEN 'WALK_IN'
          ELSE origin_channel::text
        END::client_origin;

-- Un pedido telefónico que el vendedor tipea entra por carga manual, así que PHONE
-- sobra como canal de ingesta. Postgres no puede quitar un valor de un enum: se
-- recrea el tipo con los cuatro que quedan.
UPDATE channel SET type = 'MANUAL_ENTRY' WHERE type = 'PHONE';

ALTER TYPE channel_type RENAME TO channel_type_old;
CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'MANUAL_ENTRY');
ALTER TABLE channel ALTER COLUMN type TYPE channel_type USING type::text::channel_type;
DROP TYPE channel_type_old;

-- =============================================================================
-- El combo pasa a nivel cuenta
-- =============================================================================
--
-- El catálogo ya es de la cuenta: producto, sinónimo y alternativa no llevan sucursal,
-- y la disponibilidad y el precio viven en branch_product y product_price. El combo era
-- la excepción, y sostenerla obligaba a explicar por qué justo él se duplica por
-- sucursal. El modelo es multi-sucursal parejo; que la UI del Piloto no lo exponga es
-- otra cosa.
CREATE TABLE branch_combo (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  branch_id  UUID NOT NULL,
  combo_id   UUID NOT NULL,
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_branch_combo UNIQUE (branch_id, combo_id)
);

ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_combo FOREIGN KEY (combo_id) REFERENCES combo(id);

CREATE INDEX idx_branch_combo_branch ON branch_combo(branch_id) WHERE is_active = TRUE;

CREATE TRIGGER trg_branch_combo_updated BEFORE UPDATE ON branch_combo FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Sin política de RLS la tabla nueva no falla: devuelve las filas de todas las cuentas.
-- El GRANT lo cubre ALTER DEFAULT PRIVILEGES de 00001 porque las migraciones corren con
-- el mismo rol owner, pero se escribe explícito para no depender de eso.
GRANT SELECT, INSERT, UPDATE, DELETE ON branch_combo TO coti_app;
ALTER TABLE branch_combo ENABLE ROW LEVEL SECURITY;
CREATE POLICY branch_combo_account_isolation ON branch_combo
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- La disponibilidad del combo se arma con la sucursal que tenía antes de dejar de
-- llevarla. Ni precio ni stock: el precio del combo sale de sus items, ya valorizados
-- por sucursal, y su stock sale de sus componentes.
INSERT INTO branch_combo (account_id, branch_id, combo_id)
SELECT account_id, branch_id, id FROM combo;

ALTER TABLE combo DROP COLUMN branch_id;

-- +goose Down

-- El combo vuelve a llevar sucursal. Es lossy por definición: un combo disponible en
-- dos sucursales vuelve con una sola, y uno sin disponibilidad no puede volver — el
-- SET NOT NULL falla antes de inventar una sucursal.
ALTER TABLE combo ADD COLUMN branch_id UUID;
UPDATE combo c SET branch_id = (
  SELECT bc.branch_id FROM branch_combo bc
  WHERE bc.combo_id = c.id
  ORDER BY bc.created_at, bc.branch_id
  LIMIT 1
);
ALTER TABLE combo ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE combo ADD CONSTRAINT fk_combo_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
CREATE INDEX idx_combo_branch ON combo(branch_id);

DROP TABLE branch_combo;

-- channel_type vuelve a los cinco valores originales y client.origin_channel vuelve a
-- compartirlo. WALK_IN no existe en el tipo viejo: vuelve a COUNTER, igual que
-- MANUAL_ENTRY. Los canales de carga manual creados por el backfill se quedan como
-- canales de mostrador; borrarlos rompería los rfq que ya los referencian.
ALTER TYPE channel_type RENAME TO channel_type_new;
CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'COUNTER', 'PHONE');
ALTER TABLE channel ALTER COLUMN type TYPE channel_type
  USING CASE type::text WHEN 'MANUAL_ENTRY' THEN 'COUNTER' ELSE type::text END::channel_type;
ALTER TABLE client ALTER COLUMN origin_channel TYPE channel_type
  USING CASE origin_channel::text WHEN 'WALK_IN' THEN 'COUNTER' ELSE origin_channel::text END::channel_type;
DROP TYPE channel_type_new;
DROP TYPE client_origin;

-- Volver a la unicidad por (sucursal, tipo) falla si alguien aprovechó la relajación
-- para cargar dos canales del mismo tipo en una sucursal. No hay forma de restaurarla
-- sin decidir cuál de los dos se tira, así que eso queda para quien haga el down.
DROP INDEX uq_channel_branch_type_no_identifier;
ALTER TABLE channel DROP CONSTRAINT uq_channel_branch_type_identifier;
ALTER TABLE channel DROP COLUMN identifier;
ALTER TABLE channel ADD CONSTRAINT uq_channel_branch_type UNIQUE (branch_id, type);

ALTER TABLE rfq DROP COLUMN client_label;

ALTER TABLE promotion DROP COLUMN name;

ALTER TABLE rfq_attachment DROP COLUMN processed_at;
ALTER TABLE notification DROP COLUMN sent_at;

ALTER TABLE quote ALTER COLUMN rfq_id DROP NOT NULL;
