package services

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestSelectedAlternatives_AcceptsOnlyOneApprovedProductPerSentLine(t *testing.T) {
	t.Parallel()
	itemID, productID, alternativeID := uuid.New(), uuid.New(), uuid.New()
	items := []domain.QuoteItem{{ID: itemID}}
	options := map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {{
		ID: alternativeID, QuoteItemID: itemID, ProductID: &productID,
		Type: domain.QuoteItemAlternativeTypeProduct, ApprovedBySeller: true,
		PriceSnapshot: decimal.NewNullDecimal(decimal.RequireFromString("900.00")),
	}}}

	selected, err := selectedAlternatives(items, options, []domain.ClientAlternativeSelection{{
		ItemID: itemID, AlternativeID: alternativeID,
	}})
	if err != nil {
		t.Fatalf("selectedAlternatives() = %v", err)
	}
	if got, ok := selected[itemID]; !ok || got.ID != alternativeID {
		t.Fatalf("selected = %+v, want the approved alternative", selected)
	}

	_, err = selectedAlternatives(items, options, []domain.ClientAlternativeSelection{{
		ItemID: itemID, AlternativeID: alternativeID,
	}, {
		ItemID: itemID, AlternativeID: alternativeID,
	}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("duplicated line selection = %v, want ErrInvalidInput", err)
	}

	options[itemID][0].ApprovedBySeller = false
	_, err = selectedAlternatives(items, options, []domain.ClientAlternativeSelection{{
		ItemID: itemID, AlternativeID: alternativeID,
	}})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unapproved selection = %v, want ErrNotFound", err)
	}
}

func TestPreviewSelectionPricing_ReearnsFrozenPromotionsForTheChosenAlternative(t *testing.T) {
	t.Parallel()
	itemID, quotedProductID, alternativeProductID := uuid.New(), uuid.New(), uuid.New()
	promotionID := uuid.New()
	quoted := []domain.QuoteItem{{
		ID: itemID, ProductID: &quotedProductID, Quantity: decimal.NewFromInt(1),
		UnitPriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(100)),
		Subtotal:          decimal.NewNullDecimal(decimal.NewFromInt(100)),
	}}
	selected := map[uuid.UUID]domain.QuoteItemAlternative{itemID: {
		ID: uuid.New(), ProductID: &alternativeProductID,
		PriceSnapshot: decimal.NewNullDecimal(decimal.NewFromInt(50)),
	}}
	snapshot := domain.QuoteSelectionPricingSnapshot{CapturedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
		Promotions: []domain.Promotion{{
			ID: promotionID, Name: "Alternative promotion", IsActive: true,
			ConditionType: domain.PromotionConditionPerItem,
			ActionType:    domain.PromotionActionPercentage,
			ActionValue:   decimal.NewFromInt(10),
			Conditions:    []domain.PromotionCondition{{ProductID: &alternativeProductID}},
		}}}

	items := withSelectedAlternatives(quoted, selected)
	result, err := previewSelectionPricing(uuid.New(), items, nil, nil, snapshot)
	if err != nil {
		t.Fatalf("previewSelectionPricing() = %v", err)
	}
	if items[0].ID != itemID || items[0].ProductID == nil || *items[0].ProductID != alternativeProductID {
		t.Fatalf("selected item = %+v, want the sent line id with the alternative product", items[0])
	}
	if !result.total.Equal(decimal.NewFromInt(45)) {
		t.Fatalf("preview total = %s, want 45.00 after the frozen 10%% promotion", result.total)
	}
	discounts := previewSelectionDiscounts(nil, snapshot, result)
	if len(discounts) != 1 || discounts[0].Description != "Alternative promotion" ||
		discounts[0].Amount != "5.00" {
		t.Fatalf("preview discounts = %+v, want the re-earned frozen promotion", discounts)
	}
}
