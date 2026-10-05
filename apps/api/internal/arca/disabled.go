package arca

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Disabled stands in when no invoicing provider is bound: every call is refused, so a checkout
// without ARCA set up still boots and the refusal lands on the request that needed it.
type Disabled struct{}

// Issue refuses.
func (Disabled) Issue(context.Context, domain.ARCACredentials, domain.InvoiceRequest) (*domain.InvoiceAuthorization, error) {
	return nil, fmt.Errorf("%w: INVOICING_PROVIDER is disabled", domain.ErrNotConfigured)
}

// LatestAuthorized refuses.
func (Disabled) LatestAuthorized(context.Context, domain.ARCACredentials, string, domain.InvoiceType, int) (*domain.AuthorizedInvoice, error) {
	return nil, fmt.Errorf("%w: INVOICING_PROVIDER is disabled", domain.ErrNotConfigured)
}
