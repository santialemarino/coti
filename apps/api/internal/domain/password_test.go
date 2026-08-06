package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const validPassword = "Corralon-2026!"

func TestPasswordPolicy_AcceptsAPasswordMeetingEveryRule(t *testing.T) {
	if err := (domain.PasswordPolicy{MinLength: 12}).Validate(validPassword); err != nil {
		t.Fatalf("Validate(%q) = %v, want no error", validPassword, err)
	}
}

func TestPasswordPolicy_RefusesWhatItMustRefuse(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{"too short", "Corr-2026!"},
		{"no uppercase", "corralon-2026!"},
		{"no lowercase", "CORRALON-2026!"},
		{"no digit", "Corralon-abcd!"},
		{"no symbol", "Corralon20261"},
		// bcrypt hashes the first 72 bytes and refuses the rest, so the policy has to catch it
		// first: reaching the hash with a longer password fails the write instead of the input.
		{"past bcrypt's limit", "Corralon-2026!" + strings.Repeat("a", domain.PasswordMaxBytes)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (domain.PasswordPolicy{MinLength: 12}).Validate(tc.password)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("Validate(%q) = %v, want ErrInvalidInput", tc.password, err)
			}
		})
	}
}

/*
 * The length floor counts runes and the cap counts bytes, because they answer to different limits:
 * the floor is about how much the caller typed, the cap is bcrypt's own. A password of accented
 * characters is short by one measure and long by the other, and only the byte count is a hard limit.
 */
func TestPasswordPolicy_MeasuresLengthInRunesAndTheCapInBytes(t *testing.T) {
	policy := domain.PasswordPolicy{MinLength: 12}

	accented := "Ñoñerías-2026!" // 14 runes, more than 14 bytes
	if err := policy.Validate(accented); err != nil {
		t.Fatalf("Validate(%q) = %v, want no error", accented, err)
	}

	atTheCap := "Aa1!" + strings.Repeat("é", (domain.PasswordMaxBytes-4)/2)
	if len(atTheCap) > domain.PasswordMaxBytes {
		t.Fatalf("fixture is %d bytes, over the cap", len(atTheCap))
	}
	if err := policy.Validate(atTheCap); err != nil {
		t.Fatalf("Validate(a %d-byte password) = %v, want no error", len(atTheCap), err)
	}
}
