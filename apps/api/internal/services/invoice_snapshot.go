package services

import (
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
	"strings"
)

// fiscalSnapshot converts commercial line prices into the document's fiscal presentation.
func fiscalSnapshot(snapshot domain.QuoteRepresentationPayload, draft invoiceDraft) (domain.QuoteRepresentationPayload, error) {
	rates := make(map[string]domain.VATRate)
	for _, item := range draft.items {
		rate := domain.DefaultVATRate
		if item.ProductID != nil {
			if value, ok := draft.rates[*item.ProductID]; ok {
				rate = value
			}
		}
		key := invoiceLineKey(item.RequestedDescription, item.Quantity.StringFixed(2), item.UnitPriceSnapshot.Decimal.StringFixed(2), item.Subtotal.Decimal.StringFixed(2))
		if previous, ok := rates[key]; ok && previous != rate {
			return snapshot, domain.ErrInvalidInput
		}
		rates[key] = rate
	}
	subtotal := decimal.Zero
	for index := range snapshot.Items {
		item := &snapshot.Items[index]
		price, err := decimal.NewFromString(item.UnitPrice)
		if err != nil {
			return snapshot, err
		}
		amount, err := decimal.NewFromString(item.Subtotal)
		if err != nil {
			return snapshot, err
		}
		rate, ok := rates[invoiceLineKey(item.RequestedDescription, item.Quantity, item.UnitPrice, item.Subtotal)]
		if !ok {
			return snapshot, domain.ErrInvalidInput
		}
		factor := decimal.NewFromInt(1).Add(rate.Percent().Div(decimal.NewFromInt(100)))
		if draft.preview.Type == domain.InvoiceTypeA && draft.pricesIncludeVAT {
			price = price.Div(factor)
			amount = amount.Div(factor)
		}
		if draft.preview.Type == domain.InvoiceTypeB && !draft.pricesIncludeVAT {
			price = price.Mul(factor)
			amount = amount.Mul(factor)
		}
		item.UnitPrice = price.Round(domain.MoneyScale).StringFixed(2)
		item.Subtotal = amount.Round(domain.MoneyScale).StringFixed(2)
		item.Alternatives = nil
		subtotal = subtotal.Add(amount.Round(domain.MoneyScale))
	}
	target := draft.preview.Amounts.Total
	if draft.preview.Type == domain.InvoiceTypeA {
		target = draft.preview.Amounts.Net.Add(draft.preview.Amounts.Exempt)
	}
	snapshot.Discounts = nil
	discount := subtotal.Sub(target)
	if discount.IsNegative() && len(snapshot.Items) > 0 {
		last := &snapshot.Items[len(snapshot.Items)-1]
		value, err := decimal.NewFromString(last.Subtotal)
		if err != nil {
			return snapshot, err
		}
		last.Subtotal = value.Sub(discount).StringFixed(2)
	}
	if discount.IsPositive() {
		snapshot.Discounts = []domain.QuoteRepresentationDiscount{{Description: "Bonificaciones", Amount: discount.StringFixed(2)}}
	}
	snapshot.Total = draft.preview.Amounts.Total.StringFixed(2)
	return snapshot, nil
}

func invoiceLineKey(description, quantity, price, subtotal string) string {
	return strings.Join([]string{description, quantity, price, subtotal}, "\x00")
}
