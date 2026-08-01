-- Coti — schema adjustments from the closed data-modelling decisions.
--
-- Every change here is additive, relaxing, or moves a column, so the whole set applies as
-- one transaction or none of it does.

-- +goose Up

-- =============================================================================
-- quote.rfq_id NOT NULL
-- =============================================================================
--
-- uq_quote_rfq alone does not enforce 1-to-1 rfq→quote: Postgres does not compare NULLs,
-- so a nullable rfq_id let unbounded RFQ-less quotes escape it. Every quote is born from an
-- RFQ, manual entry included.
ALTER TABLE quote ALTER COLUMN rfq_id SET NOT NULL;

-- =============================================================================
-- Process timestamps
-- =============================================================================
--
-- A process table carries a timestamp per transition that matters, rather than a generic
-- updated_at that does not say which change it was. Nullable until the transition happens.
ALTER TABLE notification ADD COLUMN sent_at TIMESTAMPTZ;
ALTER TABLE rfq_attachment ADD COLUMN processed_at TIMESTAMPTZ;

-- =============================================================================
-- promotion.name
-- =============================================================================
--
-- name is the short label a seller identifies the promotion by on screen; description stays
-- the long optional text. Three steps because a NOT NULL column with no default fails on a
-- table that already has rows.
ALTER TABLE promotion ADD COLUMN name VARCHAR(128);
UPDATE promotion SET name = COALESCE(NULLIF(left(description, 128), ''), 'Promoción');
ALTER TABLE promotion ALTER COLUMN name SET NOT NULL;

-- =============================================================================
-- Manual-entry channel
-- =============================================================================
--
-- A rename, not a new value: COUNTER already meant entry typed in the backoffice, and a
-- newly added enum value cannot be used in the transaction that added it. The rename is
-- metadata only, so existing rows keep pointing at it.
--
-- MANUAL_ENTRY describes how the order arrived, not where the client stood: a phone order
-- and a message from an unintegrated channel are both manual entry.
ALTER TYPE channel_type RENAME VALUE 'COUNTER' TO 'MANUAL_ENTRY';

-- =============================================================================
-- channel: its own identifier, relaxed uniqueness
-- =============================================================================
--
-- A branch can have two WhatsApp numbers or two mailboxes, so uniqueness now includes which
-- instance of the channel it is. identifier stays NULL for channels a branch can only have
-- one of.
ALTER TABLE channel ADD COLUMN identifier VARCHAR(255);

ALTER TABLE channel DROP CONSTRAINT uq_channel_branch_type;
ALTER TABLE channel ADD CONSTRAINT uq_channel_branch_type_identifier
  UNIQUE (branch_id, type, identifier);

-- The composite constraint does not cover a NULL identifier, which would let a branch hold
-- N manual-entry channels — the same NULL-escapes-unique hole closed above.
CREATE UNIQUE INDEX uq_channel_branch_type_no_identifier
  ON channel (branch_id, type) WHERE identifier IS NULL;

-- Every branch needs its manual-entry channel: without one a counter order has nowhere to
-- originate, and a nullable rfq.channel_id would reopen the hole closed above.
INSERT INTO channel (account_id, branch_id, type)
SELECT b.account_id, b.id, 'MANUAL_ENTRY'
FROM branch b
WHERE NOT EXISTS (
  SELECT 1 FROM channel c WHERE c.branch_id = b.id AND c.type = 'MANUAL_ENTRY'
);

-- On manual entry the client is optional, but the seller still notes who the order is for.
-- The label describes this order, not a person to match against later, so it lives on the
-- rfq — minting a client row per walk-in would make "client optional" false.
ALTER TABLE rfq ADD COLUMN client_label VARCHAR(255);

-- =============================================================================
-- Client origin leaves channel_type
-- =============================================================================
--
-- channel_type and client.origin_channel answered different questions with one type: the
-- mechanism a request arrived by, versus where a client came from. Splitting them is what
-- lets PHONE leave the first without losing "came by phone" in the second.
CREATE TYPE client_origin AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'PHONE', 'WALK_IN');

ALTER TABLE client ALTER COLUMN origin_channel TYPE client_origin
  USING CASE origin_channel::text
          WHEN 'MANUAL_ENTRY' THEN 'WALK_IN'
          ELSE origin_channel::text
        END::client_origin;

-- A phone order typed in by the seller arrives as manual entry, so PHONE is redundant as an
-- intake channel. Postgres cannot drop an enum value, so the type is recreated.
UPDATE channel SET type = 'MANUAL_ENTRY' WHERE type = 'PHONE';

ALTER TYPE channel_type RENAME TO channel_type_old;
CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'MANUAL_ENTRY');
ALTER TABLE channel ALTER COLUMN type TYPE channel_type USING type::text::channel_type;
DROP TYPE channel_type_old;

-- =============================================================================
-- The combo moves to account scope
-- =============================================================================
--
-- The catalog is account-scoped everywhere else, with availability in branch_product and
-- price in product_price. The combo was the one exception.
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

-- A new table without an RLS policy does not fail loudly — it returns every account's rows.
-- The GRANT is already covered by ALTER DEFAULT PRIVILEGES in 00001, but stays explicit.
GRANT SELECT, INSERT, UPDATE, DELETE ON branch_combo TO coti_app;
ALTER TABLE branch_combo ENABLE ROW LEVEL SECURITY;
CREATE POLICY branch_combo_account_isolation ON branch_combo
  USING (account_id = app_current_account_id())
  WITH CHECK (account_id = app_current_account_id());

-- Availability is seeded from the branch the combo used to carry. No price and no stock: a
-- combo's price derives from its items and its stock from its components.
INSERT INTO branch_combo (account_id, branch_id, combo_id)
SELECT account_id, branch_id, id FROM combo;

ALTER TABLE combo DROP COLUMN branch_id;

-- +goose Down

-- The combo carries a branch again. Lossy by definition: one available in two branches comes
-- back with one, and one with no availability cannot come back at all.
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

-- channel_type returns to its five original values and client.origin_channel shares it
-- again. WALK_IN and MANUAL_ENTRY both fold back into COUNTER. The backfilled channels stay:
-- deleting them would break the rfq rows that reference them.
ALTER TYPE channel_type RENAME TO channel_type_new;
CREATE TYPE channel_type AS ENUM ('WHATSAPP', 'EMAIL', 'WEBAPP', 'COUNTER', 'PHONE');
ALTER TABLE channel ALTER COLUMN type TYPE channel_type
  USING CASE type::text WHEN 'MANUAL_ENTRY' THEN 'COUNTER' ELSE type::text END::channel_type;
ALTER TABLE client ALTER COLUMN origin_channel TYPE channel_type
  USING CASE origin_channel::text WHEN 'WALK_IN' THEN 'COUNTER' ELSE origin_channel::text END::channel_type;
DROP TYPE channel_type_new;
DROP TYPE client_origin;

-- Restoring uniqueness on (branch, type) fails if a branch took two channels of one type
-- meanwhile; which of the two to drop is the caller's decision.
DROP INDEX uq_channel_branch_type_no_identifier;
ALTER TABLE channel DROP CONSTRAINT uq_channel_branch_type_identifier;
ALTER TABLE channel DROP COLUMN identifier;
ALTER TABLE channel ADD CONSTRAINT uq_channel_branch_type UNIQUE (branch_id, type);

ALTER TABLE rfq DROP COLUMN client_label;

ALTER TABLE promotion DROP COLUMN name;

ALTER TABLE rfq_attachment DROP COLUMN processed_at;
ALTER TABLE notification DROP COLUMN sent_at;

ALTER TABLE quote ALTER COLUMN rfq_id DROP NOT NULL;
