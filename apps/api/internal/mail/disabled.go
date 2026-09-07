package mail

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DisabledMailer refuses client delivery when the configured transport only writes to logs.
type DisabledMailer struct{}

// Send reports that no real email transport is available.
func (DisabledMailer) Send(context.Context, domain.EmailMessage) error {
	return fmt.Errorf("%w: email delivery adapter is disabled", domain.ErrNotConfigured)
}

var _ domain.Mailer = DisabledMailer{}
