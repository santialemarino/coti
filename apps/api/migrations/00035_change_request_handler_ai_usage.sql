-- +goose Up

ALTER TYPE ai_operation ADD VALUE IF NOT EXISTS 'CHANGE_REQUEST_HANDLING';

-- +goose Down

-- PostgreSQL enum values are intentionally append-only.
