package domain

import (
	"time"

	"github.com/google/uuid"
)

// SignupInvite is a single-use link the team hands a corralón so it can register while public
// sign-up is closed. Only the hash of the token is stored.
type SignupInvite struct {
	ID        uuid.UUID
	TokenHash string
	Email     *string
	Note      *string
	ExpiresAt time.Time
	UsedAt    *time.Time
	AccountID *uuid.UUID
	CreatedAt time.Time
}

// NewSignupInvite is what the team records beside an invite: who it is for, for its own reference.
type NewSignupInvite struct {
	Email *string
	Note  *string
}

// Redeemable refuses an invite that was already used or has expired at now.
func (i SignupInvite) Redeemable(now time.Time) error {
	if i.UsedAt != nil {
		return WithCode(CodeSignupInviteUsed, ErrForbidden)
	}
	if !now.Before(i.ExpiresAt) {
		return WithCode(CodeSignupInviteExpired, ErrForbidden)
	}
	return nil
}
