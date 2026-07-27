package domain

import (
	"time"

	"github.com/google/uuid"
)

// AppUser is a seller or admin belonging to one account.
//
// SessionEpoch backs immediate logout: it is embedded in every access token, and
// bumping it invalidates every outstanding token for the user without a blacklist.
// LockedUntil closes out FailedAttempts — the counter alone cannot express a lockout.
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
