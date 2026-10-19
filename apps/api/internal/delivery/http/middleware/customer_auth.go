package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const customerPrincipalKey = "customer_principal"

// CustomerSessionVerifier validates customer sessions within a registered supplier hostname.
type CustomerSessionVerifier interface {
	Authenticate(context.Context, string, string) (*domain.CustomerPrincipal, error)
}

// RequireCustomer authenticates customers without establishing a staff tenant or accepting staff tokens.
func RequireCustomer(verifier CustomerSessionVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := verifier.Authenticate(c.Request.Context(), c.Param("hostname"), bearerToken(c))
		if err != nil && !errors.Is(err, domain.ErrUnauthenticated) && !errors.Is(err, domain.ErrNotFound) {
			_ = c.Error(err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "customer authentication unavailable"})
			return
		}
		if err != nil || principal == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "customer authentication required"})
			return
		}
		c.Set(customerPrincipalKey, *principal)
		c.Next()
	}
}

// CustomerFrom reads the customer scope established by RequireCustomer.
func CustomerFrom(c *gin.Context) (domain.CustomerPrincipal, bool) {
	value, exists := c.Get(customerPrincipalKey)
	if !exists {
		return domain.CustomerPrincipal{}, false
	}
	principal, ok := value.(domain.CustomerPrincipal)
	return principal, ok
}
