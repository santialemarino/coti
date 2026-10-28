package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
)

func TestRateLimitBy_CountsTheNamedValueAcrossAddressesAndKeepsTheBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/recover", RateLimitBy(ratelimit.NewMemory(nil), defaultOptions(), JSONBodyKey("tax_id", func(v string) string {
		return strings.ReplaceAll(v, "-", "")
	})), func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		c.String(http.StatusOK, string(body))
	})
	post := func(addr, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(body))
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	first := post("1.1.1.1:1", `{"tax_id":"20-12345678-6"}`)
	if first.Code != http.StatusOK || first.Body.String() != `{"tax_id":"20-12345678-6"}` {
		t.Fatalf("first: %d %q, want the body passed on", first.Code, first.Body)
	}
	if rec := post("2.2.2.2:1", `{"tax_id":"20123456786"}`); rec.Code != http.StatusOK {
		t.Fatalf("second spelling: %d", rec.Code)
	}
	if rec := post("3.3.3.3:1", `{"tax_id":"20123456786"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request for the same value from a new address: %d, want 429", rec.Code)
	}
	if rec := post("3.3.3.3:1", `{"tax_id":"30712345671"}`); rec.Code != http.StatusOK {
		t.Fatalf("another value: %d", rec.Code)
	}
	if rec := post("3.3.3.3:1", `not json`); rec.Code != http.StatusOK {
		t.Fatalf("unreadable key: %d, want it passed on to the handler", rec.Code)
	}
}
