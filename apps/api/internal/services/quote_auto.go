package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type automaticQuoteInputLocker interface {
	LockAutomaticQuoteInputs(context.Context, repository.Querier, uuid.UUID, uuid.UUID, []uuid.UUID) error
}

// WithAutomaticQuoteInputs serializes initial valuation with catalog and price edits.
func (s *QuoteService) WithAutomaticQuoteInputs(inputs automaticQuoteInputLocker) *QuoteService {
	s.automaticInputs = inputs
	return s
}

func (s *QuoteService) automaticMaterialsEligible(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, items []domain.QuoteItem, prices map[uuid.UUID]domain.BranchPrice,
) (bool, error) {
	review := func(index int, reason string) (bool, error) {
		s.log.InfoContext(ctx, "automatic quotation requires review", "reason", reason, "item_index", index)
		return false, nil
	}
	if len(items) == 0 || s.recommendationProducts == nil {
		return review(0, "INCOMPLETE_INPUT")
	}
	products, err := s.recommendationProducts.GetByIDs(ctx, q, accountID, quoteItemProductIDs(items))
	if err != nil {
		return false, err
	}
	currency := ""
	for index, item := range items {
		if item.MatchStatus != domain.ItemMatchStatusMatched || item.ProductID == nil {
			return review(index+1, "MATCH_REQUIRES_REVIEW")
		}
		if item.Quantity.LessThanOrEqual(decimal.Zero) {
			return review(index+1, "QUANTITY_REQUIRES_REVIEW")
		}
		product, found := products[*item.ProductID]
		if !found || !product.IsActive || product.AccountID != accountID {
			return review(index+1, "PRODUCT_UNAVAILABLE")
		}
		if item.Unit == nil || product.Unit == nil ||
			priceUnitKey(*item.Unit) == "" || priceUnitKey(*item.Unit) != priceUnitKey(*product.Unit) {
			return review(index+1, "UNIT_REQUIRES_REVIEW")
		}
		price, found := prices[*item.ProductID]
		if !found || price.Price.IsNegative() || price.Currency == "" {
			return review(index+1, "MISSING_PRICE")
		}
		if currency != "" && currency != price.Currency {
			return review(index+1, "MIXED_CURRENCIES")
		}
		currency = price.Currency
	}
	return true, nil
}
