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

type keyedBody struct {
	TaxID string `json:"tax_id"`
}

func TestRateLimitBy_KeysTheValueAsTheHandlerBindsItAndKeepsTheBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/recover", RateLimitBy(ratelimit.NewMemory(nil), defaultOptions(), JSONBodyKey(func(b keyedBody) (string, bool) {
		v := strings.ReplaceAll(b.TaxID, "-", "")
		return v, len(v) == 11
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
	if rec := post("2.2.2.2:1", `{"TAX_ID":"20123456786"}`); rec.Code != http.StatusOK {
		t.Fatalf("second spelling: %d", rec.Code)
	}
	if rec := post("3.3.3.3:1", `{"Tax_Id":"20123456786"}`); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request for the same value, another key case: %d, want 429", rec.Code)
	}
	if rec := post("3.3.3.3:1", `{"tax_id":"30712345671"}`); rec.Code != http.StatusOK {
		t.Fatalf("another value: %d", rec.Code)
	}
	for _, body := range []string{`not json`, `{}`, `{"tax_id":"12"}`, `{"tax_id":7}`} {
		if rec := post("4.4.4.4:1", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("unreadable key %s: %d, want 400", body, rec.Code)
		}
	}
}
