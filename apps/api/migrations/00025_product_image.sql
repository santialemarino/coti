-- +goose Up
ALTER TABLE product ADD COLUMN image_id UUID;

-- +goose Down
ALTER TABLE product DROP COLUMN image_id;
