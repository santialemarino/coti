package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func nullPrice(value string) decimal.NullDecimal {
	return decimal.NullDecimal{Valid: true, Decimal: decimal.RequireFromString(value)}
}

func diffItem(description, quantity string, unit *string, price decimal.NullDecimal) QuoteItem {
	return QuoteItem{
		ID:                   uuid.New(),
		RequestedDescription: description,
		Quantity:             decimal.RequireFromString(quantity),
		Unit:                 unit,
		UnitPriceSnapshot:    price,
	}
}

// copied is the draft line cloned from sent, which is what a fresh draft holds.
func copied(sent QuoteItem) QuoteItem {
	line := sent
	line.ID = uuid.New()
	source := sent.ID
	line.SourceItemID = &source
	return line
}

func changes(diff ChangeRequestDiff) []DiffChange {
	result := make([]DiffChange, 0, len(diff.Lines))
	for _, line := range diff.Lines {
		result = append(result, line.Change)
	}
	return result
}

func TestBuildChangeRequestDiff_IdenticalSnapshotsStayNeutral(t *testing.T) {
	unit := "bolsa"
	cement := diffItem("cemento", "10", &unit, nullPrice("8500.00"))
	membrane := diffItem("membrana", "2", nil, decimal.NullDecimal{})
	discount := QuoteDiscount{PromotionName: strPtr("Descuento obra"), Amount: decimal.NewFromInt(500)}
	frozen := ChangeRequestSnapshot{Items: []QuoteItem{cement, membrane},
		Discounts: []QuoteDiscount{discount}, Total: decimal.NewFromInt(16000)}
	draft := ChangeRequestSnapshot{Items: []QuoteItem{copied(cement), copied(membrane)},
		Discounts: []QuoteDiscount{discount}, Total: decimal.NewFromInt(16000)}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	if got := changes(diff); len(got) != 2 || got[0] != DiffUnchanged || got[1] != DiffUnchanged {
		t.Errorf("line changes = %v, want two UNCHANGED", got)
	}
	if len(diff.Discounts) != 1 || diff.Discounts[0].Change != DiffUnchanged {
		t.Errorf("discounts = %+v, want one UNCHANGED", diff.Discounts)
	}
	if !diff.OriginalTotal.Equal(decimal.NewFromInt(16000)) ||
		!diff.RequestedTotal.Equal(decimal.NewFromInt(16000)) {
		t.Errorf("totals = %s → %s, want 16000 both", diff.OriginalTotal, diff.RequestedTotal)
	}
}

/*
 * A swapped line comes back from the database out of order. Each sent line is still compared with
 * the line copied from it, so the swap reads as one modification and the untouched line as none.
 */
func TestBuildChangeRequestDiff_PairsLinesBySourceNotPosition(t *testing.T) {
	cement := diffItem("Cemento Loma Negra 50 kg", "20", nil, nullPrice("9500.00"))
	sand := diffItem("Arena gruesa", "3", nil, nullPrice("32000.00"))
	swapped := copied(cement)
	swapped.RequestedDescription = "Cemento Pampa 50 kg"
	swapped.UnitPriceSnapshot = nullPrice("8400.00")
	frozen := ChangeRequestSnapshot{Items: []QuoteItem{cement, sand}}
	draft := ChangeRequestSnapshot{Items: []QuoteItem{copied(sand), swapped}}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	if len(diff.Lines) != 2 {
		t.Fatalf("lines = %+v, want 2", diff.Lines)
	}
	first, second := diff.Lines[0], diff.Lines[1]
	if first.Change != DiffModified || first.Original.Description != "Cemento Loma Negra 50 kg" ||
		first.Requested.Description != "Cemento Pampa 50 kg" || *first.Requested.UnitPrice != "8400.00" {
		t.Errorf("first = %+v, want the cement paired with its swap", first)
	}
	if second.Change != DiffUnchanged || second.Original.Description != "Arena gruesa" ||
		second.Requested.Description != "Arena gruesa" {
		t.Errorf("second = %+v, want the sand unchanged", second)
	}
}

func TestBuildChangeRequestDiff_FlagsModifiedAddedAndRemovedLines(t *testing.T) {
	unit := "bolsa"
	cement := diffItem("cemento", "10", &unit, nullPrice("8500.00"))
	sand := diffItem("arena", "3", nil, nullPrice("32000.00"))
	requantified := copied(cement)
	requantified.Quantity = decimal.NewFromInt(12)
	lime := diffItem("cal", "5", nil, decimal.NullDecimal{})
	frozen := ChangeRequestSnapshot{Items: []QuoteItem{cement, sand}}
	draft := ChangeRequestSnapshot{Items: []QuoteItem{lime, requantified}}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	want := []DiffChange{DiffModified, DiffRemoved, DiffAdded}
	got := changes(diff)
	if len(got) != len(want) {
		t.Fatalf("line changes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line changes = %v, want %v", got, want)
		}
	}
	if removed := diff.Lines[1]; removed.Original.Description != "arena" || removed.Requested != nil {
		t.Errorf("removed = %+v, want the sand with no draft side", removed)
	}
	if added := diff.Lines[2]; added.Original != nil || added.Requested.Description != "cal" {
		t.Errorf("added = %+v, want the lime with no sent side", added)
	}
}

// A product swap that keeps the customer's wording is still a change.
func TestBuildChangeRequestDiff_AProductSwapIsAChange(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	sent := diffItem("cemento", "10", nil, nullPrice("9500.00"))
	sent.ProductID = &first
	swapped := copied(sent)
	swapped.ProductID = &second

	diff := BuildChangeRequestDiff(nil, ChangeRequestSnapshot{Items: []QuoteItem{sent}},
		ChangeRequestSnapshot{Items: []QuoteItem{swapped}})

	if got := changes(diff); len(got) != 1 || got[0] != DiffModified {
		t.Errorf("line changes = %v, want MODIFIED", got)
	}
}

func TestBuildChangeRequestDiff_PairsDiscountsByName(t *testing.T) {
	named := func(name, amount string, suppressed bool) QuoteDiscount {
		return QuoteDiscount{PromotionName: strPtr(name), Amount: decimal.RequireFromString(amount),
			SuppressedBySeller: suppressed}
	}
	frozen := ChangeRequestSnapshot{Discounts: []QuoteDiscount{
		named("Cemento 10%", "19000", false), named("Compra grande", "80700", false),
		named("Cliente frecuente", "1000", false)}}
	draft := ChangeRequestSnapshot{Discounts: []QuoteDiscount{
		named("Compra grande", "80700", true), named("Cemento 10%", "16800", false),
		named("Hierro 8%", "2000", false)}}

	diff := BuildChangeRequestDiff(nil, frozen, draft)

	want := map[string]DiffChange{"Compra grande": DiffModified, "Cemento 10%": DiffModified,
		"Hierro 8%": DiffAdded, "Cliente frecuente": DiffRemoved}
	if len(diff.Discounts) != len(want) {
		t.Fatalf("discounts = %+v, want %d", diff.Discounts, len(want))
	}
	for _, discount := range diff.Discounts {
		if discount.Change != want[discount.Name] {
			t.Errorf("%s = %s, want %s", discount.Name, discount.Change, want[discount.Name])
		}
	}
	if suppressed := diff.Discounts[0]; !suppressed.Requested.Suppressed || suppressed.Original.Suppressed {
		t.Errorf("Compra grande = %+v, want suppressed only on the draft", suppressed)
	}
}
