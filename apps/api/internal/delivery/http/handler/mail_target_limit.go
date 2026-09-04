package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// mailTargetScope is one bucket per address rather than one per route: what is protected is
// the mailbox, and the mailbox does not care which route filled it.
const mailTargetScope = "mail:address"

// MailTargetLimitOptions is the per-address allowance the mail routes share.
type MailTargetLimitOptions struct {
	Limit   int
	Window  time.Duration
	Enabled bool
}

// MailTargetLimiter caps how many messages one address receives, whatever caller asks for
// them: the caller-keyed middleware bounds what the API serves, not what a mailbox gets.
type MailTargetLimiter struct {
	limiter middleware.Limiter
	opts    MailTargetLimitOptions
}

// NewMailTargetLimiter builds the per-address cap over the same counter the middleware uses.
func NewMailTargetLimiter(limiter middleware.Limiter, opts MailTargetLimitOptions) *MailTargetLimiter {
	return &MailTargetLimiter{limiter: limiter, opts: opts}
}

// Allow reports whether one more message may go to this address.
func (l *MailTargetLimiter) Allow(ctx context.Context, email string) bool {
	if !l.opts.Enabled || l.opts.Limit <= 0 {
		return true
	}
	allowed, _ := l.limiter.Allow(ctx, mailTargetKey(email), l.opts.Limit, l.opts.Window)
	return allowed
}

// mailTargetKey is the counter key for an address: hashed, so a dump of the counter store
// holds no mailbox in the clear, and normalised, so case and padding count as one address.
func mailTargetKey(email string) string {
	sum := sha256.Sum256([]byte(domain.NormalizeEmail(email)))
	return mailTargetScope + ":" + hex.EncodeToString(sum[:])
}
