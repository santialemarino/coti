package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestSignupInvite_Redeemable(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	used := now.Add(-time.Hour)
	cases := []struct {
		name   string
		invite domain.SignupInvite
		want   domain.ErrorCode
	}{
		{"unused and in time", domain.SignupInvite{ExpiresAt: now.Add(time.Minute)}, ""},
		{"expires this instant", domain.SignupInvite{ExpiresAt: now}, domain.CodeSignupInviteExpired},
		{"expired", domain.SignupInvite{ExpiresAt: now.Add(-time.Minute)}, domain.CodeSignupInviteExpired},
		{"used", domain.SignupInvite{ExpiresAt: now.Add(time.Hour), UsedAt: &used}, domain.CodeSignupInviteUsed},
		// A used invite says so even once it has also expired: that is the more useful answer.
		{"used and expired", domain.SignupInvite{ExpiresAt: now.Add(-time.Minute), UsedAt: &used},
			domain.CodeSignupInviteUsed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.invite.Redeemable(now)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Redeemable() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, domain.ErrForbidden) || domain.CodeOf(err) != tc.want {
				t.Fatalf("Redeemable() = %v (%s), want %s", err, domain.CodeOf(err), tc.want)
			}
		})
	}
}
