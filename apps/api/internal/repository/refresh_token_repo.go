package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const refreshTokenColumns = `id, account_id, user_id, family_id, token_hash,
	expires_at, consumed_at, revoked_at, created_at`

// RefreshTokenRepository owns persistence for refresh_token.
type RefreshTokenRepository struct{}

// NewRefreshTokenRepository builds a RefreshTokenRepository.
func NewRefreshTokenRepository() *RefreshTokenRepository {
	return &RefreshTokenRepository{}
}

// GetByHashCrossAccount finds a token by its hash across every account. It runs on the
// owner pool because the caller learns the account from the token, not before it.
func (r *RefreshTokenRepository) GetByHashCrossAccount(ctx context.Context, q Querier, hash string) (*domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := q.QueryRow(ctx,
		`SELECT `+refreshTokenColumns+` FROM refresh_token WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.AccountID, &t.UserID, &t.FamilyID, &t.TokenHash,
		&t.ExpiresAt, &t.ConsumedAt, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create stores a freshly minted token.
func (r *RefreshTokenRepository) Create(ctx context.Context, q Querier, t domain.RefreshToken) error {
	_, err := q.Exec(ctx,
		`INSERT INTO refresh_token (account_id, user_id, family_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		t.AccountID, t.UserID, t.FamilyID, t.TokenHash, t.ExpiresAt)
	return err
}

// Consume marks a token as rotated. The WHERE clause makes it a no-op on an
// already-consumed row, so two concurrent refreshes cannot both rotate the same token.
// Returns domain.ErrConflict when it was already consumed.
func (r *RefreshTokenRepository) Consume(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE refresh_token SET consumed_at = now()
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

// RevokeFamily revokes every unrevoked token in a family. Used on logout and on
// detected reuse, where the whole chain is considered compromised.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, q Querier, accountID, familyID uuid.UUID) error {
	_, err := q.Exec(ctx,
		`UPDATE refresh_token SET revoked_at = now()
		 WHERE account_id = $1 AND family_id = $2 AND revoked_at IS NULL`,
		accountID, familyID)
	return err
}

// DeleteExpired removes tokens that expired before the cutoff, keeping the table
// bounded. Runs across accounts on the owner pool, like the other maintenance sweeps.
func (r *RefreshTokenRepository) DeleteExpiredCrossAccount(ctx context.Context, q Querier) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM refresh_token WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
