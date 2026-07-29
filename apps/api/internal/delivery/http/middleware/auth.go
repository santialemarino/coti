package middleware

import (
	"context"
	"errors"
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

// TenantResolver turns verified claims plus a requested branch into the request's tenant
// scope, rejecting a stale session or an inaccessible branch. Implemented by the auth
// service.
type TenantResolver interface {
	ResolveTenant(ctx context.Context, claims domain.AccessClaims, requestedBranch uuid.UUID) (domain.Tenant, error)
}

// Authenticate resolves the tenant from the Authorization header.
//
// The token's signature covers account_id, so the claims are trustworthy enough to build
// a tenant scope from before anything is read from the database — which is what breaks
// the chicken-and-egg of needing an account to run a query and a query to learn the
// account. Everything the signature cannot vouch for (the user still exists and is
// active, the session epoch is current, the requested branch is one this caller may use)
// is checked by the resolver.
//
// An **absent** branch header means account-wide, which admins legitimately do. A
// **present but inaccessible** branch is a 403, not a silent downgrade to account-wide:
// the caller must not end up believing they are scoped to one branch while reading all of
// them. A malformed one is a 400, because it is a client bug either way.
//
// A request with no Authorization header passes through unauthenticated; RequireTenant is
// what rejects it. That split keeps optional-auth routes possible without a second
// middleware.
func Authenticate(verifier AccessVerifier, resolver TenantResolver) gin.HandlerFunc {
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

		var requestedBranch uuid.UUID
		if header := c.GetHeader(branchHeader); header != "" {
			parsed, parseErr := uuid.Parse(header)
			if parseErr != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest,
					gin.H{"error": branchHeader + " is not a valid uuid"})
				return
			}
			requestedBranch = parsed
		}

		tenant, err := resolver.ResolveTenant(c.Request.Context(), claims, requestedBranch)
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				c.AbortWithStatusJSON(http.StatusForbidden,
					gin.H{"error": "branch not accessible"})
				return
			}
			abortUnauthenticated(c)
			return
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
