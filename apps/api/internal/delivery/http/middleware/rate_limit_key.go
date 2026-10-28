package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RateLimitBy counts requests by what they name rather than by who sends them, so one account cannot
// be probed from many addresses. A request whose key cannot be read is refused with 400, never let
// through uncounted.
func RateLimitBy(limiter Limiter, opts RateLimitOptions, key func(*gin.Context) (string, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !opts.Enabled || opts.Limit <= 0 {
			c.Next()
			return
		}
		k, ok := key(c)
		if !ok {
			_ = c.Error(domain.ErrInvalidInput)
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.ErrorResponse{Error: "invalid request", Code: string(domain.CodeInvalidInput)})
			return
		}
		allowed, retryIn := limiter.Allow(c.Request.Context(), opts.Scope+":"+k, opts.Limit, opts.Window)
		if !allowed {
			refuseRateLimited(c, retryIn)
			return
		}
		c.Next()
	}
}

// JSONBodyKey decodes the body into the handler's own request type, so the key is read exactly as the
// handler will bind it (field names match case-insensitively), and leaves the body for the handler.
// key canonicalizes the value, so every spelling of one value shares a bucket.
func JSONBodyKey[T any](key func(T) (string, bool)) func(*gin.Context) (string, bool) {
	return func(c *gin.Context) (string, bool) {
		if c.Request.Body == nil {
			return "", false
		}
		data, err := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(data))
		if err != nil {
			return "", false
		}
		var body T
		if json.Unmarshal(data, &body) != nil {
			return "", false
		}
		return key(body)
	}
}
