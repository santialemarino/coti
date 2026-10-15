package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeConversationQuotes struct {
	quotes map[uuid.UUID]domain.Quote
	locked []uuid.UUID
}

func (f *fakeConversationQuotes) GetByIDForUpdate(_ context.Context, _ repository.Querier, _, _,
	id uuid.UUID,
) (*domain.Quote, error) {
	f.locked = append(f.locked, id)
	quote, found := f.quotes[id]
	if !found {
		return nil, domain.ErrNotFound
	}
	return &quote, nil
}

type fakeConversationSends struct {
	candidates   []domain.ConversationCandidate
	destinations []string
}

func (f *fakeConversationSends) ListLiveConversations(_ context.Context, _ repository.Querier,
	_, _, _ uuid.UUID, destinations []string, _ time.Time,
) ([]domain.ConversationCandidate, error) {
	f.destinations = destinations
	return f.candidates, nil
}

type fakeConversationMessages struct{ created []domain.NewQuoteMessage }

func (f *fakeConversationMessages) Create(_ context.Context, _ repository.Querier, _, _ uuid.UUID,
	in domain.NewQuoteMessage,
) (*domain.QuoteMessage, error) {
	f.created = append(f.created, in)
	return &domain.QuoteMessage{ID: in.ID, QuoteID: in.QuoteID, AuthorType: in.AuthorType}, nil
}

type fakeConversationWindows struct{ opened []time.Time }

func (f *fakeConversationWindows) OpenOrExtend(_ context.Context, _ repository.Querier, _, _,
	_ uuid.UUID, now time.Time, _, _ time.Duration,
) (*domain.MessageBatch, error) {
	f.opened = append(f.opened, now)
	return &domain.MessageBatch{}, nil
}

func conversationRoute() domain.InboundChannelRoute {
	return domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
}

// The candidate read is unlocked, so a quote accepted in between is skipped for the next one.
func TestConversationService_RouteRechecksEachCandidateUnderItsLock(t *testing.T) {
	t.Parallel()
	accepted, live, older := uuid.New(), uuid.New(), uuid.New()
	quotes := &fakeConversationQuotes{quotes: map[uuid.UUID]domain.Quote{
		accepted: {ID: accepted, CurrentStatus: domain.QuoteStatusAccepted},
		live:     {ID: live, CurrentStatus: domain.QuoteStatusChangeRequested},
		older:    {ID: older, CurrentStatus: domain.QuoteStatusSent},
	}}
	sends := &fakeConversationSends{candidates: []domain.ConversationCandidate{
		{QuoteID: accepted}, {QuoteID: live, QuoteSendID: uuid.New()}, {QuoteID: older},
	}}
	service := NewConversationService(quotes, sends, nil, nil, config.ConversationConfig{})

	match, err := service.Route(context.Background(), nil, conversationRoute(), "5491112345678")
	if err != nil {
		t.Fatalf("Route() = %v, want no error", err)
	}
	if match == nil || match.QuoteID != live || match.QuoteSendID != sends.candidates[1].QuoteSendID ||
		match.OtherLiveQuotes != 1 {
		t.Fatalf("Route() = %+v, want the live quote with one other candidate", match)
	}
	if len(sends.destinations) != 2 {
		t.Errorf("destinations = %v, want both Argentine forms", sends.destinations)
	}
}

func TestConversationService_RouteFindsNoConversationAfterTheLastCandidateLeft(t *testing.T) {
	t.Parallel()
	archived := uuid.New()
	now := time.Now()
	quotes := &fakeConversationQuotes{quotes: map[uuid.UUID]domain.Quote{
		archived: {ID: archived, CurrentStatus: domain.QuoteStatusSent, ArchivedAt: &now},
	}}
	sends := &fakeConversationSends{candidates: []domain.ConversationCandidate{{QuoteID: archived}}}
	service := NewConversationService(quotes, sends, nil, nil, config.ConversationConfig{})

	match, err := service.Route(context.Background(), nil, conversationRoute(), "5491112345678")
	if err != nil || match != nil {
		t.Errorf("Route() = %+v, %v; want no conversation", match, err)
	}
}

func TestConversationService_AppendMessageMovesTheWindowOnlyForTheClient(t *testing.T) {
	t.Parallel()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	for _, test := range []struct {
		author     domain.MessageAuthorType
		wantWindow bool
	}{
		{domain.MessageAuthorClient, true},
		{domain.MessageAuthorSeller, false},
		{domain.MessageAuthorSystem, false},
	} {
		t.Run(string(test.author), func(t *testing.T) {
			t.Parallel()
			messages, windows := &fakeConversationMessages{}, &fakeConversationWindows{}
			service := NewConversationService(nil, nil, messages, windows, config.ConversationConfig{})
			_, err := service.AppendMessage(context.Background(), nil, tenant, domain.NewQuoteMessage{
				QuoteID: uuid.New(), AuthorType: test.author, Body: "hola"})
			if err != nil {
				t.Fatalf("AppendMessage() = %v, want no error", err)
			}
			if len(messages.created) != 1 || messages.created[0].ID == uuid.Nil {
				t.Errorf("created = %#v, want one message with an ID", messages.created)
			}
			if got := len(windows.opened) == 1; got != test.wantWindow {
				t.Errorf("window moved = %t, want %t", got, test.wantWindow)
			}
		})
	}
}
