-- The users screen reads each user's newest invite on every load; this index answers it per user
-- without sorting, and carries the id that breaks a created_at tie.

-- +goose Up
CREATE INDEX idx_auth_token_latest_invite
  ON auth_token (account_id, user_id, created_at DESC, id DESC)
  WHERE type = 'INVITE';

-- +goose Down
DROP INDEX idx_auth_token_latest_invite;
