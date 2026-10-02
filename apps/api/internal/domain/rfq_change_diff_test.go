package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func nullPrice(value string) decimal.NullDecimal {
	return decimal.NullDecimal{Valid: true, Decimal: decimal.RequireFromString(value)}
}

func diffItem(description, quantity string, unit *string, price decimal.NullDecimal) QuoteItem {
	return QuoteItem{
		RequestedDescription: description,
		Quantity:             decimal.RequireFromString(quantity),
		Unit:                 unit,
		UnitPriceSnapshot:    price,
	}
}

func TestBuildChangeRequestDiff_IdenticalSnapshotsStayNeutral(t *testing.T) {
	unit := "bolsa"
	frozen := ChangeRequestSnapshot{
		Items: []QuoteItem{
			diffItem("cemento", "10", &unit, nullPrice("8500.00")),
			diffItem("membrana", "2", nil, decimal.NullDecimal{}),
		},
		Discounts: []QuoteDiscount{{PromotionName: strPtr("Descuento obra"), Amount: decimal.NewFromFloat(500)}},
		Total:     decimal.NewFromFloat(16000),
	}

	diff := BuildChangeRequestDiff(nil, frozen, frozen)

	if len(diff.Original.Items) != 2 || len(diff.Requested.Items) != 2 {
		t.Fatalf("items = original %d requested %d, want 2 and 2",
			len(diff.Original.Items), len(diff.Requested.Items))
	}
	for i := range diff.Requested.Items {
		if diff.Requested.Items[i].Changed {
			t.Errorf("requested item %d marked changed on identical snapshots", i)
		}
		if diff.Requested.Items[i].ChangeType != "" {
			t.Errorf("requested item %d change type = %q, want empty", i, diff.Requested.Items[i].ChangeType)
		}
	}
	if diff.Requested.Discounts[0].Changed {
		t.Error("discount marked changed on identical snapshots")
	}
	if !diff.Original.Total.Equal(decimal.NewFromFloat(16000)) {
		t.Errorf("original total = %v, want 16000", diff.Original.Total)
	}
	if !diff.Requested.Total.Equal(decimal.NewFromFloat(16000)) {
		t.Errorf("requested total = %v, want 16000", diff.Requested.Total)
	}
}

func TestBuildChangeRequestDiff_FlagsModifiedAddedAndRemovedLines(t *testing.T) {
	unit := "bolsa"
	frozen := ChangeRequestSnapshot{
		Items: []QuoteItem{
			diffItem("cemento", "10", &unit, nullPrice("8500.00")),
			diffItem("membrana", "2", nil, decimal.NullDecimal{}),
		},
		Discounts: []QuoteDiscount{{PromotionName: strPtr("Descuento obra"), Amount: decimal.NewFromFloat(500)}},
		Total:     decimal.NewFromFloat(16000),
	}
	draft := ChangeRequestSnapshot{
		Items: []QuoteItem{
			diffItem("cemento", "12", &unit, nullPrice("8500.00")),
			diffItem("membrana", "2", nil, decimal.NullDecimal{}),
			diffItem("cal", "4", nil, decimal.NullDecimal{}),
		},
		Total: decimal.NewFromFloat(17000),
	}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	if len(diff.Requested.Items) != 3 {
		t.Fatalf("requested items = %d, want 3", len(diff.Requested.Items))
	}
	if !diff.Requested.Items[0].Changed || diff.Requested.Items[0].ChangeType != "modified" {
		t.Errorf("item 0 = changed %v type %q, want true modified",
			diff.Requested.Items[0].Changed, diff.Requested.Items[0].ChangeType)
	}
	if diff.Requested.Items[1].Changed {
		t.Error("item 1 (unchanged) marked changed")
	}
	if !diff.Requested.Items[2].Changed || diff.Requested.Items[2].ChangeType != "added" {
		t.Errorf("item 2 = changed %v type %q, want true added",
			diff.Requested.Items[2].Changed, diff.Requested.Items[2].ChangeType)
	}
}

func TestBuildChangeRequestDiff_FlagsDroppedLinesAsRemovedPlaceholders(t *testing.T) {
	frozen := ChangeRequestSnapshot{
		Items: []QuoteItem{
			diffItem("cemento", "10", nil, decimal.NullDecimal{}),
			diffItem("membrana", "2", nil, decimal.NullDecimal{}),
		},
		Total: decimal.NewFromFloat(100),
	}
	draft := ChangeRequestSnapshot{
		Items: []QuoteItem{diffItem("cemento", "10", nil, decimal.NullDecimal{})},
		Total: decimal.NewFromFloat(90),
	}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	if len(diff.Requested.Items) != 2 {
		t.Fatalf("requested items = %d, want 2 (kept + removed placeholder)", len(diff.Requested.Items))
	}
	if diff.Requested.Items[0].Changed {
		t.Error("kept line marked changed")
	}
	removed := diff.Requested.Items[1]
	if !removed.Changed || removed.ChangeType != "removed" {
		t.Errorf("removed placeholder = changed %v type %q, want true removed",
			removed.Changed, removed.ChangeType)
	}
	if removed.Description != "membrana" {
		t.Errorf("removed placeholder description = %q, want membrana", removed.Description)
	}
}

func TestBuildChangeRequestDiff_FlagsDiscountAmountAndMayAddOne(t *testing.T) {
	frozen := ChangeRequestSnapshot{
		Discounts: []QuoteDiscount{
			{PromotionName: strPtr("Descuento obra"), Amount: decimal.NewFromFloat(500)},
		},
		Total: decimal.NewFromFloat(16000),
	}
	draft := ChangeRequestSnapshot{
		Discounts: []QuoteDiscount{
			{PromotionName: strPtr("Descuento obra"), Amount: decimal.NewFromFloat(800)},
			{PromotionName: strPtr("Promo cal"), Amount: decimal.NewFromFloat(200)},
		},
		Total: decimal.NewFromFloat(15500),
	}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	if len(diff.Requested.Discounts) != 2 {
		t.Fatalf("requested discounts = %d, want 2", len(diff.Requested.Discounts))
	}
	if !diff.Requested.Discounts[0].Changed {
		t.Error("discount with changed amount not flagged")
	}
	if !diff.Requested.Discounts[1].Changed {
		t.Error("newly added discount not flagged")
	}
	if diff.Original.Discounts[0].Amount != "500.00" {
		t.Errorf("original discount amount = %q, want 500.00", diff.Original.Discounts[0].Amount)
	}
}

func TestBuildChangeRequestDiff_CarriesTheReason(t *testing.T) {
	reason := "necesito entrega el viernes"
	diff := BuildChangeRequestDiff(&reason, ChangeRequestSnapshot{}, ChangeRequestSnapshot{})

	if diff.Reason == nil || *diff.Reason != reason {
		t.Errorf("reason = %v, want %q", diff.Reason, reason)
	}
}
