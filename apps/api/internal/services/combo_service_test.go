package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func comboInput(items ...domain.ComboItemInput) domain.ComboInput {
	return domain.ComboInput{Name: " Obra gruesa ", IsActive: true, Items: items, BranchIDs: []uuid.UUID{uuid.New()}}
}

func comboItem(quantity string) domain.ComboItemInput {
	return domain.ComboItemInput{ProductID: uuid.New(), Quantity: decimal.RequireFromString(quantity)}
}

func TestNormalizeCombo(t *testing.T) {
	t.Parallel()
	repeated := comboItem("1")
	cases := []struct {
		name  string
		in    domain.ComboInput
		valid bool
	}{
		{"two products", comboInput(comboItem("10"), comboItem("1.5")), true},
		{"one product", comboInput(comboItem("10")), false},
		{"repeated product", comboInput(repeated, repeated), false},
		{"zero quantity", comboInput(comboItem("0"), comboItem("1")), false},
		{"three decimals", comboInput(comboItem("1.125"), comboItem("1")), false},
		{"blank name", domain.ComboInput{Name: "  ", Items: []domain.ComboItemInput{comboItem("1"), comboItem("1")}, BranchIDs: []uuid.UUID{uuid.New()}}, false},
		{"no branch", domain.ComboInput{Name: "x", Items: []domain.ComboItemInput{comboItem("1"), comboItem("1")}}, false},
		{"long name", domain.ComboInput{Name: strings.Repeat("a", 256), Items: []domain.ComboItemInput{comboItem("1"), comboItem("1")}, BranchIDs: []uuid.UUID{uuid.New()}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := normalizeCombo(tc.in)
			if tc.valid && (err != nil || out.Name != "Obra gruesa") {
				t.Fatalf("normalizeCombo() = %+v, %v; want a trimmed valid combo", out, err)
			}
			if !tc.valid && !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("normalizeCombo() err = %v, want invalid input", err)
			}
		})
	}
}

func TestComboSuggestions(t *testing.T) {
	t.Parallel()
	cement, sand, lime, gravel := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	item := func(id uuid.UUID, active bool) domain.ComboItem {
		return domain.ComboItem{ProductID: id, Quantity: decimal.NewFromInt(1), ProductName: "x", ProductIsActive: active}
	}
	combo := func(items ...domain.ComboItem) domain.Combo {
		return domain.Combo{ID: uuid.New(), Name: "Combo", Items: items}
	}
	priced := map[uuid.UUID]domain.BranchPrice{
		cement: {Price: decimal.NewFromInt(100)}, sand: {Price: decimal.NewFromInt(50)},
		lime: {Price: decimal.NewFromInt(30)},
	}
	quote := []domain.QuoteItem{{ProductID: &cement}, {ProductID: nil}}

	partial := combo(item(cement, true), item(sand, true), item(lime, true))
	complete := combo(item(cement, true))
	unrelated := combo(item(sand, true), item(lime, true))
	unpriced := combo(item(cement, true), item(gravel, true))
	inactive := combo(item(cement, true), item(sand, false))

	got := comboSuggestions([]domain.Combo{partial, complete, unrelated, unpriced, inactive}, quote, priced)
	if len(got) != 1 || got[0].ComboID != partial.ID {
		t.Fatalf("suggestions = %+v; want only the partially held, sellable combo", got)
	}
	if len(got[0].Present) != 1 || got[0].Present[0].ProductID != cement ||
		len(got[0].Missing) != 2 || got[0].Missing[0].ProductID != sand || got[0].Missing[1].ProductID != lime {
		t.Fatalf("present/missing = %+v / %+v", got[0].Present, got[0].Missing)
	}
}
