package services

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// pricedLine builds a line that matched a product, so a case only has to state its quantity.
func pricedLine(productID uuid.UUID, quantity string) domain.QuoteItem {
	return domain.QuoteItem{
		ID:                   uuid.New(),
		ProductID:            &productID,
		RequestedDescription: "cemento",
		Quantity:             decimal.RequireFromString(quantity),
		MatchStatus:          domain.ItemMatchStatusMatched,
	}
}

// flaggedLine builds a line nothing in the catalog matched: no product, so no price.
func flaggedLine(quantity string) domain.QuoteItem {
	return domain.QuoteItem{
		ID:                   uuid.New(),
		RequestedDescription: "algo que no está en el catálogo",
		Quantity:             decimal.RequireFromString(quantity),
		MatchStatus:          domain.ItemMatchStatusNoMatch,
	}
}

func branchPrice(productID uuid.UUID, price string, minPrice *string) domain.BranchPrice {
	current := domain.BranchPrice{
		ProductID: productID,
		Price:     decimal.RequireFromString(price),
	}
	if minPrice != nil {
		current.MinPrice = decimal.NewNullDecimal(decimal.RequireFromString(*minPrice))
	}
	return current
}

func TestValueQuoteItems_FreezesThePriceAndSumsTheTotal(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	floor := "900.00"
	items := []domain.QuoteItem{pricedLine(product, "10")}
	prices := map[uuid.UUID]domain.BranchPrice{
		product: branchPrice(product, "1200.50", &floor),
	}

	valuation, err := valueQuoteItems(items, prices)
	if err != nil {
		t.Fatalf("valueQuoteItems() = %v, want no error", err)
	}
	if len(valuation.pricings) != 1 || len(valuation.items) != 1 {
		t.Fatalf("valuation covered %d pricings and %d items, want 1 of each",
			len(valuation.pricings), len(valuation.items))
	}

	pricing := valuation.pricings[0]
	if pricing.ItemID != items[0].ID {
		t.Errorf("pricing item = %v, want %v", pricing.ItemID, items[0].ID)
	}
	// 10 × 1200.50, by hand.
	assertAmount(t, "unit price", pricing.UnitPriceSnapshot, "1200.50")
	assertAmount(t, "floor", pricing.MinPriceSnapshot, "900.00")
	assertAmount(t, "subtotal", pricing.Subtotal, "12005.00")
	if !valuation.total.Equal(decimal.RequireFromString("12005.00")) {
		t.Errorf("total = %s, want 12005.00", valuation.total)
	}
	if len(valuation.unpricedProducts) != 0 {
		t.Errorf("unpriced = %v, want none", valuation.unpricedProducts)
	}

	// The line the caller gets back carries what was frozen onto it, so the response and the
	// rows agree without reading them again.
	assertAmount(t, "line unit price", valuation.items[0].UnitPriceSnapshot, "1200.50")
	assertAmount(t, "line subtotal", valuation.items[0].Subtotal, "12005.00")
}

func TestValueQuoteItems_AbsentFloorStaysAbsent(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	items := []domain.QuoteItem{pricedLine(product, "4")}
	prices := map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "250.00", nil)}

	valuation, err := valueQuoteItems(items, prices)
	if err != nil {
		t.Fatalf("valueQuoteItems() = %v, want no error", err)
	}

	pricing := valuation.pricings[0]
	// The whole point: most accounts set no floor, and a floor of zero would let a discount take
	// the line to nothing. Absent has to stay absent.
	if pricing.MinPriceSnapshot.Valid {
		t.Errorf("floor = %s, want null: no floor is not a floor of zero",
			pricing.MinPriceSnapshot.Decimal)
	}
	if valuation.items[0].MinPriceSnapshot.Valid {
		t.Errorf("line floor = %s, want null", valuation.items[0].MinPriceSnapshot.Decimal)
	}
	// The line is priced regardless — an absent floor is not an absent price.
	assertAmount(t, "unit price", pricing.UnitPriceSnapshot, "250.00")
	assertAmount(t, "subtotal", pricing.Subtotal, "1000.00")
}

func TestValueQuoteItems_LineWithNoProductStaysEmptyAndAddsNothing(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	items := []domain.QuoteItem{pricedLine(product, "2"), flaggedLine("7")}
	prices := map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "100.00", nil)}

	valuation, err := valueQuoteItems(items, prices)
	if err != nil {
		t.Fatalf("valueQuoteItems() = %v, want no error", err)
	}
	if len(valuation.items) != 2 {
		t.Fatalf("kept %d lines, want both: a flagged line is never dropped", len(valuation.items))
	}

	flagged := valuation.pricings[1]
	if flagged.UnitPriceSnapshot.Valid || flagged.MinPriceSnapshot.Valid || flagged.Subtotal.Valid {
		t.Errorf("flagged line = %+v, want all three null: valuing it at zero is a false quote",
			flagged)
	}
	// 2 × 100.00 and nothing from the flagged line, by hand.
	if !valuation.total.Equal(decimal.RequireFromString("200.00")) {
		t.Errorf("total = %s, want 200.00: the flagged line contributes nothing", valuation.total)
	}
	if len(valuation.unpricedProducts) != 0 {
		t.Errorf("unpriced = %v, want none: a line with no product is not an unpriced product",
			valuation.unpricedProducts)
	}
}

func TestValueQuoteItems_MatchedProductWithNoPriceInForceStaysEmpty(t *testing.T) {
	t.Parallel()
	priced, unpriced := uuid.New(), uuid.New()
	items := []domain.QuoteItem{pricedLine(priced, "3"), pricedLine(unpriced, "5")}
	prices := map[uuid.UUID]domain.BranchPrice{priced: branchPrice(priced, "80.00", nil)}

	valuation, err := valueQuoteItems(items, prices)
	if err != nil {
		t.Fatalf("valueQuoteItems() = %v, want no error", err)
	}

	gap := valuation.pricings[1]
	if gap.UnitPriceSnapshot.Valid || gap.Subtotal.Valid {
		t.Errorf("unpriced line = %+v, want empty: the branch has no price to freeze", gap)
	}
	// 3 × 80.00, by hand; the unpriced line adds nothing.
	if !valuation.total.Equal(decimal.RequireFromString("240.00")) {
		t.Errorf("total = %s, want 240.00", valuation.total)
	}
	// Reported rather than swallowed: the quote reaches QUOTED with a gap in it.
	if len(valuation.unpricedProducts) != 1 || valuation.unpricedProducts[0] != unpriced {
		t.Errorf("unpriced = %v, want [%v]", valuation.unpricedProducts, unpriced)
	}
}

func TestValueQuoteItems_RoundsToTheMoneyScale(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		quantity  string
		unitPrice string
		want      string // computed by hand.
	}{
		{"exact", "3", "1000.00", "3000.00"},
		{"four decimals truncate to two", "3.33", "10.01", "33.33"},
		{"half rounds away from zero", "1.5", "3.33", "5.00"},
		{"a zero quantity is priced at zero, not left empty", "0", "1200.00", "0.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			product := uuid.New()
			valuation, err := valueQuoteItems(
				[]domain.QuoteItem{pricedLine(product, tc.quantity)},
				map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, tc.unitPrice, nil)})
			if err != nil {
				t.Fatalf("valueQuoteItems() = %v, want no error", err)
			}
			assertAmount(t, "subtotal", valuation.pricings[0].Subtotal, tc.want)
			if !valuation.total.Equal(decimal.RequireFromString(tc.want)) {
				t.Errorf("total = %s, want %s", valuation.total, tc.want)
			}
			if valuation.total.Exponent() < -domain.MoneyScale {
				t.Errorf("total = %s, want at most %d decimals: the column holds no more",
					valuation.total, domain.MoneyScale)
			}
		})
	}
}

func TestValueQuoteItems_RefusesATotalTheColumnCannotHold(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	// Two lines each inside NUMERIC(14,2) whose sum is not: the overflow is the total's, so
	// checking the lines alone would let the database raise it as an unreadable 500.
	items := []domain.QuoteItem{pricedLine(product, "100"), pricedLine(product, "100")}
	prices := map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "5000000000.00", nil)}

	if _, err := valueQuoteItems(items, prices); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("valueQuoteItems() = %v, want ErrInvalidInput", err)
	}
}

func TestQuoteItemProductIDs_SkipsFlaggedLinesAndRepeats(t *testing.T) {
	t.Parallel()
	first, second := uuid.New(), uuid.New()
	items := []domain.QuoteItem{
		pricedLine(first, "1"),
		flaggedLine("2"),
		pricedLine(second, "3"),
		pricedLine(first, "4"),
	}

	got := quoteItemProductIDs(items)
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("quoteItemProductIDs() = %v, want [%v %v] once each", got, first, second)
	}
}

func assertAmount(t *testing.T, what string, got decimal.NullDecimal, want string) {
	t.Helper()
	if !got.Valid {
		t.Errorf("%s = null, want %s", what, want)
		return
	}
	if !got.Decimal.Equal(decimal.RequireFromString(want)) {
		t.Errorf("%s = %s, want %s", what, got.Decimal, want)
	}
	if got.Decimal.StringFixed(domain.MoneyScale) != want {
		t.Errorf("%s renders as %s, want %s", what, got.Decimal.StringFixed(domain.MoneyScale), want)
	}
}
