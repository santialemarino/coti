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

// SetTenant stores the resolved tenant on the request context. The authentication
// middleware calls it after validating the access token.
func SetTenant(c *gin.Context, tenant domain.Tenant) {
	c.Set(tenantContextKey, tenant)
}

// TenantFrom returns the tenant resolved for this request.
//
// The second result is false when no tenant was resolved, which means the route is
// mounted outside RequireTenant. Callers must not fall back to an unscoped query:
// under row level security that reads zero rows, and treating "no rows" as "no data"
// would turn a wiring mistake into a silent empty response.
func TenantFrom(c *gin.Context) (domain.Tenant, bool) {
	v, ok := c.Get(tenantContextKey)
	if !ok {
		return domain.Tenant{}, false
	}
	tenant, ok := v.(domain.Tenant)
	return tenant, ok
}

// RequireTenant aborts with 401 unless a tenant was resolved for the request.
//
// It is the gate in front of every authenticated route: services reach the database
// through a tenant-scoped transaction, so a request without an account has nothing
// it could legitimately query.
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
