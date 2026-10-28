package services

import (
	"errors"
	"testing"

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
