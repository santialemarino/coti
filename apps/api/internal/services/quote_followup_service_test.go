package services

import (
	"slices"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestFollowupItems_WordsMatchedAndUnmatchedLines(t *testing.T) {
	name, unit := "Cemento Loma Negra 50 kg", "bolsa"
	items := []domain.QuoteItem{
		{RequestedDescription: "10 bolsas de cemento", Quantity: decimal.RequireFromString("10.00"),
			ProductName: &name, ProductUnit: &unit},
		{RequestedDescription: "un poco de arena fina", Quantity: decimal.RequireFromString("1.00")},
	}

	got := followupItems(items)
	want := []string{"10 bolsa Cemento Loma Negra 50 kg", "un poco de arena fina"}
	if !slices.Equal(got, want) {
		t.Errorf("followupItems() = %q, want %q", got, want)
	}
}
