-- An external destination identifies one active inbox across every tenant before row-level
-- security can be scoped to the account it belongs to.

-- +goose Up
CREATE UNIQUE INDEX uq_channel_active_whatsapp_phone_number_id
  ON channel ((config ->> 'phone_number_id'))
  WHERE type = 'WHATSAPP' AND is_active = TRUE
    AND (config ->> 'phone_number_id') IS NOT NULL;

CREATE UNIQUE INDEX uq_channel_active_email_identifier_global
  ON channel (lower(identifier))
  WHERE type = 'EMAIL' AND is_active = TRUE AND identifier IS NOT NULL;

-- +goose Down
DROP INDEX uq_channel_active_email_identifier_global;
DROP INDEX uq_channel_active_whatsapp_phone_number_id;
