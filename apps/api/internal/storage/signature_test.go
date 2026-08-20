package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const testSecret = "a-signing-secret-of-at-least-32-chars"

func TestURLSigner_Verify_AcceptsWhatItSigned(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	signer := NewURLSigner([]byte(testSecret), func() time.Time { return now })
	expiresAt := now.Add(15 * time.Minute)

	if err := signer.Verify("accounts/a/rfqs/b/plan.pdf", expiresAt,
		signer.Sign("accounts/a/rfqs/b/plan.pdf", expiresAt)); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestURLSigner_Verify_RefusesAnExpiredDeadline(t *testing.T) {
	t.Parallel()
	issuedAt := time.Unix(1_700_000_000, 0)
	expiresAt := issuedAt.Add(15 * time.Minute)
	signature := NewURLSigner([]byte(testSecret), func() time.Time { return issuedAt }).
		Sign("accounts/a/rfqs/b/plan.pdf", expiresAt)

	// One second past the deadline, holding the signature that was valid a moment earlier.
	late := NewURLSigner([]byte(testSecret), func() time.Time { return expiresAt.Add(time.Second) })
	err := late.Verify("accounts/a/rfqs/b/plan.pdf", expiresAt, signature)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if got := domain.CodeOf(err); got != domain.CodeLinkExpired {
		t.Fatalf("code = %q, want %q", got, domain.CodeLinkExpired)
	}
}

func TestURLSigner_Verify_RefusesTheInstantTheDeadlinePasses(t *testing.T) {
	t.Parallel()
	issuedAt := time.Unix(1_700_000_000, 0)
	expiresAt := issuedAt.Add(15 * time.Minute)
	signature := NewURLSigner([]byte(testSecret), func() time.Time { return issuedAt }).
		Sign("accounts/a/rfqs/b/plan.pdf", expiresAt)

	// The deadline is the first instant that no longer serves, not the last that does.
	at := NewURLSigner([]byte(testSecret), func() time.Time { return expiresAt })
	if err := at.Verify("accounts/a/rfqs/b/plan.pdf", expiresAt, signature); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestURLSigner_Verify_RefusesATamperedRequest(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	signer := NewURLSigner([]byte(testSecret), func() time.Time { return now })
	const key = "accounts/aaaa/rfqs/b/plan.pdf"
	expiresAt := now.Add(15 * time.Minute)
	signature := signer.Sign(key, expiresAt)

	cases := []struct {
		name      string
		key       string
		expiresAt time.Time
		signature string
	}{
		// The key names the account, so this is one tenant reaching for another's object with
		// a link it was legitimately given.
		{"another account's key", "accounts/bbbb/rfqs/b/plan.pdf", expiresAt, signature},
		{"another object in the same account", "accounts/aaaa/rfqs/b/other.pdf", expiresAt, signature},
		{"a deadline pushed further out", key, expiresAt.Add(time.Hour), signature},
		{"a forged signature", key, expiresAt, "not-the-signature"},
		{"no signature", key, expiresAt, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := signer.Verify(tc.key, tc.expiresAt, tc.signature)
			if !errors.Is(err, domain.ErrForbidden) {
				t.Fatalf("err = %v, want ErrForbidden", err)
			}
			if got := domain.CodeOf(err); got != domain.CodeInvalidLink {
				t.Fatalf("code = %q, want %q", got, domain.CodeInvalidLink)
			}
		})
	}
}

func TestURLSigner_Verify_RefusesASignatureFromAnotherSecret(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	expiresAt := now.Add(15 * time.Minute)
	const key = "accounts/a/rfqs/b/plan.pdf"

	other := NewURLSigner([]byte("a-different-secret-of-at-least-32-chars"), func() time.Time { return now })
	ours := NewURLSigner([]byte(testSecret), func() time.Time { return now })

	if err := ours.Verify(key, expiresAt, other.Sign(key, expiresAt)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestURLSigner_Deadline_IsWholeSecondsSoTheSignatureCoversIt(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 500_000_000)
	signer := NewURLSigner([]byte(testSecret), func() time.Time { return now })

	// Sign covers expiresAt.Unix(), so a deadline carrying sub-second precision would verify
	// against a different instant than the one the link was issued for.
	deadline := signer.deadline(15 * time.Minute)
	if deadline.Nanosecond() != 0 {
		t.Fatalf("deadline = %s, want whole seconds", deadline)
	}
	if err := signer.Verify("k", deadline, signer.Sign("k", deadline)); err != nil {
		t.Fatalf("verify: %v", err)
	}
}
