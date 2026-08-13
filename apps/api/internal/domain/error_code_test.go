package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCodeOf_FallsBackToTheSentinelTheErrorWraps(t *testing.T) {
	cases := []struct {
		err  error
		want domain.ErrorCode
	}{
		{domain.ErrNotFound, domain.CodeNotFound},
		{domain.ErrConflict, domain.CodeConflict},
		{domain.ErrImmutable, domain.CodeImmutable},
		{domain.ErrInvalidInput, domain.CodeInvalidInput},
		{domain.ErrUnauthenticated, domain.CodeUnauthenticated},
		{domain.ErrEmailNotVerified, domain.CodeEmailNotVerified},
		{domain.ErrLocked, domain.CodeLocked},
		{domain.ErrForbidden, domain.CodeForbidden},
		{domain.ErrRateLimited, domain.CodeRateLimited},
		{domain.ErrAIUnavailable, domain.CodeAIUnavailable},
		{errors.New("something else entirely"), domain.CodeInternal},
	}

	for _, tc := range cases {
		t.Run(string(tc.want), func(t *testing.T) {
			if got := domain.CodeOf(tc.err); got != tc.want {
				t.Fatalf("CodeOf(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestCodeOf_ReadsTheTagThroughAWrap(t *testing.T) {
	tagged := domain.WithCode(domain.CodeLastActiveBranch,
		fmt.Errorf("%w: an account needs at least one active branch", domain.ErrInvalidInput))
	wrapped := fmt.Errorf("close branch: %w", tagged)

	if got := domain.CodeOf(wrapped); got != domain.CodeLastActiveBranch {
		t.Fatalf("CodeOf(wrapped) = %q, want %q", got, domain.CodeLastActiveBranch)
	}
}

/*
 * The tag decides the code and the sentinel still decides the status, which is the whole point:
 * every layer above keeps matching on the sentinel, and only the envelope reads the code.
 */
func TestWithCode_KeepsTheSentinelMatchable(t *testing.T) {
	err := domain.WithCode(domain.CodeSelfDeactivation,
		fmt.Errorf("%w: an admin cannot deactivate themselves", domain.ErrInvalidInput))

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("a tagged error stopped matching its sentinel")
	}
	if err.Error() == "" {
		t.Fatal("a tagged error lost the message it wraps")
	}
}
