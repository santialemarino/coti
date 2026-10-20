package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestClientService_ValidateForSend_RejectsDuplicateTaxID(t *testing.T) {
	t.Parallel()
	h := newClientServiceHarness()
	taxID := "20123456786"
	existing := domain.Client{ID: uuid.New(), AccountID: testAccountID, TaxID: &taxID}
	h.clients.clients[existing.ID] = existing
	h.clients.matches = []domain.Client{existing}
	quote := domain.Quote{CurrentStatus: domain.QuoteStatusQuoted}
	name := "Customer"
	for _, input := range []*domain.AssociateQuoteClientInput{
		{TaxID: taxID, New: &domain.NewClient{Name: &name}},
		{TaxID: taxID, ClientID: &h.quotes.quote.ID},
	} {
		err := h.service.ValidateForSend(context.Background(), nil, branchTenant(), quote, input)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("duplicate identifier: %v", err)
		}
	}
	err := h.service.ValidateForSend(context.Background(), nil, branchTenant(), quote,
		&domain.AssociateQuoteClientInput{TaxID: taxID, ClientID: &existing.ID})
	if err != nil {
		t.Fatalf("existing profile: %v", err)
	}
	if h.clients.created != nil {
		t.Fatal("validation created a profile")
	}
}
