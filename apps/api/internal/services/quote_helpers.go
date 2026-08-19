package services

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// quoteValuation is one version priced, before any of it is written: the valuation to freeze on
// each line, the lines carrying it, and the total they add up to.
type quoteValuation struct {
	pricings []domain.QuoteItemPricing
	items    []domain.QuoteItem
	total    decimal.Decimal
	// unpricedProducts carries one product per line that matched a product the branch has no
	// price in force for. Those lines stay empty, so the gap has to be reported rather than
	// left for the seller to notice.
	unpricedProducts []uuid.UUID
}

// valueQuoteItems freezes each line's unit price and floor from the branch's prices in force and
// sums the version total. A line with no product, or whose product the branch has not priced,
// keeps all three values empty: it stays in the quote and contributes nothing, because dropping
// it and valuing it at zero are both ways of misreporting what the client asked for.
func valueQuoteItems(
	items []domain.QuoteItem, prices map[uuid.UUID]domain.BranchPrice,
) (quoteValuation, error) {
	valuation := quoteValuation{
		pricings: make([]domain.QuoteItemPricing, 0, len(items)),
		items:    make([]domain.QuoteItem, 0, len(items)),
	}
	subtotals := decimal.Zero

	for i, item := range items {
		pricing := domain.QuoteItemPricing{ItemID: item.ID}
		if price, ok := priceFor(item, prices); ok {
			subtotal := item.Quantity.Mul(price.Price).Round(domain.MoneyScale)
			if err := validateAmount(subtotal, fmt.Sprintf("items[%d].subtotal", i)); err != nil {
				return quoteValuation{}, err
			}
			pricing.UnitPriceSnapshot = decimal.NewNullDecimal(price.Price)
			// Taken straight across so an absent floor stays absent: read as zero, a later
			// discount could drive the line to nothing.
			pricing.MinPriceSnapshot = price.MinPrice
			pricing.Subtotal = decimal.NewNullDecimal(subtotal)
			subtotals = subtotals.Add(subtotal)
		} else if item.ProductID != nil {
			valuation.unpricedProducts = append(valuation.unpricedProducts, *item.ProductID)
		}

		item.UnitPriceSnapshot = pricing.UnitPriceSnapshot
		item.MinPriceSnapshot = pricing.MinPriceSnapshot
		item.Subtotal = pricing.Subtotal
		valuation.pricings = append(valuation.pricings, pricing)
		valuation.items = append(valuation.items, item)
	}

	// quote_version.total is the sum of the line subtotals less the sum of the version's
	// discounts. The promotion sweep that produces the second term is US-38, so it is empty
	// here — part of the formula rather than missing from it.
	discounts := decimal.Zero
	valuation.total = subtotals.Sub(discounts).Round(domain.MoneyScale)
	if err := validateAmount(valuation.total, "total"); err != nil {
		return quoteValuation{}, err
	}
	return valuation, nil
}

// priceFor returns the price in force for a line's product. The second result is false for a line
// with no product and for one whose product the branch has no open price period for.
func priceFor(
	item domain.QuoteItem, prices map[uuid.UUID]domain.BranchPrice,
) (domain.BranchPrice, bool) {
	if item.ProductID == nil {
		return domain.BranchPrice{}, false
	}
	price, ok := prices[*item.ProductID]
	return price, ok
}

// quoteItemProductIDs collects the products a version's lines matched, skipping the flagged lines
// that matched none, so the prices are read in one query instead of one per line.
func quoteItemProductIDs(items []domain.QuoteItem) []uuid.UUID {
	productIDs := make([]uuid.UUID, 0, len(items))
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, item := range items {
		if item.ProductID == nil {
			continue
		}
		if _, duplicate := seen[*item.ProductID]; duplicate {
			continue
		}
		seen[*item.ProductID] = struct{}{}
		productIDs = append(productIDs, *item.ProductID)
	}
	return productIDs
}
