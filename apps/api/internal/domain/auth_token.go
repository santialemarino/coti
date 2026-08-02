package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuthTokenType is what a single-use link entitles its bearer to do without a session.
type AuthTokenType string

const (
	AuthTokenTypePasswordReset     AuthTokenType = "PASSWORD_RESET"
	AuthTokenTypeEmailVerification AuthTokenType = "EMAIL_VERIFICATION"
)

// AuthToken is a single-use, expiring grant sent to a user's address. Only the SHA-256 of
// the raw value is stored, so the table hands out nothing usable if it leaks.
type AuthToken struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	UserID     uuid.UUID
	Type       AuthTokenType
	TokenHash  string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// IsUsable reports whether the token can still be redeemed at the given time.
func (t AuthToken) IsUsable(now time.Time) bool {
	return t.ConsumedAt == nil && t.ExpiresAt.After(now)
}
