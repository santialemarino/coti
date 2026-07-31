package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Respond translates a domain error into an HTTP response.
//
// This is the single mapping point, so services stay free of HTTP concerns and no
// handler invents its own status codes. Anything unrecognised becomes a 500 with a
// generic body: an unmapped error is a bug, and its text may not be safe to show.
func Respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "not found"})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, dto.ErrorResponse{Error: "conflict"})
	case errors.Is(err, domain.ErrInvalidInput):
		c.JSON(http.StatusUnprocessableEntity, dto.ErrorResponse{Error: err.Error()})
	case errors.Is(err, domain.ErrUnauthenticated):
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid credentials"})
	case errors.Is(err, domain.ErrLocked):
		c.JSON(http.StatusTooManyRequests, dto.ErrorResponse{Error: "account temporarily locked"})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "forbidden"})
	case errors.Is(err, domain.ErrImmutable):
		c.JSON(http.StatusConflict, dto.ErrorResponse{Error: "target is immutable"})
	default:
		// Attached so the request log carries the detail the client does not get.
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "internal error"})
	}
}

// RespondBindError reports a request body that failed binding or validation.
func RespondBindError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, dto.ErrorResponse{
		Error:  "invalid request body",
		Detail: err.Error(),
	})
}
