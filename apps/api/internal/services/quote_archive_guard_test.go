package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// Every write on an archived quote's lines and discounts refuses with QUOTE_ARCHIVED and touches
// nothing, even in a status that would otherwise allow the edit: it has to be unarchived first.
func TestRFQService_LineEdits_RefuseAnArchivedQuote(t *testing.T) {
	archivedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	quantity := decimal.RequireFromString("3")
	cases := []struct {
		name string
		edit func(h *rfqHarness) error
	}{
		{"add item", func(h *rfqHarness) error {
			_, err := h.service.AddItem(context.Background(), rfqTenant(), testQuoteID,
				domain.QuoteItemCreate{RequestedDescription: "arena", Quantity: quantity})
			return err
		}},
		{"update item", func(h *rfqHarness) error {
			_, err := h.service.UpdateItem(context.Background(), rfqTenant(), testQuoteID,
				h.quotes.itemsByVersionID[0].ID, domain.QuoteItemUpdate{Quantity: &quantity})
			return err
		}},
		{"delete item", func(h *rfqHarness) error {
			return h.service.DeleteItem(context.Background(), rfqTenant(), testQuoteID,
				h.quotes.itemsByVersionID[0].ID)
		}},
		{"add discount", func(h *rfqHarness) error {
			_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
				domain.QuoteDiscountCreate{
					Description: "Contado", ActionType: domain.PromotionActionFixedAmount,
					Value: decimal.RequireFromString("100"), Scope: domain.DiscountScopeTotal,
				})
			return err
		}},
		{"update discount", func(h *rfqHarness) error {
			value := decimal.RequireFromString("50")
			_, err := h.service.UpdateDiscount(context.Background(), rfqTenant(), testQuoteID,
				uuid.New(), domain.QuoteDiscountUpdate{Value: &value})
			return err
		}},
		{"delete discount", func(h *rfqHarness) error {
			return h.service.DeleteDiscount(context.Background(), rfqTenant(), testQuoteID,
				uuid.New())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := discountHarness()
			h.quotes.quoteByID.ArchivedAt = &archivedAt

			err := tc.edit(h)

			if domain.CodeOf(err) != domain.CodeQuoteArchived {
				t.Fatalf("err = %v, want QUOTE_ARCHIVED", err)
			}
			if len(h.quotes.itemsByVersionID) != 1 || len(h.discounts.stored) != 0 {
				t.Errorf("lines = %d, discounts = %d, want the quote untouched",
					len(h.quotes.itemsByVersionID), len(h.discounts.stored))
			}
		})
	}
}

// Issuing the invoice of an archived sale is an edit like any other.
func TestInvoiceService_IssueRefusesAnArchivedQuote(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	archivedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f.quotes.quote.ArchivedAt = &archivedAt

	_, err := f.issue(context.Background())

	if domain.CodeOf(err) != domain.CodeQuoteArchived {
		t.Fatalf("err = %v, want QUOTE_ARCHIVED", err)
	}
	if len(f.issuer.requests) != 0 {
		t.Fatal("ARCA was asked for the invoice of an archived quote")
	}
}

func (f *fakeQuoteDrafts) ClaimForEdit(
	ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.Quote, error) {
	f.claims++
	return f.GetByID(ctx, q, accountID, branchID, quoteID)
}

func (f *fakeQuoteRepo) ClaimForEdit(
	ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.Quote, error) {
	return f.GetByID(ctx, q, accountID, branchID, quoteID)
}

// A line edit reads the quote through the claim that locks it and marks the edit as activity, so the
// inactivity sweep can neither archive it mid-edit nor treat a quote being worked as idle.
func TestRFQService_LineEdits_ClaimTheQuote(t *testing.T) {
	h := discountHarness()
	quantity := decimal.RequireFromString("3")

	if _, err := h.service.UpdateItem(context.Background(), rfqTenant(), testQuoteID,
		h.quotes.itemsByVersionID[0].ID, domain.QuoteItemUpdate{Quantity: &quantity}); err != nil {
		t.Fatalf("UpdateItem() = %v, want no error", err)
	}
	if h.quotes.claims != 1 {
		t.Errorf("claims = %d, want 1", h.quotes.claims)
	}
}

// A live link to an archived quote no longer offers it: the customer is told it is unavailable and
// nothing of the frozen version is resolved.
func TestQuoteDeliveryService_ResolvePublic_WithdrawsAnArchivedQuote(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	archivedAt := fixedNow.Add(-time.Hour)
	h.quotes.quote.ArchivedAt = &archivedAt

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if result.Status != domain.PublicQuoteStatusUnavailable {
		t.Errorf("status = %q, want UNAVAILABLE", result.Status)
	}
	if h.representations.calls != 0 {
		t.Errorf("representation resolved %d times for a withdrawn quote", h.representations.calls)
	}
}

// A customer who already answered keeps seeing what they said, archived or not.
func TestQuoteDeliveryService_ResolvePublic_KeepsAnAnsweredArchivedQuote(t *testing.T) {
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	archivedAt := fixedNow.Add(-time.Hour)
	h.quotes.quote.ArchivedAt = &archivedAt
	h.clientActions.existing = &domain.ClientAction{ID: uuid.New(), VersionID: testVersionID,
		Type: domain.ClientActionAccept, CreatedAt: fixedNow}
	h.representations.resolve = &domain.PublicQuoteRepresentation{Status: "ACTIVE",
		ExpiresAt: *send.ExpiresAt}

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if result.Status != "ACTIVE" || result.CustomerStatus == nil {
		t.Errorf("result = %+v, want the answered quote with its answer", result)
	}
}
