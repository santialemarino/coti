// Package whatsapp contains adapters for seller-approved outbound WhatsApp delivery.
package whatsapp

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DisabledSender refuses delivery until the Meta adapter is configured by its own ticket.
type DisabledSender struct{}

// SendQuote reports that this deployment has no real WhatsApp transport.
func (DisabledSender) SendQuote(context.Context,
	domain.QuoteWhatsAppMessage) (*domain.DeliveryReceipt, error) {
	return nil, fmt.Errorf("%w: whatsapp adapter is disabled", domain.ErrNotConfigured)
}

var _ domain.QuoteWhatsAppSender = DisabledSender{}
