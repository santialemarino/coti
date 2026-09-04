package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
)

// theProxy is the peer the tests place in front of the API. A forwarding header is only read
// from a declared proxy, so every test that uses one has to say who it is.
const theProxy = "10.0.0.9:1111"

func trustedProxies(t *testing.T, cidr string) []*net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("parse %q: %v", cidr, err)
	}
	return []*net.IPNet{network}
}

// The properties worth testing here are all about the key: who a request counts against.
// Getting that wrong is what lets one caller spend another's allowance, or lets a caller
// mint themselves a fresh one by lying in a header.

const testLimit = 2

func testRouter(t *testing.T, opts RateLimitOptions, now func() time.Time) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(ratelimit.NewMemory(now), opts))
	r.GET("/thing", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func defaultOptions() RateLimitOptions {
	return RateLimitOptions{
		Scope:   "test",
		Limit:   testLimit,
		Window:  time.Minute,
		Enabled: true,
	}
}

// call issues one request and reports the status, plus the retry hint when refused.
func call(r *gin.Engine, remoteAddr string, headers map[string]string) (int, int) {
	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = remoteAddr
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		return rec.Code, 0
	}
	var body struct {
		RetryAfterSeconds int `json:"retry_after_seconds"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body.RetryAfterSeconds
}

func TestRateLimit_RefusesPastTheLimitAndSaysWhenToRetry(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r := testRouter(t, defaultOptions(), clock)

	for i := 1; i <= testLimit; i++ {
		if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i, status)
		}
	}

	status, retryAfter := call(r, "10.0.0.1:1111", nil)
	if status != http.StatusTooManyRequests {
		t.Fatalf("request past the limit = %d, want 429", status)
	}
	if retryAfter <= 0 || retryAfter > 60 {
		t.Fatalf("retry_after_seconds = %d, want a positive value inside the window", retryAfter)
	}

	// Waiting out the window is what makes the hint honest.
	now = now.Add(time.Minute + time.Second)
	if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusOK {
		t.Fatalf("request after the window reset = %d, want 200", status)
	}
}

func TestRateLimit_CountsUnauthenticatedClientsSeparately(t *testing.T) {
	r := testRouter(t, defaultOptions(), nil)

	for i := 0; i < testLimit; i++ {
		call(r, "10.0.0.1:1111", nil)
	}
	if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusTooManyRequests {
		t.Fatalf("the spent client = %d, want 429", status)
	}
	if status, _ := call(r, "10.0.0.2:2222", nil); status != http.StatusOK {
		t.Fatalf("a different client = %d, want 200: allowances are per caller", status)
	}
}

// Two clients behind one proxy have the same peer address, so only the forwarding header
// tells them apart.
func TestRateLimit_CountsClientsBehindOneProxySeparately(t *testing.T) {
	opts := defaultOptions()
	opts.TrustedProxyHops = 1
	opts.TrustedProxies = trustedProxies(t, "10.0.0.9/32")
	r := testRouter(t, opts, nil)

	behindProxy := func(clientIP string) map[string]string {
		return map[string]string{forwardedForHeader: clientIP}
	}

	for i := 0; i < testLimit; i++ {
		call(r, theProxy, behindProxy("203.0.113.1"))
	}
	if status, _ := call(r, theProxy, behindProxy("203.0.113.1")); status != http.StatusTooManyRequests {
		t.Fatalf("the spent client = %d, want 429", status)
	}
	if status, _ := call(r, theProxy, behindProxy("203.0.113.2")); status != http.StatusOK {
		t.Fatalf("a second client behind the same proxy = %d, want 200", status)
	}
}

// The AC that decides whether any of this is worth having: prepending addresses must not
// hand a client a fresh allowance.
func TestRateLimit_ForgedForwardingHeaderCannotWinAFreshAllowance(t *testing.T) {
	opts := defaultOptions()
	opts.TrustedProxyHops = 1
	opts.TrustedProxies = trustedProxies(t, "10.0.0.9/32")
	r := testRouter(t, opts, nil)

	// The proxy appends the real client, so a forged entry ends up ahead of it.
	spend := func(forged string) (int, int) {
		return call(r, theProxy, map[string]string{
			forwardedForHeader: forged + ", 203.0.113.7",
		})
	}

	for i := 0; i < testLimit; i++ {
		spend("198.51.100.1")
	}
	if status, _ := spend("198.51.100.1"); status != http.StatusTooManyRequests {
		t.Fatalf("the spent client = %d, want 429", status)
	}
	if status, _ := spend("198.51.100.99"); status != http.StatusTooManyRequests {
		t.Fatalf("the same client with a different forged hop = %d, want 429: changing what "+
			"the client writes must not reset its allowance", status)
	}
}

// A chain shorter than the configured hop count means the request did not arrive the way it
// was meant to, so the header cannot be believed at all.
func TestRateLimit_IgnoresAForwardingHeaderWhenTheChainIsTooShort(t *testing.T) {
	opts := defaultOptions()
	opts.TrustedProxyHops = 2
	opts.TrustedProxies = trustedProxies(t, "10.0.0.9/32")
	r := testRouter(t, opts, nil)

	direct := map[string]string{forwardedForHeader: "198.51.100.1"}
	for i := 0; i < testLimit; i++ {
		call(r, theProxy, direct)
	}
	if status, _ := call(r, theProxy, map[string]string{
		forwardedForHeader: "198.51.100.2",
	}); status != http.StatusTooManyRequests {
		t.Fatalf("a short chain with a different claimed client = %d, want 429: the peer "+
			"address is what counted", status)
	}
}

func TestRateLimit_KeysOnTheUserWhenABearerIsReadable(t *testing.T) {
	opts := defaultOptions()
	opts.Identify = func(token string) (string, bool) {
		if token == "unreadable" {
			return "", false
		}
		return token, true
	}
	r := testRouter(t, opts, nil)

	asUser := func(id string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + id}
	}

	// One address, two users: the allowance follows the user, not the connection.
	for i := 0; i < testLimit; i++ {
		call(r, "10.0.0.1:1111", asUser("user-a"))
	}
	if status, _ := call(r, "10.0.0.1:1111", asUser("user-a")); status != http.StatusTooManyRequests {
		t.Fatalf("the spent user = %d, want 429", status)
	}
	if status, _ := call(r, "10.0.0.1:1111", asUser("user-b")); status != http.StatusOK {
		t.Fatalf("a second user on the same address = %d, want 200", status)
	}
	// An unreadable token falls back to the address, which has not been spent by a user key.
	if status, _ := call(r, "10.0.0.5:1111", asUser("unreadable")); status != http.StatusOK {
		t.Fatalf("an unreadable bearer = %d, want 200 with the address as the key", status)
	}
}

func TestRateLimit_DisabledLetsEverythingThrough(t *testing.T) {
	opts := defaultOptions()
	opts.Enabled = false
	r := testRouter(t, opts, nil)

	for i := 0; i < testLimit*3; i++ {
		if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusOK {
			t.Fatalf("request %d with the limiter off = %d, want 200", i+1, status)
		}
	}
}

// Scopes are what let a tighter per-route allowance coexist with the global one instead of
// sharing its counter.
func TestRateLimit_ScopesCountSeparately(t *testing.T) {
	limiter := ratelimit.NewMemory(nil)
	gin.SetMode(gin.TestMode)

	global := defaultOptions()
	global.Scope = "global"
	route := defaultOptions()
	route.Scope = "credentials"
	route.Limit = 1

	r := gin.New()
	r.Use(RateLimit(limiter, global))
	r.GET("/thing", RateLimit(limiter, route), func(c *gin.Context) { c.Status(http.StatusOK) })

	if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusOK {
		t.Fatalf("first request = %d, want 200", status)
	}
	// The tighter scope bites first, with the global allowance still unspent.
	if status, _ := call(r, "10.0.0.1:1111", nil); status != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429 from the tighter scope", status)
	}
}

/*
 * Hop counting is only sound for a request that really transited the chain. A peer that is
 * not a declared proxy could have written the whole header itself, so believing it would let
 * any caller pick its own bucket — or burn a chosen victim's.
 */
func TestRateLimit_IgnoresAForwardingHeaderFromAnUndeclaredPeer(t *testing.T) {
	opts := defaultOptions()
	opts.TrustedProxyHops = 1
	opts.TrustedProxies = trustedProxies(t, "10.0.0.9/32")
	r := testRouter(t, opts, nil)

	// Same claimed client, arriving from somewhere that is not the proxy.
	spend := func(claimed string) (int, int) {
		return call(r, "198.51.100.50:5555", map[string]string{forwardedForHeader: claimed})
	}

	for i := 0; i < testLimit; i++ {
		spend("203.0.113.1")
	}
	if status, _ := spend("203.0.113.2"); status != http.StatusTooManyRequests {
		t.Fatalf("an undeclared peer rotating the header = %d, want 429: its own address is "+
			"what counts", status)
	}
}
