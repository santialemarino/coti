package services

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestValidateChangeRequestProposal(t *testing.T) {
	lineID, otherLine, stranger := uuid.New(), uuid.New(), uuid.New()
	promotionID := uuid.New()
	quantity := decimal.NewFromInt(4)
	lower, improve := domain.ChangeRequestIntentLowerTotalPrice, domain.ChangeRequestIntentImproveQuality
	unknownIntent := domain.ChangeRequestIntent("SHIP_FASTER")
	adapt, quality := domain.HandlerStrategyAdaptToReachDiscount, domain.HandlerStrategyImproveQuality
	description := "cal"
	source := []domain.QuoteItem{{ID: lineID}, {ID: otherLine}}
	promotions := []domain.Promotion{{ID: promotionID, Name: "Cemento 10%"}}
	addDiscount := func(id uuid.UUID) domain.ChangeRequestOperation {
		return domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationAddDiscount, PromotionID: &id}
	}
	escape := domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationRequiresSeller}
	requantify := domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationUpdateQuantity,
		TargetItemID: &lineID, Quantity: &quantity}

	cases := []struct {
		name     string
		proposal domain.ChangeRequestProposal
		valid    bool
	}{
		{"atomic change", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{requantify}}, true},
		{"the same line twice", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{
				{Type: domain.ChangeRequestOperationRemoveItem, TargetItemID: &lineID}, requantify}}, false},
		{"no level", domain.ChangeRequestProposal{
			Operations: []domain.ChangeRequestOperation{requantify}}, false},
		{"atomic with no operation", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic}, false},
		{"atomic carrying an intent", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Intent: &lower, Operations: []domain.ChangeRequestOperation{requantify}}, false},
		{"no-match escape", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelNoMatch,
			Operations: []domain.ChangeRequestOperation{escape}}, true},
		{"escape under an atomic level", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{escape}}, false},
		{"no-match with a change", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelNoMatch,
			Operations: []domain.ChangeRequestOperation{requantify}}, false},
		{"discount in force", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{addDiscount(promotionID), requantify}}, true},
		{"discount not in force", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{addDiscount(uuid.New())}}, false},
		{"the same discount twice", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{addDiscount(promotionID), addDiscount(promotionID)}}, false},
		{"discount carrying a line", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationAddDiscount,
				PromotionID: &promotionID, TargetItemID: &lineID}}}, false},
		{"a line change carrying a promotion", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationAddItem,
				RequestedDescription: &description, Quantity: &quantity, PromotionID: &promotionID}}}, false},
		{"complex on the whole quote", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &lower}, true},
		{"complex on named lines", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &improve, IntentItemIDs: []uuid.UUID{otherLine}}, true},
		{"complex with no intent", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex}, false},
		{"complex with an unknown intent", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &unknownIntent}, false},
		{"complex with operations", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &lower, Operations: []domain.ChangeRequestOperation{requantify}}, false},
		{"complex on another draft's line", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &lower, IntentItemIDs: []uuid.UUID{stranger}}, false},
		{"complex naming a line twice", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Intent: &lower, IntentItemIDs: []uuid.UUID{lineID, lineID}}, false},
		{"lower price with a preferred strategy", domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelComplex, Intent: &lower, LowerPriceStrategy: &adapt}, true},
		{"better quality with a lower-price strategy", domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelComplex, Intent: &improve, LowerPriceStrategy: &adapt}, false},
		{"lower price with a strategy of another goal", domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelComplex, Intent: &lower, LowerPriceStrategy: &quality}, false},
		{"atomic carrying a strategy", domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			LowerPriceStrategy: &adapt, Operations: []domain.ChangeRequestOperation{requantify}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.proposal.Interpretation = "El cliente pidió cambios."
			resolved, err := validateChangeRequestProposal(&tc.proposal, source, promotions, 10)
			if tc.valid && err != nil {
				t.Fatalf("validateChangeRequestProposal() = %v, want it accepted", err)
			}
			if !tc.valid && !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("validateChangeRequestProposal() = %v, want ErrInvalidInput", err)
			}
			if tc.valid && (resolved.Proposal.Level != tc.proposal.Level ||
				len(resolved.Operations) != len(tc.proposal.Operations) ||
				(resolved.Proposal.LowerPriceStrategy == nil) != (tc.proposal.LowerPriceStrategy == nil)) {
				t.Errorf("resolved = %+v, want the proposal carried through", resolved.Proposal)
			}
		})
	}
}
