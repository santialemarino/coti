package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteMessageWriter interface {
	CreateClientRequest(ctx context.Context, q repository.Querier,
		accountID, branchID, quoteID, channelID, actionID uuid.UUID, body string) error
}

// WithMessages wires the customer conversation persistence used by change requests.
func (s *QuoteDeliveryService) WithMessages(messages quoteMessageWriter) *QuoteDeliveryService {
	s.messages = messages
	return s
}

func (s *QuoteDeliveryService) createChangeRequest(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, quote domain.Quote, send domain.QuoteSend,
	action domain.ClientAction, message string) error {
	if s.messages == nil {
		return domain.ErrNotConfigured
	}
	version, err := s.quotes.GetCurrentVersion(ctx, q, accountID, quote.BranchID, quote.ID)
	if err != nil {
		return err
	}
	if !version.IsImmutable || version.ID != send.VersionID {
		return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
	}
	items, err := s.quotes.ListItems(ctx, q, accountID, version.ID)
	if err != nil {
		return err
	}
	alternatives, err := s.quotes.ListAlternativesByItemIDs(ctx, q, accountID,
		quoteItemIDs(items))
	if err != nil {
		return err
	}
	prices, err := s.prices.GetCurrentByProductIDs(ctx, q, accountID, quote.BranchID,
		quoteProductIDs(items, alternatives))
	if err != nil {
		return err
	}
	valuation, err := valueQuoteItems(items, prices)
	if err != nil {
		return err
	}
	// The seller must review alternatives again, even when one was approved on v1.
	for itemID, offered := range alternatives {
		for i := range offered {
			offered[i].ApprovedBySeller = false
		}
		alternatives[itemID] = offered
	}
	alternativePricings, err := valueQuoteAlternatives(alternatives, prices, valuation.currency)
	if err != nil {
		return err
	}
	applyAlternativePricing(alternatives, alternativePricings)
	draft, err := s.quotes.CreateVersion(ctx, q, accountID, domain.NewQuoteVersion{
		QuoteID: quote.ID, VersionNumber: version.VersionNumber + 1, Currency: valuation.currency,
		Total: valuation.total, Comment: &message,
	})
	if err != nil {
		return err
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
				ComboID: alternative.ComboID, Type: alternative.Type,
				Origin: alternative.Origin, Rank: alternative.Rank,
				ConfidenceScore: alternative.ConfidenceScore,
				PriceSnapshot:   alternative.PriceSnapshot,
			})
		}
	}
	if _, err := s.quotes.CreateItems(ctx, q, accountID, draft.ID, clonedItems); err != nil {
		return err
	}
	if err := s.quotes.CreateAlternatives(ctx, q, accountID, clonedAlternatives); err != nil {
		return err
	}
	if _, err := s.quotes.UpdateCurrentVersion(ctx, q, accountID, quote.ID, draft.ID); err != nil {
		return err
	}
	return s.messages.CreateClientRequest(ctx, q, accountID, quote.BranchID, quote.ID,
		send.ChannelID, action.ID, message)
}
