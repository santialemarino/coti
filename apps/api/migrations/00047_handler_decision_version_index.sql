-- The seller's detail screen reads the latest handler decision on the draft it shows.

-- +goose Up
CREATE INDEX idx_handler_decision_version ON handler_decision (quote_version_id, created_at);

-- +goose Down
DROP INDEX idx_handler_decision_version;
