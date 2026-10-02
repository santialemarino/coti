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
	if _, err := cloneQuoteVersionForEditing(ctx, q, s.quotes, s.prices, accountID,
		quote.BranchID, quote.ID, *version, nil, &message); err != nil {
		return err
	}
	return s.messages.CreateClientRequest(ctx, q, accountID, quote.BranchID, quote.ID,
		send.ChannelID, action.ID, message)
}
