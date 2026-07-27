package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// requestIDHeader is both read and echoed, so a request keeps one identity across
// the proxy, the API, and the logs.
const requestIDHeader = "X-Request-Id"

// requestIDContextKey is the Gin context key the request id is stored under.
const requestIDContextKey = "coti.request_id"

// RequestID assigns each request an id, reusing an inbound header when present.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDContextKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the request id assigned to this request.
func RequestIDFrom(c *gin.Context) string {
	return c.GetString(requestIDContextKey)
}

// Logger logs one structured line per request once it completes.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", RequestIDFrom(c)),
		}
		if tenant, ok := TenantFrom(c); ok {
			attrs = append(attrs, slog.String("account_id", tenant.AccountID.String()))
		}

		switch {
		case c.Writer.Status() >= http.StatusInternalServerError:
			attrs = append(attrs, slog.String("errors", c.Errors.String()))
			log.Error("request failed", attrs...)
		case c.Writer.Status() >= http.StatusBadRequest:
			log.Warn("request rejected", attrs...)
		default:
			log.Info("request", attrs...)
		}
	}
}

// Recovery turns a panic into a 500 without leaking the panic value to the client.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					slog.Any("panic", r),
					slog.String("path", c.Request.URL.Path),
					slog.String("request_id", RequestIDFrom(c)),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			}
		}()
		c.Next()
	}
}
