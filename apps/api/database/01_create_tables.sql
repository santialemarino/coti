-- Coti — consolidated reference schema.
--
-- This file is READ, never applied: the executable source is the goose chain in
-- apps/api/migrations/, and `pnpm db:init` builds a fresh database by running it. This file
-- has to reflect the result of that chain.
--
-- It is what you consult to know the current shape of the model: a SELECT column list, a
-- scan order and a domain struct's fields all have to agree with what is here.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS vector;

-- =============================================================================
-- ENUMS
-- =============================================================================

CREATE TYPE rfq_status AS ENUM ('RECEIVED', 'GENERATED');

-- DRAFT: the quote exists with matched materials but no accepted prices. It is the state
-- while the RFQ is GENERATED, and what lets the state x intention matrix evaluate on one
-- input. "Archived" is not a state: it is the orthogonal quote.archived_at flag.
CREATE TYPE quote_status AS ENUM (
  'DRAFT',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED'
);

CREATE TYPE user_role AS ENUM ('ADMIN', 'SELLER');

-- Unmatched items are flagged, never discarded.
CREATE TYPE item_match_status AS ENUM ('MATCHED', 'AMBIGUOUS', 'NO_MATCH');

CREATE TYPE product_alternative_type AS ENUM ('EQUIVALENT', 'PREMIUM', 'ECONOMY');

-- Where a synonym came from: a person loaded it, the matching pipeline proposed it, or it
-- arrived with a bulk load (which is where the seed rows land).
CREATE TYPE product_synonym_source AS ENUM ('MANUAL', 'LEARNED', 'IMPORTED');

CREATE TYPE quote_item_alternative_type AS ENUM ('PRODUCT', 'COMBO');
CREATE TYPE quote_item_alternative_origin AS ENUM ('AI', 'SELLER');

CREATE TYPE client_action_type AS ENUM ('ACCEPT', 'REJECT', 'REQUEST_CHANGE', 'COMMENT');

-- Discount engine. ITEM_SET covers several lines; the catalog combo is a different entity.
-- Validity is an orthogonal axis, not a type.
CREATE TYPE promotion_condition_type AS ENUM ('PER_ITEM', 'QUANTITY_TIERED', 'ITEM_SET', 'ON_TOTAL');
CREATE TYPE promotion_action_type AS ENUM ('PERCENTAGE', 'FIXED_AMOUNT', 'SPECIAL_PRICE');
CREATE TYPE discount_scope AS ENUM ('ITEM', 'ITEM_SET', 'TOTAL');
CREATE TYPE discount_origin AS ENUM ('AUTOMATIC', 'AI_ADAPTATION', 'MANUAL_SELLER');

-- channel_type is the mechanism a request arrived by, not where the client stood:
-- MANUAL_ENTRY covers the counter, the phone and any channel we do not integrate, because
-- in all of them the seller types the order in. client_origin answers a different question —
-- where the client came from — and is its own type for that reason.
CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'MANUAL_ENTRY');
CREATE TYPE client_origin AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'PHONE', 'WALK_IN');
CREATE TYPE attachment_type AS ENUM ('IMAGE', 'PDF', 'SPREADSHEET', 'AUDIO', 'TEXT');
CREATE TYPE attachment_processing_status AS ENUM ('PENDING', 'PROCESSING', 'DONE', 'FAILED');

CREATE TYPE send_format AS ENUM ('WEBAPP_LINK', 'PDF', 'MESSAGE');
CREATE TYPE send_tracking_status AS ENUM ('PENDING', 'SENT', 'DELIVERED', 'VIEWED', 'FAILED');

CREATE TYPE handler_seller_decision AS ENUM ('APPROVED_AS_IS', 'EDITED', 'REJECTED', 'MANUAL_OVERRIDE');
CREATE TYPE notification_status AS ENUM ('PENDING', 'SENT', 'FAILED');

-- Conversational engine. The seller and the system are context, not a trigger.
CREATE TYPE message_author_type AS ENUM ('CLIENT', 'SELLER', 'SYSTEM');
CREATE TYPE message_batch_status AS ENUM ('OPEN', 'CLOSED', 'PROCESSING', 'PROCESSED', 'FAILED');

-- =============================================================================
-- FUNCTIONS
-- =============================================================================

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Resolves the request's account from the per-transaction GUC. current_setting with its
-- second argument true returns NULL when unset, so a session with no context matches no
-- row: it fails closed.
CREATE OR REPLACE FUNCTION app_current_account_id() RETURNS UUID
  LANGUAGE sql STABLE
  AS $$ SELECT NULLIF(current_setting('app.current_account_id', true), '')::uuid $$;

-- =============================================================================
-- IDENTITY / TENANT
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

-- Bumping session_epoch invalidates every outstanding access token, which is immediate
-- logout without a blacklist. locked_until closes out the failed-attempt counter.
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

-- Single-use refresh tokens, rotated per family. Only the SHA-256 is stored: the raw value
-- is high-entropy, so a slow hash is unnecessary. Replaying a consumed token past the grace
-- window revokes the whole family as theft.
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
-- CATALOG
-- =============================================================================

-- The catalog belongs to the account: one product row, one embedding, one set of synonyms
-- and alternatives per account. Which branch carries it and with how much stock lives in
-- branch_product; the price in product_price.
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

-- Colloquial trade terms that improve lexical catalog matching.
CREATE TABLE product_synonym (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  product_id UUID NOT NULL,
  term       VARCHAR(255) NOT NULL,
  source     product_synonym_source NOT NULL DEFAULT 'MANUAL',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Validity-versioned history: never updated in place.
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

-- A composite product sold as one unit. Different from the ITEM_SET promotion type.
-- Account-scoped like the product: per-branch availability lives in branch_combo. It has no
-- price of its own — that derives from its items, already priced per branch — and no stock,
-- which derives from its components.
CREATE TABLE combo (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id  UUID NOT NULL,
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

-- The combo's commercial availability per branch, mirroring branch_product. No price and no
-- stock: both derive from the items.
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

-- =============================================================================
-- CLIENTS
-- =============================================================================

-- Contact details are nullable: a counter sale with none is allowed and enriched later.
-- Missing contact never blocks creation.
CREATE TABLE client (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id     UUID NOT NULL,
  name           VARCHAR(255),
  phone          VARCHAR(64),
  email          VARCHAR(255),
  origin_channel client_origin,
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
-- CHANNELS / INTAKE
-- =============================================================================

-- identifier is which instance of the channel this is: the WhatsApp number, the mailbox. It
-- stays NULL on channels a branch can only have one of, and for those the partial index is
-- what holds uniqueness up, since the composite constraint does not compare NULLs.
CREATE TABLE channel (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id UUID NOT NULL,
  branch_id  UUID NOT NULL,
  type       channel_type NOT NULL,
  config     JSONB,
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  identifier VARCHAR(255),
  CONSTRAINT uq_channel_branch_type_identifier UNIQUE (branch_id, type, identifier)
);

-- What the client asked for. Separate from quote: the UI stepper is a projection over both,
-- not a third entity.
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
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Who the order is for when there is no client record: on manual entry the seller notes a
  -- loose name and client_id stays NULL. It describes this order, not a person to match.
  client_label VARCHAR(255)
);

-- The original input is persisted before it is processed: a quote must always be
-- reconstructible from its source. The files live in object storage.
CREATE TABLE rfq_attachment (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id        UUID NOT NULL,
  rfq_id            UUID NOT NULL,
  type              attachment_type NOT NULL,
  file_url          VARCHAR(512),
  extracted_text    TEXT,
  processing_status attachment_processing_status NOT NULL DEFAULT 'PENDING',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at      TIMESTAMPTZ
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
-- QUOTES
-- =============================================================================

-- One RFQ has exactly one quote (uq_quote_rfq enforces it). Reopening from
-- ACCEPTED/REJECTED reactivates the same quote, never a duplicate. seller_id is nullable
-- until someone claims the RFQ from the inbox. current_status has no default: it is set
-- explicitly on each transition alongside the quote_status_change insert, never by a human
-- or the AI.
CREATE TABLE quote (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id          UUID NOT NULL,
  branch_id           UUID NOT NULL,
  client_id           UUID,
  rfq_id              UUID NOT NULL,
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

-- A snapshot of the quote. Immutable once frozen.
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

-- The item does NOT carry its discount: a discount is its own entity. min_price_snapshot is
-- the discount engine's floor, snapshotted alongside the price so re-sweeping a version is
-- deterministic. Rows on a non-frozen version are edited in place; the service rejects any
-- mutation whose parent version has is_immutable = true.
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

-- The client never chooses among alternatives the seller did not approve.
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
-- SENDING / CLIENT INTERACTION
-- =============================================================================

-- The link is issued per send and channel, not stored on quote. public_token is the
-- webapp's only lookup key, so it is unique.
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

-- Rejection is an explicit client or seller action, never inferred by the AI.
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
-- CONVERSATIONAL ENGINE
-- =============================================================================

-- The window and the queue. closes_at is recomputed on each new message as
-- min(now + reset, opened_at + cap); a job closes the expired ones. The partial unique
-- indexes guarantee one open window and one processing batch per quote; CLOSED ones wait
-- FIFO by closed_at.
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

-- One message in a quote's conversation. author_type separates the trigger (CLIENT) from
-- context (SELLER, SYSTEM). client_action_id ties the message to the webapp action that
-- produced it, so a REQUEST_CHANGE with a comment is not represented twice. batch_id is
-- assigned when the window closes; nothing else on the row is touched.
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
-- DISCOUNT ENGINE
-- =============================================================================

-- The reusable rule. Hangs off a mandatory account and a nullable branch (null = the whole
-- account). Distinct from its application, which is quote_discount.
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
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  name           VARCHAR(128) NOT NULL
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

-- One application of a discount to a version. The amount is computed by the deterministic
-- engine, NEVER by the AI. suppressed_by_seller stops the sweep re-applying it: suppressing
-- an AUTOMATIC is reversible, deleting a MANUAL_SELLER is not.
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
-- HANDLER LOG (LEVEL 1) + NOTIFICATIONS
-- =============================================================================

-- A two-phase record: the row is inserted when the handler proposes (user_id and
-- seller_decision nullable) and completed when the seller decides. Inserting only on the
-- decision would lose the proposals nobody ever touches, which is exactly what the pilot
-- metric needs to count.
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
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at    TIMESTAMPTZ
);

-- =============================================================================
-- FOREIGN KEYS
-- (at the end, to resolve the circular quote <-> quote_version dependency)
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
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_combo FOREIGN KEY (combo_id) REFERENCES combo(id);
ALTER TABLE combo_item ADD CONSTRAINT fk_combo_item_product FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_account FOREIGN KEY (account_id) REFERENCES account(id);
ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_branch FOREIGN KEY (branch_id) REFERENCES branch(id);
ALTER TABLE branch_combo ADD CONSTRAINT fk_branch_combo_combo FOREIGN KEY (combo_id) REFERENCES combo(id);

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
-- INDEXES
-- =============================================================================

-- The vector index is created AFTER the catalog loads: built on an empty table it is
-- suboptimal. Commented out on purpose.
-- CREATE INDEX idx_product_embedding ON product USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE INDEX idx_branch_account ON branch(account_id);
CREATE INDEX idx_app_user_account ON app_user(account_id);
-- Login resolves a user by email alone, so an address identifies exactly one of them across
-- every account. Functional, so case-insensitivity does not depend on the service lowercasing
-- on write the way uq_app_user_email does.
CREATE UNIQUE INDEX uq_app_user_email_global ON app_user (lower(email));
CREATE INDEX idx_refresh_token_user ON refresh_token(user_id);
CREATE INDEX idx_refresh_token_family ON refresh_token(family_id);

CREATE INDEX idx_product_account ON product(account_id);
-- A code identifies one row per account, so "update the price by code" has a single target.
-- Partial because code is nullable.
CREATE UNIQUE INDEX uq_product_account_code ON product (account_id, code) WHERE code IS NOT NULL;
CREATE INDEX idx_branch_product_branch ON branch_product(branch_id) WHERE is_active = TRUE;
CREATE INDEX idx_product_synonym_product ON product_synonym(product_id);
-- One term per product, case-insensitively: "Portland" and "portland" are the same term to
-- a matcher. It is also the ON CONFLICT target the insert names.
CREATE UNIQUE INDEX uq_product_synonym_term
  ON product_synonym (account_id, product_id, lower(term));
CREATE INDEX idx_product_price_product ON product_price(product_id, branch_id);
CREATE INDEX idx_branch_combo_branch ON branch_combo(branch_id) WHERE is_active = TRUE;

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

-- Business invariants expressed in the database, not left to the good will of the code.
-- uq_channel_branch_type_identifier does not compare NULLs, so without this index a branch
-- could hold N identifier-less channels of one type — N manual-entry channels, say, with
-- the rfq pointing at any of them.
CREATE UNIQUE INDEX uq_channel_branch_type_no_identifier
  ON channel (branch_id, type) WHERE identifier IS NULL;
CREATE UNIQUE INDEX uq_quote_version_draft ON quote_version(quote_id) WHERE is_immutable = FALSE;
-- One open price period per branch and product. The service also takes a FOR UPDATE on the
-- parent product row, which is what lets two concurrent repricings both succeed; this index
-- is the backstop that makes a missing lock loud instead of silently duplicating a period.
CREATE UNIQUE INDEX uq_product_price_open_period
  ON product_price (branch_id, product_id) WHERE valid_to IS NULL;
CREATE UNIQUE INDEX uq_message_batch_open ON message_batch(quote_id) WHERE status = 'OPEN';
CREATE UNIQUE INDEX uq_message_batch_processing ON message_batch(quote_id) WHERE status = 'PROCESSING';

-- =============================================================================
-- updated_at TRIGGERS (only tables that mutate in place)
-- =============================================================================

CREATE TRIGGER trg_account_updated        BEFORE UPDATE ON account        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_branch_updated         BEFORE UPDATE ON branch         FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_app_user_updated       BEFORE UPDATE ON app_user       FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_product_updated        BEFORE UPDATE ON product        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_branch_product_updated BEFORE UPDATE ON branch_product FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_combo_updated          BEFORE UPDATE ON combo          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_branch_combo_updated   BEFORE UPDATE ON branch_combo   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_client_updated         BEFORE UPDATE ON client         FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_channel_updated        BEFORE UPDATE ON channel        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_rfq_updated            BEFORE UPDATE ON rfq            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_quote_updated          BEFORE UPDATE ON quote          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_promotion_updated      BEFORE UPDATE ON promotion      FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- =============================================================================
-- RESTRICTED ROLE + RLS
-- =============================================================================
--
-- Two roles hold the isolation up:
--   * the owner (this script, the migrations, the follow-up cron and the pre-auth lookups)
--     bypasses RLS because it owns the tables. That is what the jobs and the reads which
--     legitimately cross accounts need: login by email, and resolving
--     quote_send.public_token for the sessionless webapp;
--   * coti_app has DML but is NOT the owner and is NOBYPASSRLS, so every request connection
--     is subject to the policies.
--
-- Each transaction sets app.current_account_id, re-applied on every BEGIN because the pool
-- reuses connections. With no GUC the policy matches no row.
--
-- ENABLE rather than FORCE is deliberate: FORCE would subject the owner too.
--
-- Only the account is enforced. Branch scoping stays in the application: an admin
-- legitimately reads every branch of their own account.

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

-- account matches on its own id; everything else on its account_id column.
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
    'combo', 'combo_item', 'branch_combo',
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
