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
	// AuthTokenTypeInvite lets an admin-created user choose their first password.
	AuthTokenTypeInvite AuthTokenType = "INVITE"
)

// InviteStatus is where an invited user stands, derived from their latest invite link. Empty
// means there is nothing outstanding: never invited, or the user has set a password.
type InviteStatus string

const (
	InviteStatusNone    InviteStatus = ""
	InviteStatusPending InviteStatus = "PENDING"
	InviteStatusExpired InviteStatus = "EXPIRED"
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

// InviteStatusOf derives where an invited user stands from their latest invite link. A link
// already consumed means the user set a password, whichever link they used to do it.
func InviteStatusOf(latest *AuthToken, now time.Time) InviteStatus {
	switch {
	case latest == nil || latest.ConsumedAt != nil:
		return InviteStatusNone
	case latest.ExpiresAt.After(now):
		return InviteStatusPending
	default:
		return InviteStatusExpired
	}
}
