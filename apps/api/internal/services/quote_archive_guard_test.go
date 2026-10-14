package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
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
