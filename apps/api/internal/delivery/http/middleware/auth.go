package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// branchHeader carries the active branch. It is a header rather than a token claim
// because a seller switches branch without re-authenticating.
const branchHeader = "X-Branch-Id"

// AccessVerifier parses and verifies an access token. Implemented by the token service.
type AccessVerifier interface {
	ParseAccessToken(raw string) (domain.AccessClaims, error)
}

// SessionChecker confirms the token's session epoch still matches the stored one and
// that the user is active. Implemented by the auth service.
type SessionChecker interface {
	VerifySession(ctx context.Context, accountID, userID uuid.UUID, epoch int) error
}

// Authenticate resolves the tenant from the Authorization header.
//
// The token's signature covers account_id, so the claims are trustworthy enough to
// build a tenant scope from before anything is read from the database — which is what
// breaks the chicken-and-egg of needing an account to run a query and a query to learn
// the account. The session epoch is then checked against the stored value, one indexed
// primary-key read, which is the cost of making logout immediate.
//
// A request with no header passes through unauthenticated; RequireTenant is what
// rejects it. That split keeps optional-auth routes possible without a second
// middleware.
func Authenticate(verifier AccessVerifier, sessions SessionChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearerToken(c)
		if raw == "" {
			c.Next()
			return
		}

		claims, err := verifier.ParseAccessToken(raw)
		if err != nil {
			abortUnauthenticated(c)
			return
		}

		if err := sessions.VerifySession(c.Request.Context(), claims.AccountID, claims.UserID, claims.SessionEpoch); err != nil {
			abortUnauthenticated(c)
			return
		}

		tenant := domain.Tenant{
			AccountID: claims.AccountID,
			UserID:    claims.UserID,
			Role:      claims.Role,
		}
		// An unparsable branch header is ignored rather than fatal: the caller is
		// simply operating account-wide, which admins legitimately do.
		if branchID, parseErr := uuid.Parse(c.GetHeader(branchHeader)); parseErr == nil {
			tenant.BranchID = branchID
		}

		SetTenant(c, tenant)
		c.Next()
	}
}

// RequireAdmin aborts with 403 unless the resolved tenant is an admin. Mount it after
// RequireTenant.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant, ok := TenantFrom(c)
		if !ok {
			abortUnauthenticated(c)
			return
		}
		if !tenant.IsAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": domain.ErrForbidden.Error()})
			return
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func abortUnauthenticated(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": domain.ErrUnauthenticated.Error()})
}
