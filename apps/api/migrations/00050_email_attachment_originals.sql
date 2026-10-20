-- +goose Up
ALTER TYPE attachment_type ADD VALUE IF NOT EXISTS 'OTHER';
ALTER TABLE rfq_attachment ADD COLUMN original_filename TEXT;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM rfq_attachment WHERE type::text = 'OTHER') THEN
    RAISE EXCEPTION 'cannot remove OTHER while retained attachments use it';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE rfq_attachment DROP COLUMN original_filename;
ALTER TYPE attachment_type RENAME TO attachment_type_with_other;
CREATE TYPE attachment_type AS ENUM ('IMAGE', 'PDF', 'SPREADSHEET', 'AUDIO', 'TEXT');
ALTER TABLE rfq_attachment ALTER COLUMN type TYPE attachment_type USING type::text::attachment_type;
DROP TYPE attachment_type_with_other;
