-- Coti — esquema consolidado de referencia.
--
-- Se LEE, no se aplica: la fuente ejecutable son las migraciones goose de
-- apps/api/migrations/, y `pnpm db:init` construye una base nueva corriéndolas.
-- Este archivo tiene que reflejar el resultado de esa cadena; cuando divergen se
-- regenera contra una base migrada.
--
-- Es lo que se lee para saber la forma actual del modelo: la lista de columnas de
-- un SELECT, el orden de scan y los campos del struct de dominio tienen que
-- coincidir con lo que está acá.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS vector;

-- =============================================================================
-- ENUMS
-- =============================================================================

CREATE TYPE rfq_status AS ENUM ('RECEIVED', 'GENERATED');

-- DRAFT: la cotización existe con materiales matcheados pero sin precios
-- aceptados. Es el estado mientras el RFQ está en GENERATED, y lo que permite
-- que la matriz estado×intención se evalúe con un solo input.
-- "Archivado" no es estado: es el flag ortogonal quote.archived_at.
CREATE TYPE quote_status AS ENUM (
  'DRAFT',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED'
);

CREATE TYPE user_role AS ENUM ('ADMIN', 'SELLER');

-- El sistema nunca descarta ítems sin match: los flaggea.
CREATE TYPE item_match_status AS ENUM ('MATCHED', 'AMBIGUOUS', 'NO_MATCH');

CREATE TYPE product_alternative_type AS ENUM ('EQUIVALENT', 'PREMIUM', 'ECONOMY');
CREATE TYPE quote_item_alternative_type AS ENUM ('PRODUCT', 'COMBO');
CREATE TYPE quote_item_alternative_origin AS ENUM ('AI', 'SELLER');

CREATE TYPE client_action_type AS ENUM ('ACCEPT', 'REJECT', 'REQUEST_CHANGE', 'COMMENT');

-- Motor de descuentos. ITEM_SET cubre varias líneas (la entidad de catálogo
-- combo es otra cosa). La vigencia es eje ortogonal, no un tipo.
CREATE TYPE promotion_condition_type AS ENUM ('PER_ITEM', 'QUANTITY_TIERED', 'ITEM_SET', 'ON_TOTAL');
CREATE TYPE promotion_action_type AS ENUM ('PERCENTAGE', 'FIXED_AMOUNT', 'SPECIAL_PRICE');
CREATE TYPE discount_scope AS ENUM ('ITEM', 'ITEM_SET', 'TOTAL');
CREATE TYPE discount_origin AS ENUM ('AUTOMATIC', 'AI_ADAPTATION', 'MANUAL_SELLER');

CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'COUNTER', 'PHONE');
CREATE TYPE attachment_type AS ENUM ('IMAGE', 'PDF', 'SPREADSHEET', 'AUDIO', 'TEXT');
CREATE TYPE attachment_processing_status AS ENUM ('PENDING', 'PROCESSING', 'DONE', 'FAILED');

CREATE TYPE send_format AS ENUM ('WEBAPP_LINK', 'PDF', 'MESSAGE');
CREATE TYPE send_tracking_status AS ENUM ('PENDING', 'SENT', 'DELIVERED', 'VIEWED', 'FAILED');

CREATE TYPE handler_seller_decision AS ENUM ('APPROVED_AS_IS', 'EDITED', 'REJECTED', 'MANUAL_OVERRIDE');
CREATE TYPE notification_status AS ENUM ('PENDING', 'SENT', 'FAILED');

-- Motor conversacional. El vendedor y el sistema son contexto, no trigger.
CREATE TYPE message_author_type AS ENUM ('CLIENT', 'SELLER', 'SYSTEM');
CREATE TYPE message_batch_status AS ENUM ('OPEN', 'CLOSED', 'PROCESSING', 'PROCESSED', 'FAILED');

-- =============================================================================
-- FUNCIONES
-- =============================================================================

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Resuelve la cuenta del request desde la GUC por transacción. current_setting
-- con el segundo argumento en true devuelve NULL si nunca se seteó, así que una
-- sesión sin contexto no matchea ninguna fila: falla cerrado.
CREATE OR REPLACE FUNCTION app_current_account_id() RETURNS UUID
  LANGUAGE sql STABLE
  AS $$ SELECT NULLIF(current_setting('app.current_account_id', true), '')::uuid $$;

-- =============================================================================
-- IDENTIDAD / TENANT
-- =============================================================================

CREATE TABLE account (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name            VARCHAR(255) NOT NULL,
  legal_name      VARCHAR(255),
  tax_id          VARCHAR(255),
  brand_logo_url  VARCHAR(512),
  brand_color     VARCHAR(32),
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE branch (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id          UUID NOT NULL,
  name                VARCHAR(255) NOT NULL,
  address             VARCHAR(255),
  default_expiry_days INTEGER NOT NULL DEFAULT 7,
  is_active           BOOLEAN NOT NULL DEFAULT TRUE,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- session_epoch invalida todos los access tokens vigentes al incrementarse
-- (logout inmediato sin lista negra). locked_until cierra el lockout por
-- intentos fallidos.
CREATE TABLE app_user (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      UUID NOT NULL,
  name            VARCHAR(255) NOT NULL,
  email           VARCHAR(255) NOT NULL,
  password_hash   VARCHAR(255) NOT NULL,
  role            user_role NOT NULL,
  is_active       BOOLEAN NOT NULL DEFAULT TRUE,
  session_epoch   INTEGER NOT NULL DEFAULT 1,
  last_login_at   TIMESTAMPTZ,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  locked_until    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_app_user_email UNIQUE (account_id, email)
);

CREATE TABLE user_branch (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  user_id    UUID NOT NULL,
  branch_id  UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_user_branch UNIQUE (user_id, branch_id)
);

-- Refresh tokens de un solo uso, rotativos por familia. Se guarda solo el
-- SHA-256: el valor crudo es de alta entropía, no hace falta un hash lento.
-- Re-presentar un token consumido más allá de la ventana de gracia revoca toda
-- la familia (robo).
CREATE TABLE refresh_token (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id  UUID NOT NULL,
  user_id     UUID NOT NULL,
  family_id   UUID NOT NULL,
  token_hash  CHAR(64) NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  revoked_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_refresh_token_hash UNIQUE (token_hash)
);

-- =============================================================================
-- CATÁLOGO
-- =============================================================================

-- El catálogo es de la cuenta: un producto es una fila por cuenta, con un solo
-- embedding y un solo juego de sinónimos y alternativas. Qué sucursal lo tiene y
-- con qué stock vive en branch_product; el precio en product_price.
CREATE TABLE product (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  code           VARCHAR(255),
  canonical_name VARCHAR(255) NOT NULL,
  description    VARCHAR(512),
  unit           VARCHAR(64),
  category       VARCHAR(255),
  embedding      VECTOR(1536),
  is_active      BOOLEAN NOT NULL DEFAULT TRUE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE branch_product (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  branch_id  UUID NOT NULL,
  product_id UUID NOT NULL,
  stock      NUMERIC(14,2),
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_branch_product UNIQUE (branch_id, product_id)
);

-- Términos coloquiales que mejoran el matching léxico (mitigación de R06).
CREATE TABLE product_synonym (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  product_id UUID NOT NULL,
  term       VARCHAR(255) NOT NULL,
  source     VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Historial por vigencia: no se updatea in-place.
CREATE TABLE product_price (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  branch_id  UUID NOT NULL,
  product_id UUID NOT NULL,
  user_id    UUID,
  price      NUMERIC(14,2) NOT NULL,
  currency   VARCHAR(8) NOT NULL DEFAULT 'ARS',
  conditions VARCHAR(255),
  min_price  NUMERIC(14,2),
  valid_from TIMESTAMPTZ NOT NULL,
  valid_to   TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE product_alternative (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id             UUID NOT NULL,
  base_product_id        UUID NOT NULL,
  alternative_product_id UUID NOT NULL,
  type                   product_alternative_type NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_product_alternative UNIQUE (base_product_id, alternative_product_id)
);

-- Producto compuesto que se vende como unidad. Distinto del tipo de promo ITEM_SET.
CREATE TABLE combo (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id  UUID NOT NULL,
  branch_id   UUID NOT NULL,
  name        VARCHAR(255) NOT NULL,
  description VARCHAR(512),
  is_active   BOOLEAN NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE combo_item (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  combo_id   UUID NOT NULL,
  product_id UUID NOT NULL,
  quantity   NUMERIC(14,2) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- CLIENTES
-- =============================================================================

-- Contacto nullable: mostrador sin datos está permitido, se enriquece
-- just-in-time. No se bloquea la creación por falta de contacto.
CREATE TABLE client (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  name           VARCHAR(255),
  phone          VARCHAR(64),
  email          VARCHAR(255),
  origin_channel channel_type,
  notes          VARCHAR(512),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tag (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  name       VARCHAR(128) NOT NULL,
  color      VARCHAR(32),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE client_tag (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  client_id  UUID NOT NULL,
  tag_id     UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_client_tag UNIQUE (client_id, tag_id)
);

-- =============================================================================
-- CANALES / INGESTA
-- =============================================================================

CREATE TABLE channel (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  branch_id  UUID NOT NULL,
  type       channel_type NOT NULL,
  config     JSONB,
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_channel_branch_type UNIQUE (branch_id, type)
);

-- Lo que pidió el cliente. Entidad separada de quote: el stepper de la UI es una
-- proyección sobre ambas, no una tercera entidad.
CREATE TABLE rfq (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id  UUID NOT NULL,
  branch_id   UUID NOT NULL,
  client_id   UUID,
  channel_id  UUID NOT NULL,
  raw_text    TEXT,
  status      rfq_status NOT NULL DEFAULT 'RECEIVED',
  work_type   VARCHAR(255),
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- El input original se persiste antes de procesarlo: una cotización siempre
-- tiene que poder reconstruirse desde su origen. Los archivos viven en Spaces.
CREATE TABLE rfq_attachment (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id        UUID NOT NULL,
  rfq_id            UUID NOT NULL,
  type              attachment_type NOT NULL,
  file_url          VARCHAR(512),
  extracted_text    TEXT,
  processing_status attachment_processing_status NOT NULL DEFAULT 'PENDING',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE rfq_status_change (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      UUID NOT NULL,
  rfq_id          UUID NOT NULL,
  previous_status rfq_status,
  new_status      rfq_status NOT NULL,
  user_id         UUID,
  changed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- COTIZACIONES
-- =============================================================================

-- Un RFQ tiene UNA sola cotización (uq_quote_rfq lo blinda). Reabrir desde
-- ACCEPTED/REJECTED reactiva la misma cotización, nunca duplica.
-- seller_id es nullable: un RFQ que entra por WhatsApp no tiene vendedor hasta
-- que alguien lo toma del inbox.
-- current_status no tiene default: se setea explícito en cada transición, junto
-- al insert en quote_status_change. Nunca lo edita un humano ni la IA.
CREATE TABLE quote (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id          UUID NOT NULL,
  branch_id           UUID NOT NULL,
  client_id           UUID,
  rfq_id              UUID,
  seller_id           UUID,
  current_version_id  UUID,
  current_status      quote_status NOT NULL,
  expires_at          TIMESTAMPTZ,
  archived_at         TIMESTAMPTZ,
  needs_followup      BOOLEAN NOT NULL DEFAULT FALSE,
  followup_flagged_at TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_rfq UNIQUE (rfq_id)
);

-- Snapshot de la cotización. Inmutable una vez congelada.
CREATE TABLE quote_version (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  quote_id       UUID NOT NULL,
  author_id      UUID,
  version_number INTEGER NOT NULL,
  total          NUMERIC(14,2) NOT NULL DEFAULT 0,
  is_immutable   BOOLEAN NOT NULL DEFAULT FALSE,
  comment        VARCHAR(512),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_version UNIQUE (quote_id, version_number)
);

-- El ítem NO contiene el descuento: el descuento es entidad propia.
-- min_price_snapshot es el piso que usa el motor de descuentos. Se snapshotea
-- junto al precio para que re-barrer la misma versión dé siempre el mismo total.
-- Las filas de una versión no congelada se editan in-place; el service rechaza
-- cualquier mutación cuya versión padre tenga is_immutable = true.
CREATE TABLE quote_item (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id            UUID NOT NULL,
  version_id            UUID NOT NULL,
  product_id            UUID,
  requested_description VARCHAR(512) NOT NULL,
  quantity              NUMERIC(14,2) NOT NULL,
  unit                  VARCHAR(64),
  unit_price_snapshot   NUMERIC(14,2),
  min_price_snapshot    NUMERIC(14,2),
  subtotal              NUMERIC(14,2),
  confidence_score      NUMERIC(5,4),
  match_status          item_match_status NOT NULL DEFAULT 'MATCHED',
  quantity_rationale    VARCHAR(512),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- El cliente nunca elige sobre alternativas que el vendedor no aprobó.
CREATE TABLE quote_item_alternative (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id         UUID NOT NULL,
  quote_item_id      UUID NOT NULL,
  product_id         UUID,
  combo_id           UUID,
  type               quote_item_alternative_type NOT NULL,
  origin             quote_item_alternative_origin NOT NULL,
  price_snapshot     NUMERIC(14,2),
  approved_by_seller BOOLEAN NOT NULL DEFAULT FALSE,
  chosen_by_client   BOOLEAN NOT NULL DEFAULT FALSE,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ck_qia_target CHECK (product_id IS NOT NULL OR combo_id IS NOT NULL)
);

CREATE TABLE quote_status_change (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      UUID NOT NULL,
  quote_id        UUID NOT NULL,
  previous_status quote_status,
  new_status      quote_status NOT NULL,
  user_id         UUID,
  changed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- ENVÍO / INTERACCIÓN DEL CLIENTE
-- =============================================================================

-- El link se emite por envío/canal, no vive en quote. public_token es la única
-- clave de lookup de la webapp, así que es único.
CREATE TABLE quote_send (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id      UUID NOT NULL,
  version_id      UUID NOT NULL,
  channel_id      UUID NOT NULL,
  public_token    VARCHAR(255),
  format          send_format NOT NULL,
  sent_at         TIMESTAMPTZ,
  expires_at      TIMESTAMPTZ,
  tracking_status send_tracking_status NOT NULL DEFAULT 'PENDING',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_send_public_token UNIQUE (public_token)
);

-- Rechazar es acción explícita del cliente o del vendedor, nunca la infiere la IA.
CREATE TABLE client_action (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id    UUID NOT NULL,
  version_id    UUID NOT NULL,
  quote_send_id UUID,
  quote_item_id UUID,
  type          client_action_type NOT NULL,
  comment       VARCHAR(512),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- MOTOR CONVERSACIONAL
-- =============================================================================

-- La ventana y la cola. closes_at se recalcula en cada mensaje nuevo como
-- min(now + reset, opened_at + tope); un job cierra los vencidos. Los índices
-- únicos parciales garantizan una ventana abierta y un batch procesando por
-- cotización: los CLOSED esperan en FIFO por closed_at.
CREATE TABLE message_batch (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id   UUID NOT NULL,
  quote_id     UUID NOT NULL,
  status       message_batch_status NOT NULL DEFAULT 'OPEN',
  opened_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  closes_at    TIMESTAMPTZ NOT NULL,
  closed_at    TIMESTAMPTZ,
  processed_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Cada mensaje de la conversación de una cotización. author_type distingue
-- trigger (CLIENT) de contexto (SELLER, SYSTEM). client_action_id liga el
-- mensaje a la acción de la webapp que lo originó, para que un REQUEST_CHANGE
-- con comentario no quede representado dos veces.
-- batch_id se asigna al cerrar la ventana; el resto de la fila no se toca.
CREATE TABLE quote_message (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id       UUID NOT NULL,
  quote_id         UUID NOT NULL,
  channel_id       UUID,
  batch_id         UUID,
  client_action_id UUID,
  author_type      message_author_type NOT NULL,
  author_user_id   UUID,
  body             TEXT NOT NULL,
  received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- MOTOR DE DESCUENTOS
-- =============================================================================

-- La regla reusable. Cuelga de account obligatorio + branch nullable
-- (null = toda la cuenta). Distinta de su aplicación (quote_discount).
CREATE TABLE promotion (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  branch_id      UUID,
  condition_type promotion_condition_type NOT NULL,
  action_type    promotion_action_type NOT NULL,
  action_value   NUMERIC(14,2) NOT NULL,
  valid_from     TIMESTAMPTZ,
  valid_to       TIMESTAMPTZ,
  is_active      BOOLEAN NOT NULL DEFAULT TRUE,
  is_exclusive   BOOLEAN NOT NULL DEFAULT FALSE,
  priority       INTEGER NOT NULL DEFAULT 0,
  description    VARCHAR(512),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE promotion_condition_item (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id   UUID NOT NULL,
  promotion_id UUID NOT NULL,
  product_id   UUID,
  category     VARCHAR(255),
  min_quantity NUMERIC(14,2),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ck_pci_target CHECK (product_id IS NOT NULL OR category IS NOT NULL)
);

CREATE TABLE promotion_tier (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id    UUID NOT NULL,
  promotion_id  UUID NOT NULL,
  from_quantity NUMERIC(14,2) NOT NULL,
  to_quantity   NUMERIC(14,2),
  value         NUMERIC(14,2) NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- La aplicación de un descuento a una versión. El monto lo calcula el motor
-- determinístico, NUNCA la IA. suppressed_by_seller impide que el barrido lo
-- re-aplique; suprimir un AUTOMATIC es reversible, borrar un MANUAL_SELLER no.
CREATE TABLE quote_discount (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id           UUID NOT NULL,
  quote_version_id     UUID NOT NULL,
  promotion_id         UUID,
  condition_type       promotion_condition_type NOT NULL,
  scope                discount_scope NOT NULL,
  origin               discount_origin NOT NULL,
  amount               NUMERIC(14,2) NOT NULL,
  description          VARCHAR(512),
  suppressed_by_seller BOOLEAN NOT NULL DEFAULT FALSE,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE quote_discount_item (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id        UUID NOT NULL,
  quote_discount_id UUID NOT NULL,
  quote_item_id     UUID NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_quote_discount_item UNIQUE (quote_discount_id, quote_item_id)
);

-- =============================================================================
-- REGISTRO DEL HANDLER (NIVEL 1) + NOTIFICACIONES
-- =============================================================================

-- Registro de dos fases: la fila se inserta cuando el handler propone (user_id y
-- seller_decision nullable) y se completa cuando el vendedor decide. Si se
-- insertara solo al decidir, una propuesta que el vendedor nunca toca no
-- quedaría registrada, y ese es justo el caso que la métrica del Piloto necesita
-- contar.
CREATE TABLE handler_decision (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id        UUID NOT NULL,
  quote_version_id  UUID NOT NULL,
  message_batch_id  UUID,
  user_id           UUID,
  client_input      TEXT,
  ai_interpretation VARCHAR(512),
  ai_proposal       TEXT,
  seller_decision   handler_seller_decision,
  state_at_decision quote_status,
  decided_at        TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  user_id    UUID,
  client_id  UUID,
  quote_id   UUID,
  event      VARCHAR(128) NOT NULL,
  medium     VARCHAR(64) NOT NULL,
  status     notification_status NOT NULL DEFAULT 'PENDING',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =============================================================================
-- FOREIGN KEYS
-- (al final, para resolver la dependencia circular quote <-> quote_version)
-- =============================================================================

ALTER TABLE branch ADD CONSTRAINT fk_branch_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE app_user ADD CONSTRAINT fk_app_user_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE user_branch ADD CONSTRAINT fk_user_branch_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE user_branch ADD CONSTRAINT fk_user_branch_user FOREIGN KEY (user_id) REFERENCES app_user(id);
ALTER TABLE user_branch ADD CONSTRAINT fk_user_branch_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE refresh_token ADD CONSTRAINT fk_refresh_token_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE refresh_token ADD CONSTRAINT fk_refresh_token_user FOREIGN KEY (user_id) REFERENCES app_user(id);

ALTER TABLE product ADD CONSTRAINT fk_product_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE branch_product ADD CONSTRAINT fk_branch_product_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE branch_product ADD CONSTRAINT fk_branch_product_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE branch_product ADD CONSTRAINT fk_branch_product_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE product_synonym ADD CONSTRAINT fk_product_synonym_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE product_synonym ADD CONSTRAINT fk_product_synonym_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE product_price ADD CONSTRAINT fk_product_price_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE product_price ADD CONSTRAINT fk_product_price_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE product_price ADD CONSTRAINT fk_product_price_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE product_price ADD CONSTRAINT fk_product_price_user FOREIGN KEY (user_id) REFERENCES app_user(id);
ALTER TABLE product_alternative ADD CONSTRAINT fk_product_alt_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE product_alternative ADD CONSTRAINT fk_product_alt_base FOREIGN KEY (base_product_id) REFERENCES product(id);
ALTER TABLE product_alternative ADD CONSTRAINT fk_product_alt_alt FOREIGN KEY (alternative_product_id) REFERENCES product(id);
ALTER TABLE combo ADD CONSTRAINT fk_combo_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE combo ADD CONSTRAINT fk_combo_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_combo FOREIGN KEY (combo_id) REFERENCES combo(id);
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_product FOREIGN KEY (product_id) REFERENCES product(id);

ALTER TABLE client ADD CONSTRAINT fk_client_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE tag ADD CONSTRAINT fk_tag_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_client FOREIGN KEY (client_id) REFERENCES client(id);
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_tag FOREIGN KEY (tag_id) REFERENCES tag(id);

ALTER TABLE channel ADD CONSTRAINT fk_channel_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE channel ADD CONSTRAINT fk_channel_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_client FOREIGN KEY (client_id) REFERENCES client(id);
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_channel FOREIGN KEY (channel_id) REFERENCES channel(id);
ALTER TABLE rfq_attachment ADD CONSTRAINT fk_rfq_attachment_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE rfq_attachment ADD CONSTRAINT fk_rfq_attachment_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id);
ALTER TABLE rfq_status_change ADD CONSTRAINT fk_rfq_status_change_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE rfq_status_change ADD CONSTRAINT fk_rfq_status_change_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id);
ALTER TABLE rfq_status_change ADD CONSTRAINT fk_rfq_status_change_user FOREIGN KEY (user_id) REFERENCES app_user(id);

ALTER TABLE quote ADD CONSTRAINT fk_quote_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote ADD CONSTRAINT fk_quote_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE quote ADD CONSTRAINT fk_quote_client FOREIGN KEY (client_id) REFERENCES client(id);
ALTER TABLE quote ADD CONSTRAINT fk_quote_rfq FOREIGN KEY (rfq_id) REFERENCES rfq(id);
ALTER TABLE quote ADD CONSTRAINT fk_quote_seller FOREIGN KEY (seller_id) REFERENCES app_user(id);
ALTER TABLE quote ADD CONSTRAINT fk_quote_current_version FOREIGN KEY (current_version_id) REFERENCES quote_version(id);
ALTER TABLE quote_version ADD CONSTRAINT fk_quote_version_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_version ADD CONSTRAINT fk_quote_version_quote FOREIGN KEY (quote_id) REFERENCES quote(id);
ALTER TABLE quote_version ADD CONSTRAINT fk_quote_version_author FOREIGN KEY (author_id) REFERENCES app_user(id);
ALTER TABLE quote_item ADD CONSTRAINT fk_quote_item_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_item ADD CONSTRAINT fk_quote_item_version FOREIGN KEY (version_id) REFERENCES quote_version(id);
ALTER TABLE quote_item ADD CONSTRAINT fk_quote_item_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE quote_item_alternative ADD CONSTRAINT fk_qia_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_item_alternative ADD CONSTRAINT fk_qia_item FOREIGN KEY (quote_item_id) REFERENCES quote_item(id);
ALTER TABLE quote_item_alternative ADD CONSTRAINT fk_qia_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE quote_item_alternative ADD CONSTRAINT fk_qia_combo FOREIGN KEY (combo_id) REFERENCES combo(id);
ALTER TABLE quote_status_change ADD CONSTRAINT fk_quote_status_change_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_status_change ADD CONSTRAINT fk_quote_status_change_quote FOREIGN KEY (quote_id) REFERENCES quote(id);
ALTER TABLE quote_status_change ADD CONSTRAINT fk_quote_status_change_user FOREIGN KEY (user_id) REFERENCES app_user(id);

ALTER TABLE quote_send ADD CONSTRAINT fk_quote_send_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_send ADD CONSTRAINT fk_quote_send_version FOREIGN KEY (version_id) REFERENCES quote_version(id);
ALTER TABLE quote_send ADD CONSTRAINT fk_quote_send_channel FOREIGN KEY (channel_id) REFERENCES channel(id);
ALTER TABLE client_action ADD CONSTRAINT fk_client_action_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE client_action ADD CONSTRAINT fk_client_action_version FOREIGN KEY (version_id) REFERENCES quote_version(id);
ALTER TABLE client_action ADD CONSTRAINT fk_client_action_send FOREIGN KEY (quote_send_id) REFERENCES quote_send(id);
ALTER TABLE client_action ADD CONSTRAINT fk_client_action_item FOREIGN KEY (quote_item_id) REFERENCES quote_item(id);

ALTER TABLE message_batch ADD CONSTRAINT fk_message_batch_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE message_batch ADD CONSTRAINT fk_message_batch_quote FOREIGN KEY (quote_id) REFERENCES quote(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_quote FOREIGN KEY (quote_id) REFERENCES quote(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_channel FOREIGN KEY (channel_id) REFERENCES channel(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_batch FOREIGN KEY (batch_id) REFERENCES message_batch(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_action FOREIGN KEY (client_action_id) REFERENCES client_action(id);
ALTER TABLE quote_message ADD CONSTRAINT fk_quote_message_user FOREIGN KEY (author_user_id) REFERENCES app_user(id);

ALTER TABLE promotion ADD CONSTRAINT fk_promotion_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE promotion ADD CONSTRAINT fk_promotion_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE promotion_condition_item ADD CONSTRAINT fk_pci_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE promotion_condition_item ADD CONSTRAINT fk_pci_promotion FOREIGN KEY (promotion_id) REFERENCES promotion(id);
ALTER TABLE promotion_condition_item ADD CONSTRAINT fk_pci_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE promotion_tier ADD CONSTRAINT fk_promotion_tier_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE promotion_tier ADD CONSTRAINT fk_promotion_tier_promotion FOREIGN KEY (promotion_id) REFERENCES promotion(id);
ALTER TABLE quote_discount ADD CONSTRAINT fk_quote_discount_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_discount ADD CONSTRAINT fk_quote_discount_version FOREIGN KEY (quote_version_id) REFERENCES quote_version(id);
ALTER TABLE quote_discount ADD CONSTRAINT fk_quote_discount_promotion FOREIGN KEY (promotion_id) REFERENCES promotion(id);
ALTER TABLE quote_discount_item ADD CONSTRAINT fk_qdi_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE quote_discount_item ADD CONSTRAINT fk_qdi_discount FOREIGN KEY (quote_discount_id) REFERENCES quote_discount(id);
ALTER TABLE quote_discount_item ADD CONSTRAINT fk_qdi_item FOREIGN KEY (quote_item_id) REFERENCES quote_item(id);

ALTER TABLE handler_decision ADD CONSTRAINT fk_handler_decision_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE handler_decision ADD CONSTRAINT fk_handler_decision_version FOREIGN KEY (quote_version_id) REFERENCES quote_version(id);
ALTER TABLE handler_decision ADD CONSTRAINT fk_handler_decision_batch FOREIGN KEY (message_batch_id) REFERENCES message_batch(id);
ALTER TABLE handler_decision ADD CONSTRAINT fk_handler_decision_user FOREIGN KEY (user_id) REFERENCES app_user(id);
ALTER TABLE notification ADD CONSTRAINT fk_notification_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE notification ADD CONSTRAINT fk_notification_user FOREIGN KEY (user_id) REFERENCES app_user(id);
ALTER TABLE notification ADD CONSTRAINT fk_notification_client FOREIGN KEY (client_id) REFERENCES client(id);
ALTER TABLE notification ADD CONSTRAINT fk_notification_quote FOREIGN KEY (quote_id) REFERENCES quote(id);

-- =============================================================================
-- ÍNDICES
-- =============================================================================

-- Índice vectorial: se crea DESPUÉS de cargar datos (con la tabla vacía queda
-- subóptimo). Va comentado a propósito.
-- CREATE INDEX idx_product_embedding ON product USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE INDEX idx_branch_account ON branch(account_id);
CREATE INDEX idx_app_user_account ON app_user(account_id);
CREATE INDEX idx_refresh_token_user ON refresh_token(user_id);
CREATE INDEX idx_refresh_token_family ON refresh_token(family_id);

CREATE INDEX idx_product_account ON product(account_id);
CREATE INDEX idx_branch_product_branch ON branch_product(branch_id) WHERE is_active = TRUE;
CREATE INDEX idx_product_synonym_product ON product_synonym(product_id);
CREATE INDEX idx_product_price_product ON product_price(product_id, branch_id);
CREATE INDEX idx_combo_branch ON combo(branch_id);

CREATE INDEX idx_client_account ON client(account_id);
CREATE INDEX idx_rfq_branch_status ON rfq(branch_id, status);
CREATE INDEX idx_rfq_attachment_pending ON rfq_attachment(processing_status) WHERE processing_status IN ('PENDING', 'PROCESSING');

CREATE INDEX idx_quote_branch_status ON quote(branch_id, current_status);
CREATE INDEX idx_quote_expires ON quote(expires_at) WHERE expires_at IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_quote_needs_followup ON quote(needs_followup) WHERE needs_followup = TRUE;
CREATE INDEX idx_quote_version_quote ON quote_version(quote_id);
CREATE INDEX idx_quote_item_version ON quote_item(version_id);
CREATE INDEX idx_quote_discount_version ON quote_discount(quote_version_id);
CREATE INDEX idx_promotion_account ON promotion(account_id) WHERE is_active = TRUE;
CREATE INDEX idx_quote_message_quote ON quote_message(quote_id, received_at);
CREATE INDEX idx_message_batch_due ON message_batch(closes_at) WHERE status = 'OPEN';
CREATE INDEX idx_message_batch_queue ON message_batch(quote_id, closed_at) WHERE status = 'CLOSED';

-- Invariantes de negocio expresadas en la base, no en la buena voluntad del código.
CREATE UNIQUE INDEX uq_quote_version_draft ON quote_version(quote_id) WHERE is_immutable = FALSE;
CREATE UNIQUE INDEX uq_message_batch_open ON message_batch(quote_id) WHERE status = 'OPEN';
CREATE UNIQUE INDEX uq_message_batch_processing ON message_batch(quote_id) WHERE status = 'PROCESSING';

-- =============================================================================
-- TRIGGERS updated_at (solo tablas que mutan in-place)
-- =============================================================================

CREATE TRIGGER trg_account_updated        BEFORE UPDATE ON account        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_branch_updated         BEFORE UPDATE ON branch         FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_app_user_updated       BEFORE UPDATE ON app_user       FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_product_updated        BEFORE UPDATE ON product        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_branch_product_updated BEFORE UPDATE ON branch_product FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_combo_updated          BEFORE UPDATE ON combo          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_client_updated         BEFORE UPDATE ON client         FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_channel_updated        BEFORE UPDATE ON channel        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_rfq_updated            BEFORE UPDATE ON rfq            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_quote_updated          BEFORE UPDATE ON quote          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_promotion_updated      BEFORE UPDATE ON promotion      FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- ROL RESTRINGIDO + RLS
-- =============================================================================
--
-- Dos roles sostienen el aislamiento:
--   * el owner (este script, las migraciones, el cron de follow-up y los lookups
--     pre-auth) bypassea RLS porque es dueño de las tablas — es lo que necesitan
--     los jobs y las lecturas que legítimamente cruzan cuentas: login por email
--     (todavía no se sabe la cuenta) y resolución de quote_send.public_token
--     desde la webapp sin sesión;
--   * coti_app tiene DML pero NO es owner y es NOBYPASSRLS, así que toda conexión
--     de request queda sujeta a las políticas.
--
-- Cada transacción setea app.current_account_id con SET LOCAL, re-aplicado en
-- cada BEGIN porque el pool reutiliza conexiones. Sin GUC, la política no
-- matchea ninguna fila.
--
-- ENABLE y no FORCE es deliberado: FORCE también sujetaría al owner.
--
-- Solo se enforcea la cuenta. El scoping por sucursal queda en la aplicación: un
-- admin lee legítimamente todas las sucursales de su cuenta.

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'coti_app') THEN
    CREATE ROLE coti_app LOGIN PASSWORD 'coti_app' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
  END IF;
END $$;

GRANT USAGE ON SCHEMA public TO coti_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO coti_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO coti_app;
GRANT EXECUTE ON FUNCTION app_current_account_id() TO coti_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO coti_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO coti_app;

-- account matchea por su propio id; el resto por su columna account_id.
ALTER TABLE account ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_isolation ON account
  USING (id = app_current_account_id())
  WITH CHECK (id = app_current_account_id());

DO $$
DECLARE t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'branch', 'app_user', 'user_branch', 'refresh_token',
    'product', 'branch_product', 'product_synonym', 'product_price', 'product_alternative',
    'combo', 'combo_item',
    'client', 'tag', 'client_tag',
    'channel', 'rfq', 'rfq_attachment', 'rfq_status_change',
    'quote', 'quote_version', 'quote_item', 'quote_item_alternative', 'quote_status_change',
    'quote_send', 'client_action',
    'message_batch', 'quote_message',
    'promotion', 'promotion_condition_item', 'promotion_tier',
    'quote_discount', 'quote_discount_item',
    'handler_decision', 'notification'
  ] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY %I ON %I USING (account_id = app_current_account_id()) WITH CHECK (account_id = app_current_account_id())',
      t || '_account_isolation', t);
  END LOOP;
END $$;
