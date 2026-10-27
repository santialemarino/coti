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

// base is the portal's origin for the account; q is only read when a hostname has to be looked up.
func (p customerPortal) base(ctx context.Context, q repository.Querier, accountID uuid.UUID) (string, error) {
	if p.localOrigin != "" {
		return strings.TrimRight(p.localOrigin, "/"), nil
	}
	fallback := strings.TrimRight(p.fallback, "/")
	if p.hostnames == nil {
		return fallback, nil
	}
	hostname, err := p.hostnames.GetCustomerHostname(ctx, q, accountID)
	if errors.Is(err, domain.ErrNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return "https://" + hostname, nil
}

func customerQuoteURL(base, token string) string { return base + "/quotes/" + url.PathEscape(token) }

// customerSignupURL is the page where a customer creates the account that carries their CUIT.
func customerSignupURL(base string) string { return base + "/signup" }
