package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteVersionCloneRepository interface {
	CreateVersion(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteVersion) (*domain.QuoteVersion, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID,
		versionID uuid.UUID) (*domain.Quote, error)
	ListItems(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) ([]domain.QuoteItem, error)
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	CreateAlternatives(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		alternatives []domain.NewQuoteItemAlternative) error
}

func cloneQuoteVersionForEditing(
	ctx context.Context, q repository.Querier, quotes quoteVersionCloneRepository,
	prices branchPriceReader, accountID, branchID, quoteID uuid.UUID, source domain.QuoteVersion,
	authorID *uuid.UUID, comment *string,
) (*domain.QuoteVersion, error) {
	items, err := quotes.ListItems(ctx, q, accountID, source.ID)
	if err != nil {
		return nil, err
	}
	alternatives, err := quotes.ListAlternativesByItemIDs(ctx, q, accountID,
		quoteItemIDs(items))
	if err != nil {
		return nil, err
	}
	currentPrices, err := prices.GetCurrentByProductIDs(ctx, q, accountID, branchID,
		quoteProductIDs(items, alternatives))
	if err != nil {
		return nil, err
	}
	valuation, err := valueQuoteItems(items, currentPrices)
	if err != nil {
		return nil, err
	}

	for itemID, offered := range alternatives {
		for i := range offered {
			offered[i].ApprovedBySeller = false
		}
		alternatives[itemID] = offered
	}
	alternativePricings, err := valueQuoteAlternatives(alternatives, currentPrices,
		valuation.currency)
	if err != nil {
		return nil, err
	}
	applyAlternativePricing(alternatives, alternativePricings)

	draft, err := quotes.CreateVersion(ctx, q, accountID, domain.NewQuoteVersion{
		QuoteID: quoteID, AuthorID: authorID, VersionNumber: source.VersionNumber + 1,
		Currency: valuation.currency, Total: valuation.total, Comment: comment,
	})
	if err != nil {
		return nil, err
	}

	clonedItems := make([]domain.NewQuoteItem, 0, len(items))
	clonedAlternatives := make([]domain.NewQuoteItemAlternative, 0)
	for _, item := range valuation.items {
		newID := uuid.New()
		clonedItems = append(clonedItems, domain.NewQuoteItem{
			ID: newID, ProductID: item.ProductID, RequestedDescription: item.RequestedDescription,
			Quantity: item.Quantity, Unit: item.Unit, ConfidenceScore: item.ConfidenceScore,
			MatchStatus: item.MatchStatus, QuantityRationale: item.QuantityRationale,
			UnitPriceSnapshot: item.UnitPriceSnapshot, MinPriceSnapshot: item.MinPriceSnapshot,
			Subtotal: item.Subtotal,
		})
		for _, alternative := range alternatives[item.ID] {
			clonedAlternatives = append(clonedAlternatives, domain.NewQuoteItemAlternative{
				QuoteItemID: newID, ProductID: alternative.ProductID,
				ComboID: alternative.ComboID, Type: alternative.Type, Origin: alternative.Origin,
				Rank: alternative.Rank, ConfidenceScore: alternative.ConfidenceScore,
				PriceSnapshot: alternative.PriceSnapshot,
			})
		}
	}
	if _, err := quotes.CreateItems(ctx, q, accountID, draft.ID, clonedItems); err != nil {
		return nil, err
	}
	if err := quotes.CreateAlternatives(ctx, q, accountID, clonedAlternatives); err != nil {
		return nil, err
	}
	if _, err := quotes.UpdateCurrentVersion(ctx, q, accountID, quoteID, draft.ID); err != nil {
		return nil, err
	}
	return draft, nil
}
