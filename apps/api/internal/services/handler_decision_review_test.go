package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestDraftSnapshot_CountsOnlyTheOptionsTheOfferShows(t *testing.T) {
	itemID, economyID, untypedID := uuid.New(), uuid.New(), uuid.New()
	quotes := &fakeQuoteRepo{
		items: []domain.QuoteItem{{ID: itemID, Quantity: decimal.NewFromInt(2)}},
		alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {
			{ID: uuid.New(), ProductID: &economyID, ApprovedBySeller: true,
				RecommendationType: alternativeType(domain.ProductAlternativeEconomy)},
			{ID: uuid.New(), ProductID: &untypedID, ApprovedBySeller: true},
		}},
	}
	snapshot, err := draftSnapshot(context.Background(), nil, quotes, &fakeQuoteDiscounts{},
		uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("draftSnapshot() error = %v", err)
	}
	approved := snapshot.Items[0].ApprovedAlternatives
	if len(approved) != 1 || approved[0] != economyID {
		t.Fatalf("approved alternatives = %v, want only the economy option %s", approved, economyID)
	}
}
