package services

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type customerQuoteHostnameReader interface {
	GetCustomerHostname(context.Context, repository.Querier, uuid.UUID) (string, error)
}

// customerPortal resolves the base URL of a supplier's customer webapp.
type customerPortal struct {
	hostnames   customerQuoteHostnameReader
	localOrigin string // overrides every supplier's hostname on a local stack.
	fallback    string // the shared webapp, for a supplier without a hostname of its own.
}

// WithCustomerHostnames routes outgoing links to each supplier's registered customer portal.
func (s *QuoteDeliveryService) WithCustomerHostnames(reader customerQuoteHostnameReader, localOrigin string) *QuoteDeliveryService {
	s.customerHostnames = reader
	s.customerLocalOrigin = localOrigin
	return s
}

func (s *QuoteDeliveryService) customerQuoteBase(ctx context.Context, accountID uuid.UUID) (string, error) {
	portal := customerPortal{hostnames: s.customerHostnames, localOrigin: s.customerLocalOrigin, fallback: s.webappURL}
	if portal.localOrigin != "" || portal.hostnames == nil {
		return portal.base(ctx, nil, accountID)
	}
	var base string
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q repository.Querier) error {
		var err error
		base, err = portal.base(ctx, q, accountID)
		return err
	})
	return base, err
}

// base is the portal's origin for the account, or the shared webapp when it has no site of its own.
func (p customerPortal) base(ctx context.Context, q repository.Querier, accountID uuid.UUID) (string, error) {
	origin, err := p.siteOrigin(ctx, q, accountID)
	if err != nil || origin != "" {
		return origin, err
	}
	return strings.TrimRight(p.fallback, "/"), nil
}

// siteOrigin is the account's own customer site, empty when it has none: sign-up resolves the
// supplier by hostname, so the shared webapp cannot register anyone. q is read only for a lookup.
func (p customerPortal) siteOrigin(ctx context.Context, q repository.Querier, accountID uuid.UUID) (string, error) {
	if p.localOrigin != "" {
		return strings.TrimRight(p.localOrigin, "/"), nil
	}
	if p.hostnames == nil {
		return "", nil
	}
	hostname, err := p.hostnames.GetCustomerHostname(ctx, q, accountID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return "https://" + hostname, nil
}

func customerQuoteURL(base, token string) string { return base + "/quotes/" + url.PathEscape(token) }

// customerSignupURL is the page where a customer creates the account that carries their CUIT.
func customerSignupURL(base string) string { return base + "/signup" }
