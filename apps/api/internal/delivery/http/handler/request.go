package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// tenantOf returns the tenant resolved for the request, answering 401 when there is none.
//
// The second result is false once the response has been written. Every authenticated
// route sits behind RequireTenant, so a miss here is a wiring bug rather than a client
// error — but it must never fall through to an unscoped query.
func tenantOf(c *gin.Context) (domain.Tenant, bool) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return domain.Tenant{}, false
	}
	return tenant, true
}

// pathUUID reads a UUID path parameter, answering 400 when it is malformed. The second
// result is false once the response has been written.
func pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:  name + " is not a valid uuid",
			Detail: err.Error(),
		})
		return uuid.Nil, false
	}
	return id, true
}
