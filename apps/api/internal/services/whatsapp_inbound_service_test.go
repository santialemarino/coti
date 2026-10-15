package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeWhatsAppInboundDB struct {
	scopes []domain.Tenant
	locks  []string
	// results holds what each transaction returned, so a test can see a rollback.
	results []error
}

func (f *fakeWhatsAppInboundDB) CrossAccount() repository.Querier { return nil }

func (f *fakeWhatsAppInboundDB) InTenantTx(
	_ context.Context, tenant domain.Tenant, fn func(repository.Querier) error,
) error {
	f.scopes = append(f.scopes, tenant)
	err := fn(nil)
	f.results = append(f.results, err)
	return err
}

func (f *fakeWhatsAppInboundDB) WithAdvisoryLock(
	_ context.Context, key string, fn func() error,
) error {
	f.locks = append(f.locks, key)
	return fn()
}

type fakeWhatsAppInboundRouter struct {
	routes map[string]domain.InboundChannelRoute
}

func (f *fakeWhatsAppInboundRouter) ResolveChannels(_ context.Context, _ domain.ChannelType,
	identifiers []string,
) (map[string]domain.InboundChannelRoute, error) {
	resolved := make(map[string]domain.InboundChannelRoute)
	for _, identifier := range identifiers {
		if route, found := f.routes[identifier]; found {
			resolved[identifier] = route
		}
	}
	return resolved, nil
}

// fakeWhatsAppInboundMessages keeps the first reservation, as the unique provider key does.
type fakeWhatsAppInboundMessages struct {
	reserved []domain.NewInboundChannelMessage
	first    *domain.InboundChannelMessage
}

func (f *fakeWhatsAppInboundMessages) Reserve(_ context.Context, _ repository.Querier, accountID,
	branchID, channelID uuid.UUID, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, error) {
	f.reserved = append(f.reserved, in)
	if f.first == nil {
		f.first = &domain.InboundChannelMessage{ID: in.ID, AccountID: accountID, BranchID: branchID,
			ChannelID: channelID, RFQID: in.RFQID, QuoteMessageID: in.QuoteMessageID}
	}
	return f.first, nil
}

type fakeWhatsAppInboundConversations struct {
	match    *domain.ConversationRoute
	appended []domain.NewQuoteMessage
}

func (f *fakeWhatsAppInboundConversations) Route(context.Context, repository.Querier,
	domain.InboundChannelRoute, string,
) (*domain.ConversationRoute, error) {
	return f.match, nil
}

func (f *fakeWhatsAppInboundConversations) AppendMessage(_ context.Context, _ repository.Querier,
	_ domain.Tenant, in domain.NewQuoteMessage,
) (*domain.QuoteMessage, error) {
	f.appended = append(f.appended, in)
	return &domain.QuoteMessage{ID: in.ID, QuoteID: in.QuoteID}, nil
}

type fakeWhatsAppInboundRFQs struct {
	routes []domain.InboundChannelRoute
	inputs []domain.InboundTextRFQDraftInput
	rfqIDs []uuid.UUID
}

type fakeWhatsAppInboundConfigs struct {
	configs []domain.ChannelConfiguration
}

func (f *fakeWhatsAppInboundConfigs) ListActiveWhatsAppConfigurationsCrossAccount(
	context.Context, repository.Querier,
) ([]domain.ChannelConfiguration, error) {
	return f.configs, nil
}

type fakeWhatsAppChannelOpener struct{}

func (fakeWhatsAppChannelOpener) Open(value string) (string, error) {
	return strings.TrimPrefix(value, "sealed:"), nil
}

func (f *fakeWhatsAppInboundRFQs) CreateInboundTextDraftForMessage(_ context.Context,
	route domain.InboundChannelRoute, in domain.InboundTextRFQDraftInput, rfqID uuid.UUID,
) (*domain.TextRFQDraft, error) {
	f.routes = append(f.routes, route)
	f.inputs = append(f.inputs, in)
	f.rfqIDs = append(f.rfqIDs, rfqID)
	return &domain.TextRFQDraft{}, nil
}

func TestWhatsAppInboundService_IngestReservesRouteBeforeDrafting(t *testing.T) {
	t.Parallel()
	accountID := uuid.New()
	branchID := uuid.New()
	channelID := uuid.New()
	route := domain.InboundChannelRoute{AccountID: accountID, BranchID: branchID,
		ChannelID: channelID, ChannelType: domain.ChannelTypeWhatsApp}
	db := &fakeWhatsAppInboundDB{}
	messages := &fakeWhatsAppInboundMessages{}
	rfqs := &fakeWhatsAppInboundRFQs{}
	service := NewWhatsAppInboundService(db, &fakeWhatsAppInboundRouter{
		routes: map[string]domain.InboundChannelRoute{"phone-id": route},
	}, messages, nil, rfqs, nil, nil)
	service.now = func() time.Time { return time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC) }
	label := "Obra Norte (5491112345678)"

	err := service.Ingest(context.Background(), []domain.InboundTextMessage{{
		ExternalMessageID: "wamid.inbound", DestinationIdentifier: " phone-id ",
		SenderID: "5491112345678", SenderLabel: &label, Body: " 10 bolsas de cemento ",
		Payload: []byte(`{"message":"wamid.inbound"}`),
	}})
	if err != nil {
		t.Fatalf("Ingest() = %v, want no error", err)
	}
	if len(messages.reserved) != 1 || messages.reserved[0].ProviderReceivedAt.IsZero() {
		t.Fatalf("reservations = %#v, want one event with a timestamp", messages.reserved)
	}
	if len(rfqs.inputs) != 1 || rfqs.inputs[0].RawText != "10 bolsas de cemento" ||
		rfqs.inputs[0].ClientLabel == nil || *rfqs.inputs[0].ClientLabel != label {
		t.Errorf("RFQ input = %#v, want normalized inbound text and label", rfqs.inputs)
	}
	if len(rfqs.rfqIDs) != 1 || messages.first.RFQID == nil || rfqs.rfqIDs[0] != *messages.first.RFQID {
		t.Errorf("RFQ IDs = %v, want the durable reservation %v", rfqs.rfqIDs, messages.first.RFQID)
	}
	if len(db.scopes) != 1 || db.scopes[0].AccountID != route.AccountID ||
		db.scopes[0].BranchID != route.BranchID || len(db.locks) != 1 {
		t.Errorf("scopes/locks = %v/%v, want one resolved tenant lock", db.scopes, db.locks)
	}
}

func TestWhatsAppInboundService_IngestSkipsAnUnconfiguredDestination(t *testing.T) {
	t.Parallel()
	db := &fakeWhatsAppInboundDB{}
	messages := &fakeWhatsAppInboundMessages{}
	rfqs := &fakeWhatsAppInboundRFQs{}
	service := NewWhatsAppInboundService(db, &fakeWhatsAppInboundRouter{}, messages, nil, rfqs, nil, nil)

	err := service.Ingest(context.Background(), []domain.InboundTextMessage{{
		ExternalMessageID: "wamid.unrouted", DestinationIdentifier: "phone-id", SenderID: "5491112345678",
		Body: "cemento", Payload: []byte(`{"message":"wamid.unrouted"}`),
	}})
	if err != nil {
		t.Fatalf("Ingest() = %v, want an unconfigured destination acknowledged", err)
	}
	if len(messages.reserved) != 0 || len(rfqs.inputs) != 0 || len(db.scopes) != 0 {
		t.Errorf("intake = %#v/%#v/%#v, want no tenant work", messages.reserved, rfqs.inputs, db.scopes)
	}
}

func TestWhatsAppInboundService_VerifyAcceptsConfiguredChannelToken(t *testing.T) {
	t.Parallel()
	configs := &fakeWhatsAppInboundConfigs{configs: []domain.ChannelConfiguration{{
		ChannelID: uuid.New(), AccountID: uuid.New(), BranchID: uuid.New(),
		Type:   domain.ChannelTypeWhatsApp,
		Config: []byte(`{"phone_number_id":"phone-id","access_token":"sealed:access-token","webhook_verify_token":"sealed:verify-token"}`),
	}}}
	service := NewWhatsAppInboundService(&fakeWhatsAppInboundDB{}, nil, nil, configs, nil,
		fakeWhatsAppChannelOpener{}, nil)

	accepted, err := service.Verify(context.Background(), "verify-token")
	if err != nil || !accepted {
		t.Errorf("Verify() = %t, %v; want true, nil", accepted, err)
	}
	rejected, err := service.Verify(context.Background(), "other-token")
	if err != nil || rejected {
		t.Errorf("Verify() = %t, %v; want false, nil", rejected, err)
	}
}

func inboundTestService(conversations *fakeWhatsAppInboundConversations,
) (*WhatsAppInboundService, *fakeWhatsAppInboundMessages, *fakeWhatsAppInboundRFQs,
	*fakeWhatsAppInboundDB) {
	route := domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
	messages := &fakeWhatsAppInboundMessages{}
	rfqs := &fakeWhatsAppInboundRFQs{}
	db := &fakeWhatsAppInboundDB{}
	service := NewWhatsAppInboundService(db, &fakeWhatsAppInboundRouter{
		routes: map[string]domain.InboundChannelRoute{"phone-id": route},
	}, messages, nil, rfqs, nil, nil).WithConversations(conversations)
	return service, messages, rfqs, db
}

func inboundTestMessage() []domain.InboundTextMessage {
	return []domain.InboundTextMessage{{ExternalMessageID: "wamid.reply",
		DestinationIdentifier: "phone-id", SenderID: "5491112345678", Body: "sumale 5 bolsas",
		Payload: []byte(`{"message":"wamid.reply"}`)}}
}

func TestWhatsAppInboundService_IngestJoinsALiveConversationWithoutAnRFQ(t *testing.T) {
	t.Parallel()
	quoteID := uuid.New()
	conversations := &fakeWhatsAppInboundConversations{
		match: &domain.ConversationRoute{QuoteID: quoteID, OtherLiveQuotes: 1}}
	service, messages, rfqs, _ := inboundTestService(conversations)

	if err := service.Ingest(context.Background(), inboundTestMessage()); err != nil {
		t.Fatalf("Ingest() = %v, want no error", err)
	}
	if len(conversations.appended) != 1 || conversations.appended[0].QuoteID != quoteID ||
		conversations.appended[0].AuthorType != domain.MessageAuthorClient ||
		conversations.appended[0].OtherLiveQuotes != 1 {
		t.Fatalf("appended = %#v, want one CLIENT message on the quote carrying the hint", conversations.appended)
	}
	reserved := messages.reserved[0]
	if reserved.RFQID != nil || reserved.QuoteMessageID == nil ||
		*reserved.QuoteMessageID != conversations.appended[0].ID {
		t.Errorf("reservation = %#v, want the conversation message and no RFQ", reserved)
	}
	if len(rfqs.inputs) != 0 {
		t.Errorf("RFQ drafts = %d, want none", len(rfqs.inputs))
	}
}

// A retry keeps the first outcome: an order reserved before a quote went live stays an order.
func TestWhatsAppInboundService_IngestRetryKeepsTheFirstOutcome(t *testing.T) {
	t.Parallel()
	conversations := &fakeWhatsAppInboundConversations{}
	service, _, rfqs, _ := inboundTestService(conversations)
	if err := service.Ingest(context.Background(), inboundTestMessage()); err != nil {
		t.Fatalf("first Ingest() = %v", err)
	}
	conversations.match = &domain.ConversationRoute{QuoteID: uuid.New()}
	if err := service.Ingest(context.Background(), inboundTestMessage()); err != nil {
		t.Fatalf("retried Ingest() = %v", err)
	}
	if len(rfqs.rfqIDs) != 2 || rfqs.rfqIDs[0] != rfqs.rfqIDs[1] {
		t.Errorf("RFQ IDs = %v, want the first reservation resumed", rfqs.rfqIDs)
	}

	joined := &fakeWhatsAppInboundConversations{match: &domain.ConversationRoute{QuoteID: uuid.New()}}
	service, _, rfqs, db := inboundTestService(joined)
	for range 2 {
		if err := service.Ingest(context.Background(), inboundTestMessage()); err != nil {
			t.Fatalf("Ingest() = %v", err)
		}
	}
	if len(db.results) != 2 || db.results[0] != nil || !errors.Is(db.results[1], errInboundAlreadyReserved) {
		t.Errorf("transactions = %v, want the retry's join rolled back", db.results)
	}
	joined.match = nil
	if err := service.Ingest(context.Background(), inboundTestMessage()); err != nil {
		t.Fatalf("Ingest() = %v", err)
	}
	if len(rfqs.inputs) != 0 {
		t.Errorf("RFQ drafts = %d, want none for a message that joined a conversation", len(rfqs.inputs))
	}
}
