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
	session_epoch, last_login_at, failed_attempts, locked_until, created_at, updated_at,
	email_verified_at`

// authSubjectColumns is the same list qualified for the join, plus the one column the
// user row cannot carry: whether the account that owns them is still live.
const authSubjectColumns = `u.id, u.account_id, u.name, u.email, u.password_hash, u.role,
	u.is_active, u.session_epoch, u.last_login_at, u.failed_attempts, u.locked_until,
	u.created_at, u.updated_at, u.email_verified_at, a.is_active`

// An email identifies exactly one user. Two constraints back that: the per-account one, and
// the global functional index login depends on to resolve an address to a single row. Either
// firing means the address is taken, so both map to the same conflict.
const (
	userEmailIndex       = "uq_app_user_email"
	userEmailGlobalIndex = "uq_app_user_email_global"
)

// isEmailTaken reports whether the write failed because the address is already in use.
func isEmailTaken(err error) bool {
	return isUniqueViolation(err, userEmailIndex) || isUniqueViolation(err, userEmailGlobalIndex)
}

// UserRepository owns persistence for app_user.
type UserRepository struct{}

// NewUserRepository builds a UserRepository.
func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// List returns every user in the account, deactivated ones included, ordered by name. An
// admin needs to see a disabled user to re-enable them.
func (r *UserRepository) List(ctx context.Context, q Querier, accountID uuid.UUID) ([]domain.AppUser, error) {
	rows, err := q.Query(ctx,
		`SELECT `+userColumns+` FROM app_user WHERE account_id = $1 ORDER BY name`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.AppUser
	for rows.Next() {
		u, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// GetByID loads one user within the caller's account. Returns domain.ErrNotFound if
// absent.
func (r *UserRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.AppUser, error) {
	return scanUser(q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM app_user WHERE account_id = $1 AND id = $2`,
		accountID, id))
}

// GetAuthSubjectByID loads a user and their account's state together, which is what the
// per-request session check needs.
func (r *UserRepository) GetAuthSubjectByID(
	ctx context.Context, q Querier, accountID, id uuid.UUID,
) (*domain.AuthSubject, error) {
	return scanAuthSubject(q.QueryRow(ctx,
		`SELECT `+authSubjectColumns+`
		 FROM app_user u JOIN account a ON a.id = u.account_id
		 WHERE u.account_id = $1 AND u.id = $2`,
		accountID, id))
}

// GetAuthSubjectByEmailCrossAccount resolves a login across every account. It must run on
// the owner pool: at login the account is not known yet, so a tenant-scoped query would read
// zero rows. uq_app_user_email_global is what makes the answer a single row.
func (r *UserRepository) GetAuthSubjectByEmailCrossAccount(
	ctx context.Context, q Querier, email string,
) (*domain.AuthSubject, error) {
	return scanAuthSubject(q.QueryRow(ctx,
		`SELECT `+authSubjectColumns+`
		 FROM app_user u JOIN account a ON a.id = u.account_id
		 WHERE lower(u.email) = lower($1) LIMIT 1`,
		email))
}

// ExistsByEmailCrossAccount reports whether any account already holds the address. Login
// resolves a user by email alone, so two accounts sharing one would make it ambiguous.
func (r *UserRepository) ExistsByEmailCrossAccount(
	ctx context.Context, q Querier, email string,
) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE lower(email) = lower($1))`, email,
	).Scan(&exists)
	return exists, err
}

// Create inserts a user with an already-hashed password. Returns domain.ErrConflict when the
// address is already in use.
func (r *UserRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewUser, passwordHash string,
) (*domain.AppUser, error) {
	user, err := scanUser(q.QueryRow(ctx,
		`INSERT INTO app_user (account_id, name, email, password_hash, role)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		accountID, in.Name, in.Email, passwordHash, in.Role))
	if isEmailTaken(err) {
		return nil, domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)
	}
	return user, err
}

// Update replaces the user's editable fields. A nil IsActive leaves the flag alone. Returns
// domain.ErrConflict when the new address is already in use.
func (r *UserRepository) Update(
	ctx context.Context, q Querier, accountID, id uuid.UUID, in domain.UserUpdate,
) (*domain.AppUser, error) {
	// A changed address drops the confirmation: the stamp proved the OLD address was reachable
	// and says nothing about the new one. Every SET expression reads the pre-update row, so the
	// comparison sees the address being replaced. Compared folded, like the unique index.
	user, err := scanUser(q.QueryRow(ctx,
		`UPDATE app_user
		 SET name = $3, email = $4::text, role = $5, is_active = COALESCE($6, is_active),
		     email_verified_at = CASE WHEN lower(email) IS DISTINCT FROM lower($4::text)
		                              THEN NULL ELSE email_verified_at END
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+userColumns,
		accountID, id, in.Name, in.Email, in.Role, in.IsActive))
	if isEmailTaken(err) {
		return nil, domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)
	}
	return user, err
}

// UpdateEmail replaces one user's address, and only if their hash still matches the one the
// caller verified, so a password moved meanwhile cannot be spent on this. Returns
// domain.ErrConflict when the address is taken, domain.ErrNotFound when the hash moved.
func (r *UserRepository) UpdateEmail(
	ctx context.Context, q Querier, accountID, id uuid.UUID, email, currentHash string,
) (*domain.AppUser, error) {
	user, err := scanUser(q.QueryRow(ctx,
		`UPDATE app_user
		 SET email = $3::text, email_verified_at = NULL
		 WHERE account_id = $1 AND id = $2 AND password_hash = $4
		 RETURNING `+userColumns,
		accountID, id, email, currentHash))
	if isEmailTaken(err) {
		return nil, domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)
	}
	return user, err
}

// UpdatePassword replaces the stored hash. Returns domain.ErrNotFound if the user is not in
// the account.
func (r *UserRepository) UpdatePassword(
	ctx context.Context, q Querier, accountID, id uuid.UUID, passwordHash string,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE app_user SET password_hash = $3 WHERE account_id = $1 AND id = $2`,
		accountID, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdatePasswordIfCurrent replaces the hash only if it still matches the one the caller
// verified, so a change that raced a recovery cannot undo it. Returns domain.ErrConflict when
// the stored hash moved.
func (r *UserRepository) UpdatePasswordIfCurrent(
	ctx context.Context, q Querier, accountID, id uuid.UUID, currentHash, passwordHash string,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE app_user SET password_hash = $4
		 WHERE account_id = $1 AND id = $2 AND password_hash = $3`,
		accountID, id, currentHash, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// Deactivate disables the user without deleting them, so their quotes keep an author.
func (r *UserRepository) Deactivate(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE app_user SET is_active = FALSE WHERE account_id = $1 AND id = $2`,
		accountID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
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

// MarkEmailVerified stamps the confirmation. Matching no row means the address was verified
// between the caller's read and this write, which is the outcome it wanted either way.
func (r *UserRepository) MarkEmailVerified(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	_, err := q.Exec(ctx,
		`UPDATE app_user SET email_verified_at = now()
		 WHERE account_id = $1 AND id = $2 AND email_verified_at IS NULL`,
		accountID, id)
	return err
}

func scanAuthSubject(row pgx.Row) (*domain.AuthSubject, error) {
	var s domain.AuthSubject
	err := row.Scan(&s.ID, &s.AccountID, &s.Name, &s.Email, &s.PasswordHash, &s.Role,
		&s.IsActive, &s.SessionEpoch, &s.LastLoginAt, &s.FailedAttempts, &s.LockedUntil,
		&s.CreatedAt, &s.UpdatedAt, &s.EmailVerifiedAt, &s.AccountIsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanUser(row pgx.Row) (*domain.AppUser, error) {
	var u domain.AppUser
	err := row.Scan(&u.ID, &u.AccountID, &u.Name, &u.Email, &u.PasswordHash, &u.Role,
		&u.IsActive, &u.SessionEpoch, &u.LastLoginAt, &u.FailedAttempts, &u.LockedUntil,
		&u.CreatedAt, &u.UpdatedAt, &u.EmailVerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
