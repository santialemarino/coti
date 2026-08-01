package domain

import (
	"time"

	"github.com/google/uuid"
)

// AppUser is a seller or admin belonging to one account.
//
// SessionEpoch backs immediate logout: it rides in every access token, so bumping it
// invalidates every outstanding one without a blacklist.
type AppUser struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	Name           string
	Email          string
	PasswordHash   string
	Role           UserRole
	IsActive       bool
	SessionEpoch   int
	LastLoginAt    *time.Time
	FailedAttempts int
	LockedUntil    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsLocked reports whether the account is inside a lockout window at the given time.
func (u AppUser) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// UserWithBranches is a user plus the branches they may operate on, which is the shape the
// admin screens read and write.
type UserWithBranches struct {
	AppUser
	BranchIDs []uuid.UUID
}

// NewUser is an admin-created user. The account comes from the tenant scope, never the
// request, and Password is the plaintext the service hashes.
type NewUser struct {
	Name      string
	Email     string
	Password  string
	Role      UserRole
	BranchIDs []uuid.UUID
}

// UserUpdate replaces a user's editable fields. IsActive is nil to leave it alone, so an
// edit form cannot silently revive a deactivated user.
type UserUpdate struct {
	Name      string
	Email     string
	Role      UserRole
	BranchIDs []uuid.UUID
	IsActive  *bool
}

// IsValid reports whether the role is one the schema's user_role enum holds.
func (r UserRole) IsValid() bool {
	return r == UserRoleAdmin || r == UserRoleSeller
}
