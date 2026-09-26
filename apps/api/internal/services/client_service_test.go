package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeClientProfiles struct {
	clients       map[uuid.UUID]domain.Client
	matches       []domain.Client
	created       *domain.NewClient
	findPhone     *string
	findEmail     *string
	summaries     []domain.ClientSummary
	acceptedSales []domain.ClientSale
}

func (f *fakeClientProfiles) ListSummaries(
	context.Context, repository.Querier, domain.Tenant,
) ([]domain.ClientSummary, error) {
	return f.summaries, nil
}

func (f *fakeClientProfiles) GetByID(_ context.Context, _ repository.Querier, _ uuid.UUID,
	clientID uuid.UUID) (*domain.Client, error) {
	client, ok := f.clients[clientID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &client, nil
}

func (f *fakeClientProfiles) FindByContacts(_ context.Context, _ repository.Querier,
	_ uuid.UUID, phone, email *string) ([]domain.Client, error) {
	f.findPhone, f.findEmail = phone, email
	return f.matches, nil
}

func (f *fakeClientProfiles) ListAcceptedSales(
	context.Context, repository.Querier, domain.Tenant, uuid.UUID,
) ([]domain.ClientSale, error) {
	return f.acceptedSales, nil
}

func (f *fakeClientProfiles) Create(_ context.Context, _ repository.Querier, accountID uuid.UUID,
	in domain.NewClient) (*domain.Client, error) {
	f.created = &in
	client := domain.Client{ID: uuid.New(), AccountID: accountID, Name: in.Name, Phone: in.Phone,
		Email: in.Email, OriginChannel: in.OriginChannel}
	return &client, nil
}

type fakeClientTags struct {
	available  []domain.ClientTag
	byClient   map[uuid.UUID][]domain.ClientTag
	selected   []domain.ClientTag
	created    string
	replacedID uuid.UUID
	replaced   []uuid.UUID
}

func (f *fakeClientTags) List(
	context.Context, repository.Querier, uuid.UUID,
) ([]domain.ClientTag, error) {
	return f.available, nil
}

func (f *fakeClientTags) ListByClientIDs(_ context.Context, _ repository.Querier, _ uuid.UUID,
	clientIDs []uuid.UUID) (map[uuid.UUID][]domain.ClientTag, error) {
	result := make(map[uuid.UUID][]domain.ClientTag, len(clientIDs))
	for _, id := range clientIDs {
		result[id] = f.byClient[id]
	}
	return result, nil
}

func (f *fakeClientTags) GetByIDs(
	context.Context, repository.Querier, uuid.UUID, []uuid.UUID,
) ([]domain.ClientTag, error) {
	return f.selected, nil
}

func (f *fakeClientTags) Create(_ context.Context, _ repository.Querier, accountID uuid.UUID,
	name string) (*domain.ClientTag, error) {
	f.created = name
	return &domain.ClientTag{ID: uuid.New(), AccountID: accountID, Name: name}, nil
}

func (f *fakeClientTags) ReplaceClientTags(_ context.Context, _ repository.Querier, _ uuid.UUID,
	clientID uuid.UUID, tagIDs []uuid.UUID) error {
	f.replacedID = clientID
	f.replaced = append([]uuid.UUID(nil), tagIDs...)
	return nil
}

type fakeClientQuotes struct {
	quote     *domain.Quote
	setClient *uuid.UUID
}

func (f *fakeClientQuotes) GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID,
	uuid.UUID) (*domain.Quote, error) {
	if f.quote == nil {
		return nil, domain.ErrNotFound
	}
	return f.quote, nil
}

func (f *fakeClientQuotes) GetByIDForUpdate(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID) (*domain.Quote, error) {
	if f.quote == nil {
		return nil, domain.ErrNotFound
	}
	return f.quote, nil
}

func (f *fakeClientQuotes) SetClient(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
	clientID uuid.UUID) error {
	f.setClient = &clientID
	return nil
}

type fakeClientRFQs struct {
	setClient *uuid.UUID
}

func (f *fakeClientRFQs) SetClient(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
	clientID uuid.UUID) error {
	f.setClient = &clientID
	return nil
}

type fakeClientSends struct {
	sends []domain.QuoteSend
}

func (f *fakeClientSends) ListByQuote(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID,
) ([]domain.QuoteSend, error) {
	return f.sends, nil
}

type clientServiceHarness struct {
	service *ClientService
	clients *fakeClientProfiles
	tags    *fakeClientTags
	quotes  *fakeClientQuotes
	rfqs    *fakeClientRFQs
	sends   *fakeClientSends
}

func newClientServiceHarness() *clientServiceHarness {
	clientID := uuid.New()
	h := &clientServiceHarness{
		clients: &fakeClientProfiles{clients: map[uuid.UUID]domain.Client{
			clientID: {ID: clientID, AccountID: testAccountID},
		}},
		tags: &fakeClientTags{byClient: make(map[uuid.UUID][]domain.ClientTag)},
		quotes: &fakeClientQuotes{quote: &domain.Quote{ID: testQuoteID, AccountID: testAccountID,
			BranchID: testBranchID, RFQID: testRFQID, CurrentStatus: domain.QuoteStatusAccepted}},
		rfqs:  &fakeClientRFQs{},
		sends: &fakeClientSends{},
	}
	h.service = NewClientService(&fakeDB{}, h.clients, h.tags, h.quotes, h.rfqs, h.sends)
	return h
}

func TestClientService_GetQuoteAssociation_SuggestsWithoutWriting(t *testing.T) {
	h := newClientServiceHarness()
	matchID := uuid.New()
	h.clients.matches = []domain.Client{{ID: matchID, AccountID: testAccountID}}
	h.clients.clients[matchID] = h.clients.matches[0]
	h.sends.sends = []domain.QuoteSend{
		{ChannelType: domain.ChannelTypeEmail, Destination: " Compras@Cliente.test "},
		{ChannelType: domain.ChannelTypeWhatsApp, Destination: "+54 9 11 5555-0000"},
	}

	association, err := h.service.GetQuoteAssociation(context.Background(), branchTenant(),
		testQuoteID)
	if err != nil {
		t.Fatalf("GetQuoteAssociation() = %v, want no error", err)
	}
	if association.CurrentClient != nil || len(association.Suggestions) != 1 ||
		association.Suggestions[0].Client.ID != matchID {
		t.Fatalf("association = %#v, want one suggestion and no current client", association)
	}
	if h.clients.findPhone == nil || *h.clients.findPhone != "+5491155550000" {
		t.Errorf("phone match = %v, want normalized E.164", h.clients.findPhone)
	}
	if h.clients.findEmail == nil || *h.clients.findEmail != "compras@cliente.test" {
		t.Errorf("email match = %v, want normalized lowercase address", h.clients.findEmail)
	}
	if h.quotes.setClient != nil || h.rfqs.setClient != nil || h.tags.replacedID != uuid.Nil {
		t.Fatal("reading suggestions changed the quote, RFQ, or client tags")
	}
}

func TestClientService_AssociateQuote_UpdatesBothRecordsAndTags(t *testing.T) {
	h := newClientServiceHarness()
	clientID := uuid.New()
	tagID := uuid.New()
	h.clients.clients[clientID] = domain.Client{ID: clientID, AccountID: testAccountID}
	h.tags.selected = []domain.ClientTag{{ID: tagID, AccountID: testAccountID, Name: "Recurrente"}}

	match, err := h.service.AssociateQuote(context.Background(), branchTenant(), testQuoteID,
		domain.AssociateQuoteClientInput{ClientID: &clientID,
			TagIDs: []uuid.UUID{tagID, tagID}})
	if err != nil {
		t.Fatalf("AssociateQuote() = %v, want no error", err)
	}
	if match.Client.ID != clientID || len(match.Tags) != 1 {
		t.Fatalf("match = %#v, want selected client and tag", match)
	}
	if h.quotes.setClient == nil || *h.quotes.setClient != clientID ||
		h.rfqs.setClient == nil || *h.rfqs.setClient != clientID {
		t.Fatal("association did not update both quote and RFQ")
	}
	if h.tags.replacedID != clientID || len(h.tags.replaced) != 1 || h.tags.replaced[0] != tagID {
		t.Errorf("tag replacement = %v/%v, want %v/[%v]", h.tags.replacedID,
			h.tags.replaced, clientID, tagID)
	}
}

func TestClientService_AssociateQuote_RefusesNonAcceptedQuote(t *testing.T) {
	h := newClientServiceHarness()
	h.quotes.quote.CurrentStatus = domain.QuoteStatusSent
	clientID := uuid.New()
	h.clients.clients[clientID] = domain.Client{ID: clientID, AccountID: testAccountID}

	_, err := h.service.AssociateQuote(context.Background(), branchTenant(), testQuoteID,
		domain.AssociateQuoteClientInput{ClientID: &clientID})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("AssociateQuote() = %v, want ErrConflict", err)
	}
	if h.quotes.setClient != nil || h.rfqs.setClient != nil {
		t.Fatal("a non-accepted quote was associated")
	}
}

func TestClientService_ClientAssociation_RefusesArchivedQuote(t *testing.T) {
	h := newClientServiceHarness()
	archivedAt := time.Now()
	h.quotes.quote.ArchivedAt = &archivedAt

	_, getErr := h.service.GetQuoteAssociation(context.Background(), branchTenant(), testQuoteID)
	if domain.CodeOf(getErr) != domain.CodeQuoteArchived {
		t.Fatalf("GetQuoteAssociation() error = %v, want QUOTE_ARCHIVED", getErr)
	}

	clientID := uuid.New()
	h.clients.clients[clientID] = domain.Client{ID: clientID, AccountID: testAccountID}
	_, associateErr := h.service.AssociateQuote(context.Background(), branchTenant(), testQuoteID,
		domain.AssociateQuoteClientInput{ClientID: &clientID})
	if domain.CodeOf(associateErr) != domain.CodeQuoteArchived {
		t.Fatalf("AssociateQuote() error = %v, want QUOTE_ARCHIVED", associateErr)
	}
	if h.quotes.setClient != nil || h.rfqs.setClient != nil {
		t.Fatal("an archived quote was associated")
	}
}

func TestClientService_AssociateQuote_NormalizesNewProfile(t *testing.T) {
	h := newClientServiceHarness()
	name := "  Obra San Martin  "
	phone := "+54 9 (11) 5555-0000"
	email := " COMPRAS@CLIENTE.TEST "

	_, err := h.service.AssociateQuote(context.Background(), branchTenant(), testQuoteID,
		domain.AssociateQuoteClientInput{New: &domain.NewClient{Name: &name, Phone: &phone,
			Email: &email}})
	if err != nil {
		t.Fatalf("AssociateQuote() = %v, want no error", err)
	}
	if h.clients.created == nil || *h.clients.created.Name != "Obra San Martin" ||
		*h.clients.created.Phone != "+5491155550000" ||
		*h.clients.created.Email != "compras@cliente.test" {
		t.Fatalf("created client = %#v, want normalized profile", h.clients.created)
	}
	if h.clients.created.OriginChannel == nil ||
		*h.clients.created.OriginChannel != domain.ClientOriginWhatsApp {
		t.Errorf("origin = %v, want WHATSAPP", h.clients.created.OriginChannel)
	}
}

func TestClientService_CreateTag_NormalizesReusableName(t *testing.T) {
	h := newClientServiceHarness()

	tag, err := h.service.CreateTag(context.Background(), branchTenant(), "  Obra   grande  ")
	if err != nil {
		t.Fatalf("CreateTag() = %v, want no error", err)
	}
	if h.tags.created != "Obra grande" || tag.Name != "Obra grande" {
		t.Errorf("created tag = %q/%q, want normalized name", h.tags.created, tag.Name)
	}
}
