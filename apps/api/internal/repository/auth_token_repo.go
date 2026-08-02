package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const authTokenColumns = `id, account_id, user_id, type, token_hash, expires_at,
	consumed_at, created_at`

// AuthTokenRepository owns persistence for auth_token.
type AuthTokenRepository struct{}

// NewAuthTokenRepository builds an AuthTokenRepository.
func NewAuthTokenRepository() *AuthTokenRepository {
	return &AuthTokenRepository{}
}

// GetByHashCrossAccount finds a token by its hash across every account. It runs on the owner
// pool: the bearer presents a link, so the account is what the token reveals.
func (r *AuthTokenRepository) GetByHashCrossAccount(
	ctx context.Context, q Querier, hash string,
) (*domain.AuthToken, error) {
	var t domain.AuthToken
	err := q.QueryRow(ctx,
		`SELECT `+authTokenColumns+` FROM auth_token WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.AccountID, &t.UserID, &t.Type, &t.TokenHash, &t.ExpiresAt,
		&t.ConsumedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create stores a freshly minted token.
func (r *AuthTokenRepository) Create(ctx context.Context, q Querier, t domain.AuthToken) error {
	_, err := q.Exec(ctx,
		`INSERT INTO auth_token (account_id, user_id, type, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		t.AccountID, t.UserID, t.Type, t.TokenHash, t.ExpiresAt)
	return err
}

// Consume redeems a token, returning domain.ErrConflict when it was already redeemed. The
// consumed_at predicate is what makes the link single-use under concurrency.
func (r *AuthTokenRepository) Consume(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE auth_token SET consumed_at = now()
		 WHERE account_id = $1 AND id = $2 AND consumed_at IS NULL`,
		accountID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// InvalidateActive burns every outstanding token of a type for one user, so asking for a new
// link retires the previous one.
func (r *AuthTokenRepository) InvalidateActive(
	ctx context.Context, q Querier, accountID, userID uuid.UUID, tokenType domain.AuthTokenType,
) error {
	_, err := q.Exec(ctx,
		`UPDATE auth_token SET consumed_at = now()
		 WHERE account_id = $1 AND user_id = $2 AND type = $3 AND consumed_at IS NULL`,
		accountID, userID, tokenType)
	return err
}
