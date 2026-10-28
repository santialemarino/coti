package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"
)

// RateLimitBy counts requests by what they name rather than by who sends them, so one account cannot
// be probed from many addresses. A request whose key cannot be read passes on, for its handler to refuse.
func RateLimitBy(limiter Limiter, opts RateLimitOptions, key func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !opts.Enabled || opts.Limit <= 0 {
			c.Next()
			return
		}
		k := key(c)
		if k == "" {
			c.Next()
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

// JSONBodyKey reads one string field of a JSON body as a rate-limit key, normalised so spellings of
// the same value share a bucket, and leaves the body for the handler to bind.
func JSONBodyKey(field string, normalize func(string) string) func(*gin.Context) string {
	return func(c *gin.Context) string {
		if c.Request.Body == nil {
			return ""
		}
		data, err := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(data))
		if err != nil {
			return ""
		}
		var body map[string]any
		if json.Unmarshal(data, &body) != nil {
			return ""
		}
		value, _ := body[field].(string)
		if value = normalize(value); value == "" {
			return ""
		}
		return field + ":" + value
	}
}
