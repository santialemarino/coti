package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteMessageWriter interface {
	Create(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		in domain.NewQuoteMessage) (*domain.QuoteMessage, error)
	CreateClientRequest(ctx context.Context, q repository.Querier,
		accountID, branchID, quoteID, channelID, actionID uuid.UUID, body string) error
}

// changeRequestQueue is the conversation window a webapp change request is queued through.
type changeRequestQueue interface {
	OpenOrExtend(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
		now time.Time, reset, limit time.Duration) (*domain.MessageBatch, error)
	CloseOpen(ctx context.Context, q repository.Querier, accountID,
		quoteID uuid.UUID) (*domain.MessageBatch, error)
}

// WithMessages wires the conversation a seller's send is recorded in and a change request is
// written into and queued from.
func (s *QuoteDeliveryService) WithMessages(messages quoteMessageWriter,
	windows changeRequestQueue) *QuoteDeliveryService {
	s.messages = messages
	s.windows = windows
	return s
}

// createChangeRequest opens the customer's draft and queues their message in a batch that closes at
// once, joining any window still open; the conversation handler reads it from the queue.
func (s *QuoteDeliveryService) createChangeRequest(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, quote domain.Quote, send domain.QuoteSend,
	action domain.ClientAction, message string,
) error {
	if s.messages == nil || s.windows == nil {
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
	if err := s.messages.CreateClientRequest(ctx, q, accountID, quote.BranchID, quote.ID,
		send.ChannelID, action.ID, message); err != nil {
		return err
	}
	if _, err := s.windows.OpenOrExtend(ctx, q, accountID, quote.BranchID, quote.ID,
		s.now().UTC(), 0, 0); err != nil {
		return err
	}
	_, err = s.windows.CloseOpen(ctx, q, accountID, quote.ID)
	return err
}
