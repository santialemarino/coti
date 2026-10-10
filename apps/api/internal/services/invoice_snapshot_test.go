package services

import (
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
	"testing"
)

func TestFiscalSnapshot_UsesImmutableLineIdentityAcrossReordering(t *testing.T) {
	product := uuid.New()
	first := domain.QuoteItem{ProductID: &product, RequestedDescription: "Cement", Quantity: dec("1"), UnitPriceSnapshot: decimal.NewNullDecimal(dec("121")), Subtotal: decimal.NewNullDecimal(dec("121"))}
	second := domain.QuoteItem{ProductID: &product, RequestedDescription: "Sand", Quantity: dec("1"), UnitPriceSnapshot: decimal.NewNullDecimal(dec("242")), Subtotal: decimal.NewNullDecimal(dec("242"))}
	snapshot := domain.QuoteRepresentationPayload{Items: []domain.QuoteRepresentationItem{{RequestedDescription: "Sand", Quantity: "1.00", UnitPrice: "242.00", Subtotal: "242.00"}, {RequestedDescription: "Cement", Quantity: "1.00", UnitPrice: "121.00", Subtotal: "121.00"}}}
	draft := invoiceDraft{items: []domain.QuoteItem{first, second}, rates: map[uuid.UUID]domain.VATRate{product: domain.VATRateTwentyOne}, pricesIncludeVAT: true, preview: domain.InvoicePreview{Type: domain.InvoiceTypeA, Amounts: domain.InvoiceAmounts{Net: dec("270"), VAT: dec("56.70"), Total: dec("326.70")}}}
	got, err := fiscalSnapshot(snapshot, draft)
	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].UnitPrice != "200.00" || got.Items[1].UnitPrice != "100.00" || got.Discounts[0].Amount != "30.00" {
		t.Fatalf("fiscal prices = %+v", got)
	}
}
