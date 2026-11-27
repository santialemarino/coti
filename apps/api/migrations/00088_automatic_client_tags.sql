-- +goose Up
CREATE TYPE client_tag_rule_kind AS ENUM ('RECURRING', 'ABANDONER');

ALTER TABLE tag
  ADD COLUMN rule_kind client_tag_rule_kind,
  ADD COLUMN rule_enabled BOOLEAN,
  ADD COLUMN rule_minimum INTEGER,
  ADD COLUMN rule_window_days INTEGER,
  ADD CONSTRAINT ck_tag_rule CHECK (
    (rule_kind IS NULL AND rule_enabled IS NULL AND rule_minimum IS NULL AND rule_window_days IS NULL)
    OR (rule_kind IS NOT NULL AND rule_enabled IS NOT NULL
      AND (rule_minimum IS NULL OR rule_minimum > 0)
      AND (rule_window_days IS NULL OR rule_window_days >= 0))
  ),
  ADD CONSTRAINT uq_tag_account_rule UNIQUE (account_id, rule_kind);

CREATE TABLE client_tag_exclusion (
  account_id UUID NOT NULL,
  client_id  UUID NOT NULL,
  tag_id     UUID NOT NULL,
  PRIMARY KEY (account_id, client_id, tag_id),
  CONSTRAINT fk_client_tag_exclusion_client FOREIGN KEY (account_id, client_id) REFERENCES client(account_id, id),
  CONSTRAINT fk_client_tag_exclusion_tag FOREIGN KEY (account_id, tag_id) REFERENCES tag(account_id, id)
);

ALTER TABLE client_tag_exclusion ENABLE ROW LEVEL SECURITY;
CREATE POLICY client_tag_exclusion_account_isolation ON client_tag_exclusion
  USING (account_id = app_current_account_id()) WITH CHECK (account_id = app_current_account_id());

CREATE INDEX idx_quote_client_tag_activity ON quote (account_id, client_id)
  WHERE current_status IN ('ACCEPTED', 'REJECTED') OR archived_at IS NOT NULL;
CREATE INDEX idx_quote_status_change_client_tag_activity
  ON quote_status_change (account_id, quote_id, new_status, changed_at DESC)
  WHERE new_status IN ('ACCEPTED', 'REJECTED');

DELETE FROM client_tag AS link USING tag
WHERE link.account_id = tag.account_id AND link.tag_id = tag.id
  AND lower(tag.name) IN ('obra grande', 'recurrente', 'abandonador');

DELETE FROM tag WHERE lower(name) = 'obra grande';

INSERT INTO tag (account_id, name, rule_kind, rule_enabled)
SELECT account.id, rules.name, rules.kind::client_tag_rule_kind, TRUE
FROM account
CROSS JOIN (VALUES ('Recurrente', 'RECURRING'), ('Abandonador', 'ABANDONER')) AS rules(name, kind)
ON CONFLICT (account_id, lower(name)) DO UPDATE
SET name = EXCLUDED.name, rule_kind = EXCLUDED.rule_kind, rule_enabled = EXCLUDED.rule_enabled;

-- +goose Down
DROP INDEX idx_quote_status_change_client_tag_activity;
DROP INDEX idx_quote_client_tag_activity;
DROP TABLE client_tag_exclusion;
DELETE FROM tag WHERE rule_kind = 'ABANDONER';
ALTER TABLE tag
  DROP CONSTRAINT ck_tag_rule,
  DROP CONSTRAINT uq_tag_account_rule,
  DROP COLUMN rule_kind,
  DROP COLUMN rule_enabled,
  DROP COLUMN rule_minimum,
  DROP COLUMN rule_window_days;
DROP TYPE client_tag_rule_kind;

INSERT INTO tag (account_id, name)
SELECT id, 'Obra grande' FROM account
ON CONFLICT (account_id, lower(name)) DO NOTHING;
