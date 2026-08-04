package domain

import (
	"testing"
	"time"
)

// The admission predicates. IsUsable is the one every entry point asks instead of
// AppUser.IsActive, so a user row alone cannot say the corralón is open.

var fixedNow = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

func TestAuthSubject_IsUsable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		userActive      bool
		accountIsActive bool
		want            bool
	}{
		{"both active", true, true, true},
		{"the user is deactivated", false, true, false},
		{"the corralón is deactivated", true, false, false},
		{"neither is active", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			subject := AuthSubject{
				AppUser:         AppUser{IsActive: tc.userActive},
				AccountIsActive: tc.accountIsActive,
			}
			if got := subject.IsUsable(); got != tc.want {
				t.Errorf("IsUsable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppUser_IsLocked(t *testing.T) {
	t.Parallel()
	future := fixedNow.Add(time.Minute)
	past := fixedNow.Add(-time.Minute)

	if (AppUser{}).IsLocked(fixedNow) {
		t.Error("IsLocked() = true with no lockout recorded")
	}
	if !(AppUser{LockedUntil: &future}).IsLocked(fixedNow) {
		t.Error("IsLocked() = false inside the lockout window")
	}
	if (AppUser{LockedUntil: &past}).IsLocked(fixedNow) {
		t.Error("IsLocked() = true after the lockout window closed")
	}
	// The boundary is exclusive: at exactly locked_until the lockout is over.
	if (AppUser{LockedUntil: &fixedNow}).IsLocked(fixedNow) {
		t.Error("IsLocked() = true at exactly locked_until, want the window closed")
	}
}

func TestUserRole_IsValid(t *testing.T) {
	t.Parallel()
	for _, role := range []UserRole{UserRoleAdmin, UserRoleSeller} {
		if !role.IsValid() {
			t.Errorf("IsValid() = false for %q, which the user_role enum holds", role)
		}
	}
	for _, role := range []UserRole{"", "admin", "OWNER", "SELLER "} {
		if role.IsValid() {
			t.Errorf("IsValid() = true for %q, which the user_role enum does not hold", role)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	// Every path that stores, looks up or counts an address spells it this way, so the
	// spellings below have to collapse to one value or the paths disagree with each other.
	const want = "vendedor@corralon.test"
	for _, raw := range []string{
		"vendedor@corralon.test",
		"Vendedor@Corralon.test",
		"VENDEDOR@CORRALON.TEST",
		"  vendedor@corralon.test  ",
		"\tVendedor@Corralon.TEST\n",
	} {
		if got := NormalizeEmail(raw); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", raw, got, want)
		}
	}
	// An inner space is not padding, so it is not removed.
	if got := NormalizeEmail("a b@c.test"); got != "a b@c.test" {
		t.Errorf("NormalizeEmail() collapsed an inner space: %q", got)
	}
}

func TestTokens_IsUsable(t *testing.T) {
	t.Parallel()
	consumed := fixedNow.Add(-time.Hour)
	future := fixedNow.Add(time.Hour)
	past := fixedNow.Add(-time.Minute)

	t.Run("refresh token", func(t *testing.T) {
		t.Parallel()
		if !(RefreshToken{ExpiresAt: future}).IsUsable(fixedNow) {
			t.Error("IsUsable() = false for a live token")
		}
		if (RefreshToken{ExpiresAt: past}).IsUsable(fixedNow) {
			t.Error("IsUsable() = true for an expired token")
		}
		if (RefreshToken{ExpiresAt: future, ConsumedAt: &consumed}).IsUsable(fixedNow) {
			t.Error("IsUsable() = true for a consumed token")
		}
		if (RefreshToken{ExpiresAt: future, RevokedAt: &consumed}).IsUsable(fixedNow) {
			t.Error("IsUsable() = true for a revoked token")
		}
	})

	t.Run("single-use link", func(t *testing.T) {
		t.Parallel()
		if !(AuthToken{ExpiresAt: future}).IsUsable(fixedNow) {
			t.Error("IsUsable() = false for a live link")
		}
		if (AuthToken{ExpiresAt: past}).IsUsable(fixedNow) {
			t.Error("IsUsable() = true for an expired link")
		}
		if (AuthToken{ExpiresAt: future, ConsumedAt: &consumed}).IsUsable(fixedNow) {
			t.Error("IsUsable() = true for an already-redeemed link")
		}
	})
}
