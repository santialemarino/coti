package domain

import (
	"time"

	"github.com/google/uuid"
)

// Credentials is a login attempt.
type Credentials struct {
	Email      string
	Password   string
	RememberMe bool
}

// TokenPair is what a successful login or refresh returns. RefreshToken is the raw
// value, handed to the client once and stored only as a hash.
type TokenPair struct {
	AccessToken     string
	AccessExpiresAt time.Time
	RefreshToken    string
	Tenant          Tenant
}

// RefreshToken is one link in a rotation family.
//
// Only the SHA-256 of the raw value is stored: the raw token is high-entropy, so a
// fast hash is enough (unlike a password) and a database leak cannot reconstruct a
// live token. FamilyID ties a chain of rotations together so theft can revoke all of
// it at once. ConsumedAt marks a token as already rotated — re-presenting one past the
// grace window is treated as theft, not a race.
type RefreshToken struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	UserID     uuid.UUID
	FamilyID   uuid.UUID
	TokenHash  string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// IsUsable reports whether the token can still be rotated at the given time.
func (t RefreshToken) IsUsable(now time.Time) bool {
	return t.RevokedAt == nil && t.ConsumedAt == nil && t.ExpiresAt.After(now)
}

// AccessClaims is the trusted content of a verified access token. It lives in the
// domain because both the token service that produces it and the middleware that
// consumes it need the same shape.
//
// The active branch is deliberately absent: a seller switches branch without
// re-authenticating, so it is resolved per request.
type AccessClaims struct {
	UserID       uuid.UUID
	AccountID    uuid.UUID
	Role         UserRole
	SessionEpoch int
}
