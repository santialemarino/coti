package services

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func tokenServiceAt(now time.Time) *TokenService {
	return NewTokenService(testSecret, 15*time.Minute, func() time.Time { return now })
}

func tokenTestUser() domain.AppUser {
	return domain.AppUser{
		ID: testUserID, AccountID: testAccountID,
		Role: domain.UserRoleAdmin, SessionEpoch: 7,
	}
}

func TestIssueAndParseAccessToken(t *testing.T) {
	svc := tokenServiceAt(fixedNow)
	user := tokenTestUser()

	raw, expiresAt, err := svc.IssueAccessToken(user)
	if err != nil {
		t.Fatalf("IssueAccessToken() = %v, want no error", err)
	}
	if expiresAt != fixedNow.Add(15*time.Minute) {
		t.Errorf("expiresAt = %v, want %v", expiresAt, fixedNow.Add(15*time.Minute))
	}

	claims, err := svc.ParseAccessToken(raw)
	if err != nil {
		t.Fatalf("ParseAccessToken() = %v, want no error", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", claims.UserID, user.ID)
	}
	if claims.AccountID != user.AccountID {
		t.Errorf("AccountID = %v, want %v", claims.AccountID, user.AccountID)
	}
	if claims.Role != domain.UserRoleAdmin {
		t.Errorf("Role = %q, want %q", claims.Role, domain.UserRoleAdmin)
	}
	if claims.SessionEpoch != 7 {
		t.Errorf("SessionEpoch = %d, want 7", claims.SessionEpoch)
	}
}

func TestParseAccessToken_ExpiredIsRejected(t *testing.T) {
	raw, _, err := tokenServiceAt(fixedNow).IssueAccessToken(tokenTestUser())
	if err != nil {
		t.Fatalf("IssueAccessToken() = %v, want no error", err)
	}

	later := tokenServiceAt(fixedNow.Add(16 * time.Minute))
	_, err = later.ParseAccessToken(raw)
	if err == nil {
		t.Fatal("ParseAccessToken() = nil error for an expired token, want an error")
	}
	// Both the domain error and the underlying cause stay reachable, so a handler can
	// map the status while a log keeps the reason.
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("error is not %v: %v", domain.ErrUnauthenticated, err)
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("expiry cause is not reachable through the wrapper: %v", err)
	}
}

func TestParseAccessToken_WrongSecretIsRejected(t *testing.T) {
	raw, _, err := tokenServiceAt(fixedNow).IssueAccessToken(tokenTestUser())
	if err != nil {
		t.Fatalf("IssueAccessToken() = %v, want no error", err)
	}

	other := NewTokenService("fedcba9876543210fedcba9876543210", 15*time.Minute, func() time.Time { return fixedNow })
	if _, err := other.ParseAccessToken(raw); err == nil {
		t.Error("ParseAccessToken() accepted a token signed with a different secret")
	}
}

// The algorithm is pinned, so a token whose header claims "none" must not be trusted —
// otherwise anyone can forge claims.
func TestParseAccessToken_UnsignedTokenIsRejected(t *testing.T) {
	forged, err := jwt.NewWithClaims(jwt.SigningMethodNone, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   testUserID.String(),
			ExpiresAt: jwt.NewNumericDate(fixedNow.Add(time.Hour)),
		},
		AccountID: testAccountID,
		Role:      domain.UserRoleAdmin,
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build unsigned token: %v", err)
	}

	if _, err := tokenServiceAt(fixedNow).ParseAccessToken(forged); err == nil {
		t.Error("ParseAccessToken() accepted an unsigned token")
	}
}

func TestParseAccessToken_Malformed(t *testing.T) {
	svc := tokenServiceAt(fixedNow)
	for _, raw := range []string{"", "not-a-jwt", "a.b.c"} {
		if _, err := svc.ParseAccessToken(raw); err == nil {
			t.Errorf("ParseAccessToken(%q) = nil error, want an error", raw)
		}
	}
}

// A token whose subject is not a UUID must be rejected rather than yielding a zero user.
func TestParseAccessToken_NonUUIDSubjectIsRejected(t *testing.T) {
	raw, err := jwt.NewWithClaims(signingMethod, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "not-a-uuid",
			ExpiresAt: jwt.NewNumericDate(fixedNow.Add(time.Hour)),
		},
		AccountID: testAccountID,
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := tokenServiceAt(fixedNow).ParseAccessToken(raw); err == nil {
		t.Error("ParseAccessToken() accepted a non-UUID subject")
	}
}

// Without an account the middleware cannot build a tenant scope, so the token is useless
// and must be refused rather than producing an unscoped request.
func TestParseAccessToken_MissingAccountIsRejected(t *testing.T) {
	raw, err := jwt.NewWithClaims(signingMethod, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   testUserID.String(),
			ExpiresAt: jwt.NewNumericDate(fixedNow.Add(time.Hour)),
		},
		AccountID: uuid.Nil,
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := tokenServiceAt(fixedNow).ParseAccessToken(raw); err == nil {
		t.Error("ParseAccessToken() accepted a token with no account_id")
	}
}

func TestHashPassword_ProducesAVerifiableHash(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword() = %v, want no error", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatal("HashPassword() returned the plaintext")
	}
}

func TestHashToken_IsDeterministicAndNotTheInput(t *testing.T) {
	const raw = "some-refresh-token"
	first, second := hashToken(raw), hashToken(raw)

	if first != second {
		t.Error("hashToken() is not deterministic")
	}
	if first == raw {
		t.Error("hashToken() returned the raw value")
	}
	if len(first) != 64 {
		t.Errorf("hashToken() length = %d, want 64 hex chars", len(first))
	}
}
