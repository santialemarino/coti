package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

/*
 * Respond translates a domain error into an HTTP response, and is the single mapping point. The
 * status comes from the sentinel; the code comes from domain.CodeOf, so a route answering 422 for
 * two different rules stays readable to a client. Anything unrecognised becomes a 500 with a
 * generic body: an unmapped error is a bug, and its text may not be safe to show.
 */
func Respond(c *gin.Context, err error) {
	code := string(domain.CodeOf(err))

	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "not found", Code: code})
	case errors.Is(err, domain.ErrConflict):
		c.JSON(http.StatusConflict, dto.ErrorResponse{Error: "conflict", Code: code})
	case errors.Is(err, domain.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, dto.ErrorResponse{Error: err.Error(), Code: code})
	case errors.Is(err, domain.ErrInvalidInput):
		c.JSON(http.StatusUnprocessableEntity, dto.ErrorResponse{Error: err.Error(), Code: code})
	case errors.Is(err, domain.ErrUnauthenticated):
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid credentials", Code: code})
	case errors.Is(err, domain.ErrEmailNotVerified):
		// Told plainly on purpose: it is only reachable once the password matched, and the
		// caller cannot get past it without knowing what to do.
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "email not verified", Code: code})
	case errors.Is(err, domain.ErrLocked):
		c.JSON(http.StatusTooManyRequests,
			dto.ErrorResponse{Error: "account temporarily locked", Code: code})
	case errors.Is(err, domain.ErrForbidden):
		c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "forbidden", Code: code})
	case errors.Is(err, domain.ErrImmutable):
		c.JSON(http.StatusConflict, dto.ErrorResponse{Error: "target is immutable", Code: code})
	case errors.Is(err, domain.ErrAIUnavailable):
		// Attached as well: which provider failed, and why, belongs in the log and not in the
		// response. The caller only needs to know the proposal is not coming.
		_ = c.Error(err)
		c.JSON(http.StatusServiceUnavailable,
			dto.ErrorResponse{Error: "ai provider unavailable", Code: code})
	default:
		// Attached so the request log carries the detail the client does not get.
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError,
			dto.ErrorResponse{Error: "internal error", Code: string(domain.CodeInternal)})
	}
}

// RespondBindError reports a request body that failed binding or validation.
func RespondBindError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, dto.ErrorResponse{
		Error:  "invalid request body",
		Code:   string(domain.CodeInvalidBody),
		Detail: err.Error(),
	})
}
