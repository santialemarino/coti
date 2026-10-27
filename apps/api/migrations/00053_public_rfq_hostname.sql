-- +goose Up
ALTER TABLE account ADD COLUMN public_hostname VARCHAR(253);
ALTER TABLE account ADD CONSTRAINT uq_account_public_hostname UNIQUE (public_hostname);
ALTER TABLE account ADD CONSTRAINT ck_account_public_hostname
  CHECK (public_hostname = lower(public_hostname) AND public_hostname ~ '^[a-z0-9][a-z0-9.-]*[a-z0-9]$');

-- +goose Down
ALTER TABLE account DROP COLUMN public_hostname;
