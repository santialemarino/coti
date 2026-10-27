-- +goose Up
-- A client's CUIT or CUIL (tax_id) comes only from their customer account; a consumidor final's DNI
-- is the seller's to load and lives apart from it. Not unique: one person may have several profiles.
-- +goose StatementBegin
DO $$
DECLARE
  odd INTEGER;
BEGIN
  -- No route ever stored 9 or 10 digits; a hand-edited value that is neither shape is for a person
  -- to settle, not for this migration to guess at.
  SELECT count(*) INTO odd FROM client WHERE tax_id IS NOT NULL AND char_length(tax_id) NOT IN (7, 8, 11);
  IF odd > 0 THEN
    RAISE EXCEPTION '% client tax_id value(s) are neither a DNI (7-8 digits) nor a CUIT (11): fix them before migrating', odd;
  END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE client ADD COLUMN dni VARCHAR(8) CONSTRAINT ck_client_dni CHECK (dni ~ '^[0-9]{7,8}$');

UPDATE client SET dni = tax_id, tax_id = NULL WHERE char_length(tax_id) IN (7, 8);

ALTER TABLE client DROP CONSTRAINT client_tax_id_check;
ALTER TABLE client ADD CONSTRAINT ck_client_tax_id CHECK (tax_id ~ '^[0-9]{11}$');

-- +goose Down
ALTER TABLE client DROP CONSTRAINT ck_client_tax_id;
ALTER TABLE client ADD CONSTRAINT client_tax_id_check CHECK (tax_id ~ '^[0-9]{7,11}$');
-- A profile with both keeps its CUIT; its DNI has nowhere to go in the older shape.
UPDATE client SET tax_id = dni WHERE tax_id IS NULL AND dni IS NOT NULL;
ALTER TABLE client DROP COLUMN dni;
