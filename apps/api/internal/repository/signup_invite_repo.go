package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const signupInviteColumns = `id, token_hash, email, note, expires_at, used_at, account_id, created_at`

// SignupInviteRepository owns persistence for signup_invite. The table is pre-account, so every
// method runs on the owner role.
type SignupInviteRepository struct{}

// NewSignupInviteRepository builds a SignupInviteRepository.
func NewSignupInviteRepository() *SignupInviteRepository {
	return &SignupInviteRepository{}
}

// GetByHash finds an invite by the hash of its token. Returns domain.ErrNotFound if none matches.
func (r *SignupInviteRepository) GetByHash(
	ctx context.Context, q Querier, hash string,
) (*domain.SignupInvite, error) {
	return r.get(ctx, q, `SELECT `+signupInviteColumns+` FROM signup_invite WHERE token_hash = $1`, hash)
}

// GetByHashForUpdate is GetByHash under a row lock, so two registrations cannot both redeem it.
func (r *SignupInviteRepository) GetByHashForUpdate(
	ctx context.Context, q Querier, hash string,
) (*domain.SignupInvite, error) {
	return r.get(ctx, q,
		`SELECT `+signupInviteColumns+` FROM signup_invite WHERE token_hash = $1 FOR UPDATE`, hash)
}

// Create stores a freshly minted invite.
func (r *SignupInviteRepository) Create(
	ctx context.Context, q Querier, in domain.NewSignupInvite, hash string, expiresAt time.Time,
) (*domain.SignupInvite, error) {
	return r.get(ctx, q,
		`INSERT INTO signup_invite (token_hash, email, note, expires_at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+signupInviteColumns,
		hash, in.Email, in.Note, expiresAt)
}

// MarkUsed records the account an invite registered. Returns domain.ErrConflict when it was
// already used.
func (r *SignupInviteRepository) MarkUsed(
	ctx context.Context, q Querier, id, accountID uuid.UUID, at time.Time,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE signup_invite SET used_at = $3, account_id = $2 WHERE id = $1 AND used_at IS NULL`,
		id, accountID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *SignupInviteRepository) get(
	ctx context.Context, q Querier, sql string, args ...any,
) (*domain.SignupInvite, error) {
	var i domain.SignupInvite
	err := q.QueryRow(ctx, sql, args...).Scan(&i.ID, &i.TokenHash, &i.Email, &i.Note,
		&i.ExpiresAt, &i.UsedAt, &i.AccountID, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}
