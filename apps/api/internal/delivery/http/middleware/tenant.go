// Package middleware holds the Gin middleware: request identity and logging, and
// the tenant resolution every request-scoped query depends on.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// tenantContextKey is the Gin context key the resolved tenant is stored under.
const tenantContextKey = "coti.tenant"

// SetTenant stores the resolved tenant on the request context.
func SetTenant(c *gin.Context, tenant domain.Tenant) {
	c.Set(tenantContextKey, tenant)
}

// TenantFrom returns the tenant resolved for this request; the second result is false
// when there is none. Never fall back to an unscoped query — row level security would
// read zero rows and turn a wiring mistake into a silent empty response.
func TenantFrom(c *gin.Context) (domain.Tenant, bool) {
	v, ok := c.Get(tenantContextKey)
	if !ok {
		return domain.Tenant{}, false
	}
	tenant, ok := v.(domain.Tenant)
	return tenant, ok
}

// RequireTenant aborts with 401 unless a tenant was resolved for the request.
func RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := TenantFrom(c); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": domain.ErrUnauthenticated.Error(),
			})
			return
		}
		c.Next()
	}
}
