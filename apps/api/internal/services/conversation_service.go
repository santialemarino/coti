package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type conversationQuoteLocker interface {
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
}

type conversationSendFinder interface {
	ListLiveConversations(ctx context.Context, q repository.Querier, accountID, branchID,
		channelID uuid.UUID, destinations []string, now time.Time) ([]domain.ConversationCandidate, error)
}

type conversationMessageStore interface {
	Create(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		in domain.NewQuoteMessage) (*domain.QuoteMessage, error)
}

type conversationWindowStore interface {
	OpenOrExtend(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
		now time.Time, reset, limit time.Duration) (*domain.MessageBatch, error)
}

// ConversationService routes inbound messages into sent quotes' conversations and keeps their
// windows. Its methods run inside the caller's tenant transaction.
type ConversationService struct {
	quotes   conversationQuoteLocker
	sends    conversationSendFinder
	messages conversationMessageStore
	windows  conversationWindowStore
	cfg      config.ConversationConfig
	now      func() time.Time
}

// NewConversationService builds a ConversationService.
func NewConversationService(quotes conversationQuoteLocker, sends conversationSendFinder,
	messages conversationMessageStore, windows conversationWindowStore, cfg config.ConversationConfig,
) *ConversationService {
	return &ConversationService{quotes: quotes, sends: sends, messages: messages, windows: windows,
		cfg: cfg, now: time.Now}
}

// Route finds the conversation an inbound sender continues on the channel it wrote to, and locks
// that quote for the rest of the transaction. A live quote the message names («COT-000027», «la
// 27») wins; otherwise the newest send does. Returns nil when no live quote was sent to the sender,
// including a sender no quote can have been sent to.
func (s *ConversationService) Route(ctx context.Context, q repository.Querier,
	route domain.InboundChannelRoute, senderID, body string,
) (*domain.ConversationRoute, error) {
	if err := route.Validate(); err != nil {
		return nil, err
	}
	destinations, err := domain.InboundSenderDestinations(route.ChannelType, senderID)
	if errors.Is(err, domain.ErrInvalidInput) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidates, err := s.sends.ListLiveConversations(ctx, q, route.AccountID, route.BranchID,
		route.ChannelID, destinations, s.now())
	if err != nil {
		return nil, err
	}
	// A quote the customer names leaves no doubt about which one they mean.
	if references := domain.QuoteReferences(body); len(references) > 0 {
		for _, candidate := range candidates {
			if !slices.Contains(references, candidate.QuoteNumber) {
				continue
			}
			match, err := s.claim(ctx, q, route, candidate, 0)
			if err != nil || match != nil {
				return match, err
			}
		}
	}
	// The candidate read takes no lock, so each one is re-checked under the quote's own row lock:
	// a seller may have archived it in between.
	for i, candidate := range candidates {
		match, err := s.claim(ctx, q, route, candidate, len(candidates)-i-1)
		if err != nil || match != nil {
			return match, err
		}
	}
	return nil, nil
}

// claim locks a candidate's quote and returns its route when it still takes a conversation.
func (s *ConversationService) claim(ctx context.Context, q repository.Querier,
	route domain.InboundChannelRoute, candidate domain.ConversationCandidate, others int,
) (*domain.ConversationRoute, error) {
	quote, err := s.quotes.GetByIDForUpdate(ctx, q, route.AccountID, route.BranchID,
		candidate.QuoteID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil || !acceptsConversation(*quote) {
		return nil, err
	}
	return &domain.ConversationRoute{QuoteID: quote.ID, QuoteSendID: candidate.QuoteSendID,
		OtherLiveQuotes: others,
		Accepted:        quote.CurrentStatus == domain.QuoteStatusAccepted}, nil
}

// AppendMessage records a message in a quote's conversation. A CLIENT message also opens the
// quote's window or extends it; SELLER and SYSTEM messages are context and never move it.
func (s *ConversationService) AppendMessage(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, in domain.NewQuoteMessage,
) (*domain.QuoteMessage, error) {
	if err := requireBranch(tenant, "a conversation message"); err != nil {
		return nil, err
	}
	switch in.AuthorType {
	case domain.MessageAuthorClient, domain.MessageAuthorSeller, domain.MessageAuthorSystem:
	default:
		return nil, fmt.Errorf("%w: unknown message author %q", domain.ErrInvalidInput, in.AuthorType)
	}
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	now := s.now().UTC()
	if in.ReceivedAt.IsZero() {
		in.ReceivedAt = now
	}
	message, err := s.messages.Create(ctx, q, tenant.AccountID, tenant.BranchID, in)
	if err != nil {
		return nil, err
	}
	if in.AuthorType != domain.MessageAuthorClient || in.RecordedOnly {
		return message, nil
	}
	if _, err := s.windows.OpenOrExtend(ctx, q, tenant.AccountID, tenant.BranchID, in.QuoteID, now,
		s.cfg.WindowReset, s.cfg.WindowMax); err != nil {
		return nil, err
	}
	return message, nil
}

func acceptsConversation(quote domain.Quote) bool {
	return quote.ArchivedAt == nil && (quote.CurrentStatus == domain.QuoteStatusSent ||
		quote.CurrentStatus == domain.QuoteStatusChangeRequested ||
		quote.CurrentStatus == domain.QuoteStatusAccepted)
}
