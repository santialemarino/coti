package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
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
// scope, rejecting a stale session or an inaccessible branch.
type TenantResolver interface {
	ResolveTenant(ctx context.Context, claims domain.AccessClaims, requestedBranch uuid.UUID) (domain.Tenant, error)
}

// Authenticate resolves the tenant from the Authorization header. A request without one
// passes through unauthenticated; RequireTenant is what rejects it.
//
// An absent branch header means account-wide. A present but inaccessible branch is a 403,
// never a silent downgrade: the caller must not read every branch while believing they are
// scoped to one. A malformed one is a 400.
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

// RequireVerifiedEmail aborts with 403 and CodeEmailNotVerified unless the caller has
// confirmed their address. Mount it after RequireTenant, on every route except the three an
// unconfirmed caller needs to stop being one: their own identity, logout, and correcting
// their own address. Issuing a session is not using the product, so login does not check it.
//
// It takes the setting rather than being mounted conditionally, so the route tree is the same
// shape whether the requirement is on or off.
func RequireVerifiedEmail(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		tenant, ok := TenantFrom(c)
		if !ok {
			abortUnauthenticated(c)
			return
		}
		if !tenant.EmailVerified {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.ErrorResponse{
				Error: "email not verified",
				Code:  string(domain.CodeEmailNotVerified),
			})
			return
		}
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
