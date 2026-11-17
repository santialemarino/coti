package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
)

func TestRFQService_AddItem_PricesQuotedMaterialsAndRecalculatesDiscounts(t *testing.T) {
	t.Parallel()
	h := discountHarness()
	productID := uuid.New()
	price := decimal.RequireFromString("125.50")
	prices := &fakeBranchPrices{prices: map[uuid.UUID]domain.BranchPrice{productID: {Price: price}}}
	h.service.WithPrices(prices)
	item, err := h.service.AddItem(context.Background(), rfqTenant(), testQuoteID, domain.QuoteItemCreate{
		ProductID: &productID, RequestedDescription: "Cement", Quantity: decimal.RequireFromString("2.5"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !item.UnitPriceSnapshot.Valid || !item.UnitPriceSnapshot.Decimal.Equal(price) || !item.Subtotal.Decimal.Equal(decimal.RequireFromString("313.75")) {
		t.Fatalf("added item was not priced: %+v", item)
	}
	if prices.calls != 1 || prices.branches[0] != testBranchID {
		t.Fatal("price was not scoped to the quote branch")
	}
	if len(h.quotes.versionTotals) != 1 || !h.quotes.versionTotals[0].Equal(decimal.RequireFromString("2313.75")) {
		t.Fatalf("totals = %v", h.quotes.versionTotals)
	}
}
