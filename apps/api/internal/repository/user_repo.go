package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// userColumns keeps the SELECT list, the scan order, and the struct in one place.
const userColumns = `id, account_id, name, email, password_hash, role, is_active,
	session_epoch, last_login_at, failed_attempts, locked_until, created_at, updated_at`

// UserRepository owns persistence for app_user.
type UserRepository struct{}

// NewUserRepository builds a UserRepository.
func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// GetByID loads one user within the caller's account. Returns domain.ErrNotFound if
// absent.
func (r *UserRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.AppUser, error) {
	return scanUser(q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM app_user WHERE account_id = $1 AND id = $2`,
		accountID, id))
}

// GetByEmailCrossAccount looks a user up by email across every account.
//
// It must run on the owner pool: at login the account is not known yet, so a
// tenant-scoped query would match no policy and read zero rows, failing every login.
// The email is unique per account, so a shared address across accounts would be
// ambiguous here — the pilot has one account per corralón, and resolving that
// ambiguity needs a product decision (an account selector at login), not a silent
// pick.
func (r *UserRepository) GetByEmailCrossAccount(ctx context.Context, q Querier, email string) (*domain.AppUser, error) {
	return scanUser(q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM app_user WHERE lower(email) = lower($1) LIMIT 1`,
		email))
}

// RegisterFailedAttempt increments the failure counter and applies a lockout once it
// reaches maxAttempts. Returns the resulting attempt count.
func (r *UserRepository) RegisterFailedAttempt(
	ctx context.Context, q Querier, accountID, id uuid.UUID, maxAttempts int, lockFor time.Duration,
) (int, error) {
	var attempts int
	err := q.QueryRow(ctx,
		`UPDATE app_user
		 SET failed_attempts = failed_attempts + 1,
		     locked_until = CASE WHEN failed_attempts + 1 >= $3 THEN now() + $4::interval ELSE locked_until END
		 WHERE account_id = $1 AND id = $2
		 RETURNING failed_attempts`,
		accountID, id, maxAttempts, lockFor.String(),
	).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return attempts, err
}

// RegisterSuccessfulLogin clears the failure counter and the lockout, and stamps
// last_login_at.
func (r *UserRepository) RegisterSuccessfulLogin(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	_, err := q.Exec(ctx,
		`UPDATE app_user
		 SET failed_attempts = 0, locked_until = NULL, last_login_at = now()
		 WHERE account_id = $1 AND id = $2`,
		accountID, id)
	return err
}

// BumpSessionEpoch invalidates every outstanding access token for the user. Returns
// the new epoch.
func (r *UserRepository) BumpSessionEpoch(ctx context.Context, q Querier, accountID, id uuid.UUID) (int, error) {
	var epoch int
	err := q.QueryRow(ctx,
		`UPDATE app_user SET session_epoch = session_epoch + 1
		 WHERE account_id = $1 AND id = $2
		 RETURNING session_epoch`,
		accountID, id,
	).Scan(&epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return epoch, err
}

func scanUser(row pgx.Row) (*domain.AppUser, error) {
	var u domain.AppUser
	err := row.Scan(&u.ID, &u.AccountID, &u.Name, &u.Email, &u.PasswordHash, &u.Role,
		&u.IsActive, &u.SessionEpoch, &u.LastLoginAt, &u.FailedAttempts, &u.LockedUntil,
		&u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
