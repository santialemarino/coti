package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type recommendationRequestQuotes interface {
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Quote, error)
	ClaimForEdit(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteVersion, error)
	ListItems(context.Context, repository.Querier, uuid.UUID, uuid.UUID) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(context.Context, repository.Querier, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	SetRecommendationAlternative(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, domain.ProductAlternativeType, decimal.NullDecimal, decimal.NullDecimal) (*domain.QuoteItemAlternative, error)
	PromoteMatcherRecommendation(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, domain.ProductAlternativeType, decimal.NullDecimal, decimal.NullDecimal) (*domain.QuoteItemAlternative, error)
}

// RecommendationRequestService finds catalog suggestions only when the seller requests them.
type RecommendationRequestService struct {
	db           tenantTxRunner
	quotes       recommendationRequestQuotes
	alternatives productAlternativeReader
	prices       branchPriceReader
}

// NewRecommendationRequestService builds a RecommendationRequestService.
func NewRecommendationRequestService(db tenantTxRunner, quotes recommendationRequestQuotes,
	alternatives productAlternativeReader, prices branchPriceReader,
) *RecommendationRequestService {
	return &RecommendationRequestService{db: db, quotes: quotes, alternatives: alternatives, prices: prices}
}

// Search reads eligible catalog alternatives without changing or approving the quote.
func (s *RecommendationRequestService) Search(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) ([]domain.RecommendationProposal, error) {
	if err := requireBranch(tenant, "quote recommendations"); err != nil {
		return nil, err
	}
	result := []domain.RecommendationProposal{}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		version, err := s.editableVersion(ctx, q, tenant, quote)
		if err != nil {
			return err
		}
		result, err = s.findProposals(ctx, q, tenant.AccountID, quote.BranchID, *version)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AddIfVacant adds a still-eligible proposal without replacing the seller's existing choices.
func (s *RecommendationRequestService) AddIfVacant(ctx context.Context, tenant domain.Tenant,
	quoteID, itemID, productID uuid.UUID, kind domain.ProductAlternativeType,
) (*domain.QuoteItemAlternative, error) {
	if err := requireBranch(tenant, "quote recommendations"); err != nil {
		return nil, err
	}
	if !isQuoteRecommendationType(kind) {
		return nil, fmt.Errorf("%w: recommendation type must be ECONOMY or PREMIUM", domain.ErrInvalidInput)
	}
	var added *domain.QuoteItemAlternative
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		version, err := s.editableVersion(ctx, q, tenant, quote)
		if err != nil {
			return err
		}
		proposals, err := s.findProposals(ctx, q, tenant.AccountID, quote.BranchID, *version)
		if err != nil {
			return err
		}
		for _, proposal := range proposals {
			if proposal.ItemID != itemID || proposal.ProductID != productID || proposal.RecommendationType != kind {
				continue
			}
			prices, priceErr := s.prices.GetCurrentByProductIDs(ctx, q, tenant.AccountID, quote.BranchID, []uuid.UUID{productID})
			if priceErr != nil {
				return priceErr
			}
			price, ok := prices[productID]
			priceCurrency := price.Currency
			if priceCurrency == "" {
				priceCurrency = domain.DefaultCurrency
			}
			if !ok || !price.Price.Equal(proposal.UnitPrice) || priceCurrency != proposal.Currency {
				return domain.ErrConflict
			}
			offers, offerErr := s.quotes.ListAlternativesByItemIDs(ctx, q, tenant.AccountID, []uuid.UUID{itemID})
			if offerErr != nil {
				return offerErr
			}
			matcherID, reusable := reusableMatcherCandidate(offers[itemID], productID)
			if !reusable || recommendationSlotOccupied(offers[itemID], kind) {
				return domain.ErrConflict
			}
			if matcherID != nil {
				added, err = s.quotes.PromoteMatcherRecommendation(ctx, q, tenant.AccountID, version.ID,
					itemID, *matcherID, productID, kind, decimal.NewNullDecimal(price.Price), price.MinPrice)
				return err
			}
			added, err = s.quotes.SetRecommendationAlternative(ctx, q, tenant.AccountID, version.ID,
				itemID, productID, kind, decimal.NewNullDecimal(price.Price), price.MinPrice)
			return err
		}
		return domain.ErrConflict
	})
	return added, err
}

// FillVacant adds every still-eligible catalog proposal to its empty slot, unapproved, in one
// transaction. Slots the seller already decided are never touched.
func (s *RecommendationRequestService) FillVacant(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID,
) ([]domain.FilledRecommendation, error) {
	if err := requireBranch(tenant, "quote recommendations"); err != nil {
		return nil, err
	}
	filled := []domain.FilledRecommendation{}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		version, err := s.editableVersion(ctx, q, tenant, quote)
		if err != nil {
			return err
		}
		proposals, err := s.findProposals(ctx, q, tenant.AccountID, quote.BranchID, *version)
		if err != nil || len(proposals) == 0 {
			return err
		}
		productIDs := make([]uuid.UUID, 0, len(proposals))
		itemIDs := make([]uuid.UUID, 0, len(proposals))
		for _, proposal := range proposals {
			productIDs = append(productIDs, proposal.ProductID)
			itemIDs = append(itemIDs, proposal.ItemID)
		}
		prices, err := s.prices.GetCurrentByProductIDs(ctx, q, tenant.AccountID, quote.BranchID, productIDs)
		if err != nil {
			return err
		}
		offers, err := s.quotes.ListAlternativesByItemIDs(ctx, q, tenant.AccountID, itemIDs)
		if err != nil {
			return err
		}
		for _, proposal := range proposals {
			price, ok := prices[proposal.ProductID]
			matcherID, reusable := reusableMatcherCandidate(offers[proposal.ItemID], proposal.ProductID)
			// A price that moved since the proposal was found was never checked against the line.
			if !ok || !price.Price.Equal(proposal.UnitPrice) || !reusable ||
				recommendationSlotOccupied(offers[proposal.ItemID], proposal.RecommendationType) {
				continue
			}
			var added *domain.QuoteItemAlternative
			if matcherID != nil {
				added, err = s.quotes.PromoteMatcherRecommendation(ctx, q, tenant.AccountID, version.ID,
					proposal.ItemID, *matcherID, proposal.ProductID, proposal.RecommendationType,
					decimal.NewNullDecimal(price.Price), price.MinPrice)
			} else {
				added, err = s.quotes.SetRecommendationAlternative(ctx, q, tenant.AccountID, version.ID,
					proposal.ItemID, proposal.ProductID, proposal.RecommendationType,
					decimal.NewNullDecimal(price.Price), price.MinPrice)
			}
			if err != nil {
				return err
			}
			filled = append(filled, domain.FilledRecommendation{ItemID: proposal.ItemID, Alternative: *added})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return filled, nil
}

func (s *RecommendationRequestService) editableVersion(ctx context.Context, q repository.Querier, tenant domain.Tenant, quote *domain.Quote) (*domain.QuoteVersion, error) {
	if err := requireUnarchived(*quote); err != nil {
		return nil, err
	}
	if quote.CurrentStatus != domain.QuoteStatusQuoted {
		return nil, domain.ErrConflict
	}
	version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID, quote.ID)
	if err != nil {
		return nil, err
	}
	if version.IsImmutable {
		return nil, domain.ErrConflict
	}
	return version, nil
}

func (s *RecommendationRequestService) findProposals(ctx context.Context, q repository.Querier,
	accountID, branchID uuid.UUID, version domain.QuoteVersion,
) ([]domain.RecommendationProposal, error) {
	items, err := s.quotes.ListItems(ctx, q, accountID, version.ID)
	if err != nil {
		return nil, err
	}
	offers, err := s.quotes.ListAlternativesByItemIDs(ctx, q, accountID, quoteItemIDs(items))
	if err != nil {
		return nil, err
	}
	productIDs := quoteItemProductIDs(items)
	if len(productIDs) == 0 {
		return []domain.RecommendationProposal{}, nil
	}
	configured := make(map[domain.ProductAlternativeType]map[uuid.UUID][]domain.ProductAlternativeView, 2)
	ids := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]bool)
	for _, kind := range []domain.ProductAlternativeType{domain.ProductAlternativeEconomy, domain.ProductAlternativePremium} {
		configured[kind], err = s.alternatives.ListByBaseProductIDs(ctx, q, accountID, productIDs, kind)
		if err != nil {
			return nil, err
		}
		for _, candidates := range configured[kind] {
			for _, candidate := range candidates {
				if !seen[candidate.Product.ID] {
					ids = append(ids, candidate.Product.ID)
					seen[candidate.Product.ID] = true
				}
			}
		}
	}
	if len(ids) == 0 {
		return []domain.RecommendationProposal{}, nil
	}
	prices, err := s.prices.GetCurrentByProductIDs(ctx, q, accountID, branchID, ids)
	if err != nil {
		return nil, err
	}
	currency := version.Currency
	if currency == "" {
		currency = domain.DefaultCurrency
	}
	proposals := []domain.RecommendationProposal{}
	for _, item := range items {
		if item.ProductID == nil {
			continue
		}
		for _, kind := range []domain.ProductAlternativeType{domain.ProductAlternativeEconomy, domain.ProductAlternativePremium} {
			if recommendationSlotOccupied(offers[item.ID], kind) {
				continue
			}
			for _, candidate := range configured[kind][*item.ProductID] {
				product := candidate.Product
				_, reusable := reusableMatcherCandidate(offers[item.ID], product.ID)
				price, ok := prices[product.ID]
				priceCurrency := price.Currency
				if priceCurrency == "" {
					priceCurrency = domain.DefaultCurrency
				}
				if product.ID == *item.ProductID || !reusable ||
					!ok || priceCurrency != currency || !validRecommendationPrice(kind, item.UnitPriceSnapshot, decimal.NewNullDecimal(price.Price)) {
					continue
				}
				proposals = append(proposals, domain.RecommendationProposal{ItemID: item.ID, ProductID: product.ID,
					RecommendationType: kind, Name: product.CanonicalName, Code: product.Code, Unit: product.Unit,
					UnitPrice: price.Price, Currency: currency})
				break
			}
		}
	}
	return proposals, nil
}

func recommendationSlotOccupied(offers []domain.QuoteItemAlternative, kind domain.ProductAlternativeType) bool {
	for _, offer := range offers {
		if offer.RecommendationType != nil && *offer.RecommendationType == kind {
			return true
		}
	}
	return false
}

func reusableMatcherCandidate(offers []domain.QuoteItemAlternative, productID uuid.UUID) (*uuid.UUID, bool) {
	var matcherID *uuid.UUID
	for _, offer := range offers {
		if offer.ProductID != nil && *offer.ProductID == productID {
			if matcherID != nil || offer.Type != domain.QuoteItemAlternativeTypeProduct ||
				offer.Origin != domain.QuoteItemAlternativeOriginAI || offer.RecommendationType != nil ||
				offer.ApprovedBySeller || offer.ChosenByClient {
				return nil, false
			}
			id := offer.ID
			matcherID = &id
		}
	}
	return matcherID, true
}
