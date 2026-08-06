package middleware

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// forwardedForHeader is what a proxy chain appends the address it received from to.
const forwardedForHeader = "X-Forwarded-For"

// Limiter counts requests per key in fixed windows. In-memory today; the interface is what
// lets a shared store take over once there is more than one instance.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, per time.Duration) (bool, time.Duration)
}

// RateLimitOptions is what a limited group or route needs to count.
type RateLimitOptions struct {
	// Scope names the bucket, so a tighter per-route limit counts separately from the
	// global one instead of sharing its allowance.
	Scope string
	Limit int
	// TrustedProxyHops is how many intermediaries sit in front of the API.
	TrustedProxyHops int
	// TrustedProxies are the peers whose forwarding header is believed. Hop counting is
	// only spoof-resistant for a request that actually transited the chain, so a peer
	// outside this list has its header ignored however many hops are configured.
	TrustedProxies []*net.IPNet
	Window         time.Duration
	Enabled        bool
	// Identify reads a stable caller identity from a bearer, when there is one. It is here
	// only to have something to count by and never stands in for session validation.
	Identify func(token string) (string, bool)
}

// RateLimit counts a caller's requests and refuses them past the limit. The key is the
// authenticated user when a bearer is readable, and the client address when it is not.
func RateLimit(limiter Limiter, opts RateLimitOptions) gin.HandlerFunc {
	if !opts.Enabled || opts.Limit <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		key := opts.Scope + ":" + callerKey(c, opts)
		allowed, retryIn := limiter.Allow(c.Request.Context(), key, opts.Limit, opts.Window)
		if allowed {
			c.Next()
			return
		}

		retryAfter := int(math.Ceil(retryIn.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		// Which limit was reached is deliberately absent: it would tell a caller probing the
		// API how its buckets are laid out.
		_ = c.Error(domain.ErrRateLimited)
		c.AbortWithStatusJSON(http.StatusTooManyRequests, dto.RateLimitResponse{
			Code:              string(domain.CodeRateLimited),
			Error:             "too many requests",
			RetryAfterSeconds: retryAfter,
		})
	}
}

func callerKey(c *gin.Context, opts RateLimitOptions) string {
	if opts.Identify != nil {
		if raw := bearerToken(c); raw != "" {
			if id, valid := opts.Identify(raw); valid {
				return "user:" + id
			}
		}
	}
	return "ip:" + clientIP(c.Request, opts.TrustedProxyHops, opts.TrustedProxies)
}

// clientIP resolves the caller's address, counting hops back from the end the proxies append
// to. See docs/technical/authentication.md for why that end is the only trustworthy one.
func clientIP(r *http.Request, trustedHops int, trustedProxies []*net.IPNet) string {
	peer := hostOnly(r.RemoteAddr)
	// A peer that is not a declared proxy could have written the header itself, so counting
	// hops in it would let any caller pick its own key — or burn someone else's.
	if trustedHops <= 0 || !isTrustedProxy(peer, trustedProxies) {
		return peer
	}

	hops := forwardedHops(r)
	index := len(hops) - trustedHops
	// A chain shorter than configured means the request did not arrive the way it was meant
	// to, so the header cannot be believed at all.
	if index < 0 || index >= len(hops) {
		return peer
	}
	return hops[index]
}

func forwardedHops(r *http.Request) []string {
	var hops []string
	for _, header := range r.Header.Values(forwardedForHeader) {
		for _, entry := range strings.Split(header, ",") {
			if trimmed := strings.TrimSpace(entry); trimmed != "" {
				hops = append(hops, trimmed)
			}
		}
	}
	return hops
}

func isTrustedProxy(peer string, trusted []*net.IPNet) bool {
	address := net.ParseIP(peer)
	if address == nil {
		return false
	}
	for _, network := range trusted {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

func hostOnly(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}
