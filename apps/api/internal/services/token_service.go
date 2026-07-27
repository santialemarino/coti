// Package services holds business logic: it orchestrates use cases, owns the
// transaction boundary, and enforces every product invariant. No HTTP, no SQL strings.
package services

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// signingMethod is fixed rather than read from the token, so a forged header cannot
// downgrade verification.
var signingMethod = jwt.SigningMethodHS256

// accessClaims is what an access token carries.
//
// The active branch is deliberately absent: a seller switches branch without
// re-authenticating, so it is resolved per request. SessionEpoch is what makes logout
// immediate — a token whose epoch trails the user's stored one is rejected.
type accessClaims struct {
	jwt.RegisteredClaims
	AccountID    uuid.UUID       `json:"account_id"`
	Role         domain.UserRole `json:"role"`
	SessionEpoch int             `json:"session_epoch"`
}

// TokenService issues and verifies access tokens.
type TokenService struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

// NewTokenService builds a TokenService. now is injectable so expiry is testable
// without sleeping.
func NewTokenService(secret string, accessTTL time.Duration, now func() time.Time) *TokenService {
	if now == nil {
		now = time.Now
	}
	return &TokenService{secret: []byte(secret), accessTTL: accessTTL, now: now}
}

// IssueAccessToken signs a short-lived token for the user. Returns the token and the
// moment it expires.
func (s *TokenService) IssueAccessToken(user domain.AppUser) (string, time.Time, error) {
	issuedAt := s.now()
	expiresAt := issuedAt.Add(s.accessTTL)

	token := jwt.NewWithClaims(signingMethod, accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		AccountID:    user.AccountID,
		Role:         user.Role,
		SessionEpoch: user.SessionEpoch,
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccessToken verifies the signature and expiry and returns the claims.
//
// The claims are trustworthy because the signature covers them, which is what lets the
// middleware build a tenant scope from the token before it has read anything from the
// database. It does NOT check session_epoch — that needs the stored value, so the
// caller compares it.
func (s *TokenService) ParseAccessToken(raw string) (domain.AccessClaims, error) {
	var claims accessClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(*jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{signingMethod.Alg()}), jwt.WithTimeFunc(s.now))
	if err != nil {
		return domain.AccessClaims{}, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.AccessClaims{}, fmt.Errorf("%w: subject is not a uuid", domain.ErrUnauthenticated)
	}
	if claims.AccountID == uuid.Nil {
		return domain.AccessClaims{}, fmt.Errorf("%w: missing account_id", domain.ErrUnauthenticated)
	}

	return domain.AccessClaims{
		UserID:       userID,
		AccountID:    claims.AccountID,
		Role:         claims.Role,
		SessionEpoch: claims.SessionEpoch,
	}, nil
}
