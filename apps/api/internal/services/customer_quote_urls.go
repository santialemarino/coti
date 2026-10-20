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

// WithCustomerHostnames routes outgoing links to each supplier's registered customer portal.
func (s *QuoteDeliveryService) WithCustomerHostnames(reader customerQuoteHostnameReader, localOrigin string) *QuoteDeliveryService {
	s.customerHostnames = reader
	s.customerLocalOrigin = localOrigin
	return s
}

func (s *QuoteDeliveryService) customerQuoteBase(ctx context.Context, accountID uuid.UUID) (string, error) {
	if s.customerLocalOrigin != "" {
		return strings.TrimRight(s.customerLocalOrigin, "/"), nil
	}
	if s.customerHostnames == nil {
		return s.webappURL, nil
	}
	var hostname string
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q repository.Querier) error {
		var err error
		hostname, err = s.customerHostnames.GetCustomerHostname(ctx, q, accountID)
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return s.webappURL, nil
	}
	if err != nil {
		return "", err
	}
	return "https://" + hostname, nil
}

func customerQuoteURL(base, token string) string { return base + "/quotes/" + url.PathEscape(token) }
