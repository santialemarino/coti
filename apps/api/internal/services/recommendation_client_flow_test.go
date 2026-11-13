package services

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestSelectedAlternatives_RejectsChoicesOutsideTheSentItem(t *testing.T) {
	t.Parallel()
	itemID, otherItemID, productID := uuid.New(), uuid.New(), uuid.New()
	option := domain.QuoteItemAlternative{
		ID: uuid.New(), QuoteItemID: itemID, ProductID: &productID,
		Type: domain.QuoteItemAlternativeTypeProduct, ApprovedBySeller: true,
		RecommendationType: alternativeType(domain.ProductAlternativeEconomy),
		PriceSnapshot:      decimal.NewNullDecimal(decimal.NewFromInt(50)),
	}
	items := []domain.QuoteItem{{ID: itemID}, {ID: otherItemID}}
	cases := []struct {
		name      string
		selection domain.ClientAlternativeSelection
		option    domain.QuoteItemAlternative
		want      error
	}{
		{"another item", domain.ClientAlternativeSelection{ItemID: otherItemID, AlternativeID: option.ID}, option, domain.ErrNotFound},
		{"unknown item", domain.ClientAlternativeSelection{ItemID: uuid.New(), AlternativeID: option.ID}, option, domain.ErrNotFound},
		{"missing item", domain.ClientAlternativeSelection{AlternativeID: option.ID}, option, domain.ErrInvalidInput},
		{"missing alternative", domain.ClientAlternativeSelection{ItemID: itemID}, option, domain.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := selectedAlternatives(items, map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {tc.option}},
				[]domain.ClientAlternativeSelection{tc.selection})
			if !errors.Is(err, tc.want) {
				t.Fatalf("selectedAlternatives() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSelectedAlternatives_RejectsUnusableOptions(t *testing.T) {
	t.Parallel()
	itemID, productID := uuid.New(), uuid.New()
	valid := domain.QuoteItemAlternative{
		ID: uuid.New(), QuoteItemID: itemID, ProductID: &productID,
		Type: domain.QuoteItemAlternativeTypeProduct, ApprovedBySeller: true,
		RecommendationType: alternativeType(domain.ProductAlternativeEconomy),
		PriceSnapshot:      decimal.NewNullDecimal(decimal.NewFromInt(50)),
	}
	cases := []struct {
		name   string
		change func(*domain.QuoteItemAlternative)
		want   error
	}{
		{"not approved", func(option *domain.QuoteItemAlternative) { option.ApprovedBySeller = false }, domain.ErrNotFound},
		{"missing product", func(option *domain.QuoteItemAlternative) { option.ProductID = nil }, domain.ErrNotFound},
		{"missing price", func(option *domain.QuoteItemAlternative) { option.PriceSnapshot = decimal.NullDecimal{} }, domain.ErrNotFound},
		{"combo", func(option *domain.QuoteItemAlternative) { option.Type = domain.QuoteItemAlternativeTypeCombo }, domain.ErrNotFound},
		{"negative price", func(option *domain.QuoteItemAlternative) {
			option.PriceSnapshot = decimal.NewNullDecimal(decimal.NewFromInt(-1))
		}, domain.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			option := valid
			tc.change(&option)
			_, err := selectedAlternatives([]domain.QuoteItem{{ID: itemID}},
				map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {option}},
				[]domain.ClientAlternativeSelection{{ItemID: itemID, AlternativeID: option.ID}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("selectedAlternatives() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPreviewSelectionPricing_ReplacesOnlyChosenLineAndReevaluatesPromotions(t *testing.T) {
	t.Parallel()
	itemID, untouchedID := uuid.New(), uuid.New()
	baseID, alternativeID, otherID := uuid.New(), uuid.New(), uuid.New()
	basePromotionID, alternativePromotionID := uuid.New(), uuid.New()
	items := []domain.QuoteItem{
		{ID: itemID, ProductID: &baseID, Quantity: decimal.NewFromInt(2),
			UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(100)),
			Subtotal:          decimal.NewNullDecimal(decimal.NewFromInt(200))},
		{ID: untouchedID, ProductID: &otherID, Quantity: decimal.NewFromInt(3),
			UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(20)),
			Subtotal:          decimal.NewNullDecimal(decimal.NewFromInt(60))},
	}
	snapshot := domain.QuoteSelectionPricingSnapshot{
		CapturedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
		Promotions: []domain.Promotion{
			{ID: basePromotionID, Name: "Base promotion", IsActive: true,
				ConditionType: domain.PromotionConditionPerItem, ActionType: domain.PromotionActionPercentage,
				ActionValue: decimal.NewFromInt(10), Conditions: []domain.PromotionCondition{{ProductID: &baseID}}},
			{ID: alternativePromotionID, Name: "Alternative promotion", IsActive: true,
				ConditionType: domain.PromotionConditionPerItem, ActionType: domain.PromotionActionPercentage,
				ActionValue: decimal.NewFromInt(20), Conditions: []domain.PromotionCondition{{ProductID: &alternativeID}}},
		},
	}
	selected := withSelectedAlternatives(items, map[uuid.UUID]domain.QuoteItemAlternative{itemID: {
		ProductID: &alternativeID, PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(50)),
	}})
	result, err := previewSelectionPricing(uuid.New(), selected, nil, nil, snapshot)
	if err != nil {
		t.Fatalf("previewSelectionPricing() = %v", err)
	}
	if !selected[0].Subtotal.Decimal.Equal(decimal.NewFromInt(100)) ||
		selected[1].ProductID == nil || *selected[1].ProductID != otherID ||
		!selected[1].Subtotal.Decimal.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("selected items = %+v, want 100.00 for the replacement and 60.00 unchanged", selected)
	}
	if *items[0].ProductID != baseID || !items[0].Subtotal.Decimal.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("sent items changed: %+v", items)
	}
	if !result.total.Equal(decimal.NewFromInt(140)) {
		t.Fatalf("total = %s, want 140.00 after the alternative's 20.00 discount", result.total)
	}
	discounts := previewSelectionDiscounts(nil, snapshot, result)
	if len(discounts) != 1 || discounts[0].Description != "Alternative promotion" || discounts[0].Amount != "20.00" {
		t.Fatalf("discounts = %+v, want only the alternative promotion", discounts)
	}
}

// An offer sent before options had to be economy or premium may hold an approved untyped one. The
// customer can only choose what their offer showed, so accepting it keeps working.
func TestSelectedAlternatives_AcceptsAnOptionAnEarlierOfferShowed(t *testing.T) {
	t.Parallel()
	itemID, productID := uuid.New(), uuid.New()
	legacy := domain.QuoteItemAlternative{
		ID: uuid.New(), QuoteItemID: itemID, ProductID: &productID,
		Type: domain.QuoteItemAlternativeTypeProduct, ApprovedBySeller: true,
		PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(50)),
	}
	selected, err := selectedAlternatives([]domain.QuoteItem{{ID: itemID}},
		map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {legacy}},
		[]domain.ClientAlternativeSelection{{ItemID: itemID, AlternativeID: legacy.ID}})
	if err != nil || selected[itemID].ID != legacy.ID {
		t.Fatalf("selectedAlternatives() = %+v, %v; want the legacy option accepted", selected, err)
	}
}
