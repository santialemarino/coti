package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakePublicQuoteDB struct {
	scopes  []domain.Tenant
	locks   []string
	lockErr error
}

func (f *fakePublicQuoteDB) InTenantTx(_ context.Context, tenant domain.Tenant,
	fn func(repository.Querier) error) error {
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}
	f.scopes = append(f.scopes, tenant)
	return fn(nil)
}

func (f *fakePublicQuoteDB) CrossAccount() repository.Querier { return nil }

func (f *fakePublicQuoteDB) WithAdvisoryLock(_ context.Context, key string,
	fn func() error) error {
	f.locks = append(f.locks, key)
	if f.lockErr != nil {
		return f.lockErr
	}
	return fn()
}

type fakePublicQuoteSends struct {
	accountID uuid.UUID
	send      *domain.QuoteSend
	err       error
	reads     []string
}

func (f *fakePublicQuoteSends) ListByOperation(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.QuoteSend, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteSends) CreateBatch(context.Context, repository.Querier, uuid.UUID,
	[]domain.NewQuoteSend) ([]domain.QuoteSend, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteSends) CompleteBatch(context.Context, repository.Querier, uuid.UUID,
	[]domain.QuoteSendOutcome) error {
	return errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteSends) GetAccountIDByPublicToken(_ context.Context,
	_ repository.Querier, token string) (uuid.UUID, error) {
	f.reads = append(f.reads, token)
	if f.err != nil {
		return uuid.Nil, f.err
	}
	return f.accountID, nil
}

func (f *fakePublicQuoteSends) GetPublicByToken(_ context.Context, _ repository.Querier,
	_ uuid.UUID, token string) (*domain.QuoteSend, error) {
	f.reads = append(f.reads, token)
	if f.err != nil {
		return nil, f.err
	}
	if f.send == nil {
		return nil, domain.ErrNotFound
	}
	return f.send, nil
}

type quoteStatusCall struct {
	from domain.QuoteStatus
	to   domain.QuoteStatus
}

type fakePublicQuoteQuotes struct {
	quote        *domain.Quote
	version      *domain.QuoteVersion
	items        []domain.QuoteItem
	alternatives map[uuid.UUID][]domain.QuoteItemAlternative
	newVersion   *domain.QuoteVersion
	newItems     []domain.NewQuoteItem
	newAlts      []domain.NewQuoteItemAlternative
	itemBatches  [][]domain.NewQuoteItem
	deletedItems []uuid.UUID
	deletedAlts  []uuid.UUID
	quantities   []domain.QuoteItemQuantityUpdate
	replacements []domain.QuoteItemReplacement
	getErr       error
	statusErr    error
	statusCalls  []quoteStatusCall
	appendCalls  int
	appendUserID []*uuid.UUID
	appendFrom   []*domain.QuoteStatus
	appendTo     []domain.QuoteStatus
	locks        int
}

func (f *fakePublicQuoteQuotes) GetByVersionID(_ context.Context, _ repository.Querier,
	_ uuid.UUID, _ uuid.UUID) (*domain.Quote, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.quote == nil {
		return nil, domain.ErrNotFound
	}
	cp := *f.quote
	return &cp, nil
}

func (f *fakePublicQuoteQuotes) UpdateStatus(_ context.Context, _ repository.Querier,
	_ uuid.UUID, _ uuid.UUID, _ uuid.UUID, from, to domain.QuoteStatus) (*domain.Quote, error) {
	f.statusCalls = append(f.statusCalls, quoteStatusCall{from: from, to: to})
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	updated := *f.quote
	updated.CurrentStatus = to
	f.quote.CurrentStatus = to
	return &updated, nil
}

func (f *fakePublicQuoteQuotes) AppendStatusChange(_ context.Context, _ repository.Querier,
	_ uuid.UUID, quoteID uuid.UUID, previous *domain.QuoteStatus, newStatus domain.QuoteStatus,
	userID *uuid.UUID) (*domain.QuoteStatusChange, error) {
	f.appendCalls++
	f.appendUserID = append(f.appendUserID, userID)
	f.appendFrom = append(f.appendFrom, previous)
	f.appendTo = append(f.appendTo, newStatus)
	return &domain.QuoteStatusChange{ID: uuid.New(), QuoteID: quoteID,
		PreviousStatus: previous, NewStatus: newStatus, UserID: userID,
		ChangedAt: fixedNow, CreatedAt: fixedNow}, nil
}

func (f *fakePublicQuoteQuotes) GetByID(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID) (*domain.Quote, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteQuotes) GetByIDForUpdate(_ context.Context, _ repository.Querier,
	_ uuid.UUID, _ uuid.UUID, _ uuid.UUID) (*domain.Quote, error) {
	f.locks++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.quote == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.quote
	return &copy, nil
}

func (f *fakePublicQuoteQuotes) GetCurrentVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteVersion, error) {
	if f.version == nil {
		return nil, domain.ErrNotFound
	}
	copy := *f.version
	return &copy, nil
}

func (f *fakePublicQuoteQuotes) CreateVersion(_ context.Context, _ repository.Querier,
	_ uuid.UUID, in domain.NewQuoteVersion) (*domain.QuoteVersion, error) {
	f.newVersion = &domain.QuoteVersion{ID: uuid.New(), QuoteID: in.QuoteID,
		VersionNumber: in.VersionNumber, Currency: in.Currency, Total: in.Total,
		Comment: in.Comment}
	return f.newVersion, nil
}

func (f *fakePublicQuoteQuotes) UpdateCurrentVersion(_ context.Context, _ repository.Querier,
	_, _, versionID uuid.UUID) (*domain.Quote, error) {
	f.quote.CurrentVersionID = &versionID
	copy := *f.quote
	return &copy, nil
}

func (f *fakePublicQuoteQuotes) ListItems(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID) ([]domain.QuoteItem, error) {
	return f.items, nil
}

func (f *fakePublicQuoteQuotes) CreateItems(_ context.Context, _ repository.Querier,
	_, _ uuid.UUID, items []domain.NewQuoteItem) ([]domain.QuoteItem, error) {
	copy := append([]domain.NewQuoteItem(nil), items...)
	f.itemBatches = append(f.itemBatches, copy)
	f.newItems = append(f.newItems, items...)
	return make([]domain.QuoteItem, len(items)), nil
}

func (f *fakePublicQuoteQuotes) ListAlternativesByItemIDs(context.Context, repository.Querier,
	uuid.UUID, []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	return f.alternatives, nil
}

func (f *fakePublicQuoteQuotes) CreateAlternatives(_ context.Context, _ repository.Querier,
	_ uuid.UUID, alternatives []domain.NewQuoteItemAlternative) error {
	f.newAlts = alternatives
	return nil
}

func (f *fakePublicQuoteQuotes) DeleteAlternativesByItemIDs(_ context.Context, _ repository.Querier,
	_ uuid.UUID, _ uuid.UUID, itemIDs []uuid.UUID,
) error {
	f.deletedAlts = append(f.deletedAlts, itemIDs...)
	return nil
}

func (f *fakePublicQuoteQuotes) DeleteItems(_ context.Context, _ repository.Querier, _ uuid.UUID,
	_ uuid.UUID, itemIDs []uuid.UUID,
) error {
	f.deletedItems = append(f.deletedItems, itemIDs...)
	return nil
}

func (f *fakePublicQuoteQuotes) UpdateItemQuantities(_ context.Context, _ repository.Querier,
	_ uuid.UUID, _ uuid.UUID, updates []domain.QuoteItemQuantityUpdate,
) error {
	f.quantities = append(f.quantities, updates...)
	return nil
}

func (f *fakePublicQuoteQuotes) ReplaceItems(_ context.Context, _ repository.Querier, _ uuid.UUID,
	_ uuid.UUID, replacements []domain.QuoteItemReplacement,
) error {
	f.replacements = append(f.replacements, replacements...)
	return nil
}

func (f *fakePublicQuoteQuotes) FreezeVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteVersion, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteQuotes) SetClient(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID, uuid.UUID) error {
	return errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteQuotes) SetExpiry(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID, time.Time) error {
	return errors.New("not used by public quote action tests")
}

type fakePublicQuoteClientActions struct {
	existing  *domain.ClientAction
	err       error
	createErr error
	created   []domain.NewClientAction
	reads     []uuid.UUID
}

type fakeQuoteMessages struct {
	quoteID   uuid.UUID
	channelID uuid.UUID
	actionID  uuid.UUID
	body      string
	calls     int
	err       error
}

type fakeChangeRequestHandler struct {
	proposal *domain.ChangeRequestProposal
	err      error
	inputs   []domain.ChangeRequestInput
}

func (f *fakeChangeRequestHandler) Propose(_ context.Context,
	in domain.ChangeRequestInput,
) (*domain.ChangeRequestProposal, error) {
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return nil, f.err
	}
	return f.proposal, nil
}

type fakeChangeRequestMatcher struct {
	matches []domain.LineMatch
	err     error
	texts   [][]string
}

func (f *fakeChangeRequestMatcher) Match(_ context.Context, _ domain.Tenant,
	descriptions []string,
) ([]domain.LineMatch, error) {
	f.texts = append(f.texts, append([]string(nil), descriptions...))
	if f.err != nil {
		return nil, f.err
	}
	return f.matches, nil
}

type fakeHandlerDecisions struct {
	created []domain.NewHandlerDecision
	err     error
}

func (f *fakeHandlerDecisions) Create(_ context.Context, _ repository.Querier, _ uuid.UUID,
	in domain.NewHandlerDecision,
) error {
	f.created = append(f.created, in)
	return f.err
}

func (f *fakeQuoteMessages) CreateClientRequest(_ context.Context, _ repository.Querier,
	_, _, quoteID, channelID, actionID uuid.UUID, body string) error {
	f.calls++
	f.quoteID = quoteID
	f.channelID = channelID
	f.actionID = actionID
	f.body = body
	return f.err
}

func (f *fakePublicQuoteClientActions) GetBySend(_ context.Context, _ repository.Querier,
	_ uuid.UUID, sendID uuid.UUID) (*domain.ClientAction, error) {
	f.reads = append(f.reads, sendID)
	if f.err != nil {
		return nil, f.err
	}
	if f.existing != nil {
		return f.existing, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakePublicQuoteClientActions) Create(_ context.Context, _ repository.Querier,
	_ uuid.UUID, in domain.NewClientAction) (*domain.ClientAction, error) {
	f.created = append(f.created, in)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &domain.ClientAction{ID: in.ID, QuoteSendID: in.QuoteSendID, VersionID: in.VersionID,
		Type: in.Type, Comment: in.Comment, CreatedAt: fixedNow}, nil
}

type fakeQuoteDeliverRepresentations struct {
	resolve   *domain.PublicQuoteRepresentation
	err       error
	calls     int
	versionID uuid.UUID
	expiresAt time.Time
	publicURL string
}

func (f *fakeQuoteDeliverRepresentations) Ensure(context.Context, domain.Tenant,
	uuid.UUID) (*domain.QuoteRepresentationResult, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakeQuoteDeliverRepresentations) ResolvePublic(_ context.Context, _ uuid.UUID,
	versionID uuid.UUID, expiresAt time.Time, publicURL string,
) (*domain.PublicQuoteRepresentation, error) {
	f.calls++
	f.versionID = versionID
	f.expiresAt = expiresAt
	f.publicURL = publicURL
	if f.err != nil {
		return nil, f.err
	}
	if f.resolve == nil {
		return nil, domain.ErrNotFound
	}
	cp := *f.resolve
	return &cp, nil
}

type unusedDeliveryDeps struct{}

func (unusedDeliveryDeps) SetClient(context.Context, repository.Querier, uuid.UUID, uuid.UUID,
	uuid.UUID, uuid.UUID) error {
	return errors.New("not used by public quote action tests")
}

func (unusedDeliveryDeps) ListActiveByBranch(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID) ([]domain.Channel, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (unusedDeliveryDeps) GetByID(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID) (*domain.Branch, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (unusedDeliveryDeps) Send(context.Context, OutboundMail) error {
	return errors.New("not used by public quote action tests")
}

func (unusedDeliveryDeps) SendQuote(context.Context,
	domain.QuoteWhatsAppMessage) (*domain.DeliveryReceipt, error) {
	return nil, errors.New("not used by public quote action tests")
}

type quoteActionHarness struct {
	service         *QuoteDeliveryService
	db              *fakePublicQuoteDB
	sends           *fakePublicQuoteSends
	quotes          *fakePublicQuoteQuotes
	prices          *fakeBranchPrices
	clientActions   *fakePublicQuoteClientActions
	messages        *fakeQuoteMessages
	representations *fakeQuoteDeliverRepresentations
	token           string
	now             time.Time
}

func newQuoteActionHarness() *quoteActionHarness {
	h := &quoteActionHarness{
		db:              &fakePublicQuoteDB{},
		sends:           &fakePublicQuoteSends{accountID: testAccountID},
		quotes:          &fakePublicQuoteQuotes{},
		prices:          &fakeBranchPrices{},
		clientActions:   &fakePublicQuoteClientActions{},
		messages:        &fakeQuoteMessages{},
		representations: &fakeQuoteDeliverRepresentations{},
		token:           "public-token-quote-action",
		now:             fixedNow,
	}
	h.service = NewQuoteDeliveryService(h.db, h.sends, h.quotes, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, h.prices, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, nil, "https://app.coti.ar", func() time.Time { return h.now }, nil).
		WithClientActions(h.clientActions, nil).WithRepresentationService(h.representations).
		WithMessages(h.messages)
	return h
}

func (h *quoteActionHarness) seedActiveSend(versionID uuid.UUID) *domain.QuoteSend {
	expiry := h.now.AddDate(0, 0, 7)
	send := &domain.QuoteSend{ID: uuid.New(), ChannelID: uuid.New(),
		VersionID: versionID, PublicToken: h.token,
		ExpiresAt: &expiry, TrackingStatus: domain.SendTrackingStatusViewed}
	h.sends.send = send
	return send
}

func TestQuoteDeliveryService_RespondPublic_RecordsTheAnswerAndClosesTheSentVersion(t *testing.T) {
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent,
	}

	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if err != nil {
		t.Fatalf("RespondPublic returned %v", err)
	}
	if result == nil {
		t.Fatal("RespondPublic returned a nil result")
	}
	if result.CustomerStatus != domain.ClientActionAccept {
		t.Errorf("customer status = %q, want ACCEPT", result.CustomerStatus)
	}
	if result.QuoteStatus != domain.QuoteStatusAccepted {
		t.Errorf("quote status = %q, want ACCEPTED", result.QuoteStatus)
	}
	if !result.CreatedAt.Equal(fixedNow) {
		t.Errorf("created at = %v, want %v", result.CreatedAt, fixedNow)
	}
	if len(h.clientActions.created) != 1 {
		t.Fatalf("client actions created = %d, want 1", len(h.clientActions.created))
	}
	created := h.clientActions.created[0]
	if created.Type != domain.ClientActionAccept {
		t.Errorf("created type = %q, want ACCEPT", created.Type)
	}
	if created.QuoteSendID == nil || *created.QuoteSendID != send.ID {
		t.Errorf("created send = %v, want %v", created.QuoteSendID, send.ID)
	}
	if created.Comment != nil {
		t.Errorf("created comment = %q, want nil", *created.Comment)
	}
	if len(h.quotes.statusCalls) != 1 {
		t.Fatalf("status updates = %d, want 1", len(h.quotes.statusCalls))
	}
	if h.quotes.statusCalls[0].from != domain.QuoteStatusSent ||
		h.quotes.statusCalls[0].to != domain.QuoteStatusAccepted {
		t.Errorf("status update = %+v, want SENT to ACCEPTED", h.quotes.statusCalls[0])
	}
	if h.quotes.appendCalls != 1 {
		t.Fatalf("status changes appended = %d, want 1", h.quotes.appendCalls)
	}
	if h.quotes.appendFrom[0] == nil || *h.quotes.appendFrom[0] != domain.QuoteStatusSent ||
		h.quotes.appendTo[0] != domain.QuoteStatusAccepted {
		t.Errorf("status history = %v -> %s, want SENT -> ACCEPTED",
			h.quotes.appendFrom[0], h.quotes.appendTo[0])
	}
	if h.quotes.locks != 1 {
		t.Errorf("quote row locks = %d, want 1", h.quotes.locks)
	}
	if len(h.quotes.appendUserID) != 1 || h.quotes.appendUserID[0] != nil {
		t.Errorf("status change user = %v, want a system transition with nil user",
			h.quotes.appendUserID)
	}
	if len(h.db.locks) != 1 || !strings.Contains(h.db.locks[0], "quote-action") ||
		!strings.Contains(h.db.locks[0], testAccountID.String()) ||
		!strings.Contains(h.db.locks[0], h.token) {
		t.Errorf("advisory lock = %v, want quote-action:account:token", h.db.locks)
	}
}

func TestQuoteDeliveryService_RespondPublic_CreatesAReviewableChangeRequest(t *testing.T) {
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.quotes.quote = &domain.Quote{ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent}
	h.quotes.version = &domain.QuoteVersion{ID: testVersionID, QuoteID: testQuoteID,
		VersionNumber: 1, Currency: "ARS", IsImmutable: true}
	oldItemID := uuid.New()
	productID := uuid.New()
	h.quotes.items = []domain.QuoteItem{{ID: oldItemID, ProductID: &productID,
		RequestedDescription: "cemento", Quantity: decimal.NewFromInt(2),
		MatchStatus: domain.ItemMatchStatusMatched}}
	h.quotes.alternatives = map[uuid.UUID][]domain.QuoteItemAlternative{
		oldItemID: {{ProductID: &productID, Type: domain.QuoteItemAlternativeTypeProduct,
			Origin: domain.QuoteItemAlternativeOriginSeller, Rank: 1,
			ApprovedBySeller: true}},
	}
	h.prices.prices = map[uuid.UUID]domain.BranchPrice{
		productID: {ProductID: productID, Price: decimal.NewFromInt(65), Currency: "ARS"},
	}
	message := "sumar una bolsa de cemento"
	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
	if err != nil {
		t.Fatalf("RespondPublic() = %v", err)
	}
	if result.QuoteStatus != domain.QuoteStatusChangeRequested {
		t.Errorf("status = %s, want CHANGE_REQUESTED", result.QuoteStatus)
	}
	if h.quotes.newVersion == nil || h.quotes.newVersion.VersionNumber != 2 ||
		h.quotes.newVersion.IsImmutable ||
		!h.quotes.newVersion.Total.Equal(decimal.NewFromInt(130)) {
		t.Errorf("new version = %+v, want a priced, mutable v2 draft", h.quotes.newVersion)
	}
	if h.quotes.quote.CurrentVersionID == nil ||
		*h.quotes.quote.CurrentVersionID != h.quotes.newVersion.ID {
		t.Errorf("current version = %v, want v2", h.quotes.quote.CurrentVersionID)
	}
	if len(h.quotes.newItems) != 1 || h.quotes.newItems[0].ID == oldItemID ||
		h.quotes.newItems[0].Quantity.Cmp(decimal.NewFromInt(2)) != 0 ||
		!h.quotes.newItems[0].UnitPriceSnapshot.Valid ||
		!h.quotes.newItems[0].UnitPriceSnapshot.Decimal.Equal(decimal.NewFromInt(65)) ||
		!h.quotes.newItems[0].Subtotal.Decimal.Equal(decimal.NewFromInt(130)) {
		t.Errorf("cloned items = %+v, want repriced copy with fresh identity", h.quotes.newItems)
	}
	if len(h.quotes.newAlts) != 1 ||
		h.quotes.newAlts[0].QuoteItemID != h.quotes.newItems[0].ID ||
		h.quotes.newAlts[0].ProductID == nil ||
		*h.quotes.newAlts[0].ProductID != productID ||
		!h.quotes.newAlts[0].PriceSnapshot.Valid ||
		!h.quotes.newAlts[0].PriceSnapshot.Decimal.Equal(decimal.NewFromInt(65)) {
		t.Errorf("cloned alternatives = %+v, want repriced copy", h.quotes.newAlts)
	}
	if h.prices.calls != 1 || len(h.prices.askedFor[0]) != 1 ||
		h.prices.askedFor[0][0] != productID || h.prices.branches[0] != testBranchID {
		t.Errorf("price lookup = %+v, want one batch in the quote branch", h.prices)
	}
	if h.messages.calls != 1 || h.messages.quoteID != testQuoteID ||
		h.messages.channelID != send.ChannelID || h.messages.body != message ||
		h.messages.actionID != h.clientActions.created[0].ID {
		t.Errorf("linked message = %+v, want the customer request", h.messages)
	}
	if h.quotes.appendCalls != 1 || h.quotes.appendFrom[0] == nil ||
		*h.quotes.appendFrom[0] != domain.QuoteStatusSent ||
		h.quotes.appendTo[0] != domain.QuoteStatusChangeRequested {
		t.Errorf("status history = %v -> %v, want SENT -> CHANGE_REQUESTED",
			h.quotes.appendFrom, h.quotes.appendTo)
	}
}

func TestQuoteDeliveryService_RespondPublic_AppliesTheHandlerProposalToTheDraft(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	h.quotes.quote = &domain.Quote{ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent}
	h.quotes.version = &domain.QuoteVersion{ID: testVersionID, QuoteID: testQuoteID,
		VersionNumber: 1, Currency: "ARS", IsImmutable: true}
	oldItemID := uuid.New()
	oldProductID := uuid.New()
	h.quotes.items = []domain.QuoteItem{{ID: oldItemID, ProductID: &oldProductID,
		RequestedDescription: "bolsas de cemento", Quantity: decimal.NewFromInt(10),
		MatchStatus: domain.ItemMatchStatusMatched}}
	h.prices.prices = map[uuid.UUID]domain.BranchPrice{
		oldProductID: {ProductID: oldProductID, Price: decimal.NewFromInt(9256), Currency: "ARS"},
	}
	calProductID := uuid.New()
	quantity := decimal.NewFromInt(5)
	description := "bolsas de cal hidratada 25 kg"
	unit := "bolsa"
	proposal := &fakeChangeRequestHandler{proposal: &domain.ChangeRequestProposal{
		Interpretation: "El cliente pidió sumar cinco bolsas de cal hidratada.",
		Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationAddItem,
			RequestedDescription: &description, Quantity: &quantity, Unit: &unit}},
	}}
	matcher := &fakeChangeRequestMatcher{matches: []domain.LineMatch{{ProductID: &calProductID,
		MatchStatus: domain.ItemMatchStatusMatched, Confidence: decimal.RequireFromString("0.93")}}}
	decisions := &fakeHandlerDecisions{}
	h.service.WithChangeRequestHandler(proposal, matcher, decisions, 10, time.Second)
	message := "Agregar 5 bolsas de Cal hidratada 25 kg"

	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
	if err != nil {
		t.Fatalf("RespondPublic returned %v", err)
	}
	if result.QuoteStatus != domain.QuoteStatusChangeRequested {
		t.Errorf("status = %q, want CHANGE_REQUESTED", result.QuoteStatus)
	}
	if len(proposal.inputs) != 1 || proposal.inputs[0].Message != message ||
		len(proposal.inputs[0].Items) != 1 || proposal.inputs[0].Items[0].ID != oldItemID {
		t.Errorf("handler input = %+v, want the message and sent item", proposal.inputs)
	}
	if len(matcher.texts) != 1 || len(matcher.texts[0]) != 1 || matcher.texts[0][0] != description {
		t.Errorf("matcher inputs = %v, want the added material", matcher.texts)
	}
	if len(h.quotes.itemBatches) != 2 {
		t.Fatalf("item batches = %d, want cloned items plus handler additions", len(h.quotes.itemBatches))
	}
	added := h.quotes.itemBatches[1]
	if len(added) != 1 || added[0].RequestedDescription != description ||
		!added[0].Quantity.Equal(quantity) || added[0].Unit == nil || *added[0].Unit != unit ||
		added[0].ProductID == nil || *added[0].ProductID != calProductID ||
		added[0].MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("added items = %+v, want matched cal in the mutable draft", added)
	}
	if len(decisions.created) != 1 || decisions.created[0].QuoteVersionID != h.quotes.newVersion.ID ||
		decisions.created[0].ClientInput != message ||
		decisions.created[0].StateAtDecision != domain.QuoteStatusSent {
		t.Errorf("handler decisions = %+v, want the unreviewed proposal", decisions.created)
	}
}

func TestValidateChangeRequestProposal_RefusesMultipleMutationsOfTheSameLine(t *testing.T) {
	itemID := uuid.New()
	quantity := decimal.NewFromInt(4)
	proposal := &domain.ChangeRequestProposal{Interpretation: "El cliente pidió cambios.",
		Operations: []domain.ChangeRequestOperation{
			{Type: domain.ChangeRequestOperationRemoveItem, TargetItemID: &itemID},
			{Type: domain.ChangeRequestOperationUpdateQuantity, TargetItemID: &itemID, Quantity: &quantity},
		}}

	_, err := validateChangeRequestProposal(proposal, []domain.QuoteItem{{ID: itemID}}, 10)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("validateChangeRequestProposal returned %v, want ErrInvalidInput", err)
	}
}

func TestQuoteDeliveryService_RespondPublic_ReplaysItsFirstAnswer(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	h.clientActions.existing = &domain.ClientAction{
		ID: uuid.New(), VersionID: testVersionID, Type: domain.ClientActionAccept,
		CreatedAt: fixedNow,
	}
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusAccepted,
	}

	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if err != nil {
		t.Fatalf("RespondPublic returned %v", err)
	}
	if result.CustomerStatus != domain.ClientActionAccept {
		t.Errorf("customer status = %q, want the stored ACCEPT", result.CustomerStatus)
	}
	if result.QuoteStatus != domain.QuoteStatusAccepted {
		t.Errorf("quote status = %q, want ACCEPTED", result.QuoteStatus)
	}
	if len(h.clientActions.created) != 0 {
		t.Errorf("client actions created = %d, want 0 for a replay", len(h.clientActions.created))
	}
	if len(h.quotes.statusCalls) != 0 {
		t.Errorf("status updates = %d, want 0 for a replay", len(h.quotes.statusCalls))
	}
	if h.quotes.appendCalls != 0 {
		t.Errorf("status changes appended = %d, want 0 for a replay", h.quotes.appendCalls)
	}
}

func TestQuoteDeliveryService_RespondPublic_RejectsAnOutdatedVersion(t *testing.T) {
	h := newQuoteActionHarness()
	oldVersion := uuid.New()
	h.seedActiveSend(oldVersion)
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent,
	}

	_, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("outdated version response = %v, want ErrConflict", err)
	}
	if len(h.clientActions.created) != 0 {
		t.Fatalf("client actions created = %d, want 0", len(h.clientActions.created))
	}
	if len(h.quotes.statusCalls) != 0 || h.quotes.appendCalls != 0 {
		t.Errorf("quote moved although the answer addressed an outdated version: "+
			"status updates = %d, appends = %d", len(h.quotes.statusCalls), h.quotes.appendCalls)
	}
}

func TestQuoteDeliveryService_RespondPublic_RejectsAnAlreadyClosedQuote(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusAccepted,
	}

	_, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("closed quote response = %v, want ErrConflict", err)
	}
	if len(h.clientActions.created) != 0 || len(h.quotes.statusCalls) != 0 ||
		h.quotes.appendCalls != 0 {
		t.Errorf("closed quote wrote action or status: actions = %d, updates = %d, appends = %d",
			len(h.clientActions.created), len(h.quotes.statusCalls), h.quotes.appendCalls)
	}
}

func TestQuoteDeliveryService_RespondPublic_RejectsAnArchivedQuote(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	archivedAt := h.now.Add(-time.Hour)
	h.quotes.quote = &domain.Quote{ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent,
		ArchivedAt: &archivedAt}

	_, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("archived quote response = %v, want ErrConflict", err)
	}
	if len(h.clientActions.created) != 0 {
		t.Errorf("client actions created = %d, want 0", len(h.clientActions.created))
	}
}

func TestQuoteDeliveryService_RespondPublic_RequiresAnUnExpiredSend(t *testing.T) {
	h := newQuoteActionHarness()
	expired := h.now.Add(-time.Hour)
	h.sends.send = &domain.QuoteSend{ID: uuid.New(), VersionID: testVersionID,
		PublicToken: h.token, ExpiresAt: &expired}

	_, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("RespondPublic on an expired link = %v, want ErrConflict", err)
	}
	if len(h.clientActions.created) != 0 {
		t.Errorf("client actions created = %d, want 0", len(h.clientActions.created))
	}
}

func TestQuoteDeliveryService_RespondPublic_BlankOrUnknownTokenIsNotFound(t *testing.T) {
	h := newQuoteActionHarness()

	_, err := h.service.RespondPublic(context.Background(), "   ",
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("blank token RespondPublic = %v, want ErrNotFound", err)
	}
	if len(h.sends.reads) != 0 {
		t.Errorf("a blank token was looked up %d times", len(h.sends.reads))
	}

	h.sends.err = domain.ErrNotFound
	_, err = h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown token RespondPublic = %v, want ErrNotFound", err)
	}
}

func TestQuoteDeliveryService_RespondPublic_RequiresWiredActions(t *testing.T) {
	h := newQuoteActionHarness()
	service := NewQuoteDeliveryService(h.db, h.sends, h.quotes, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, h.prices, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, nil, "https://app.coti.ar", func() time.Time { return h.now }, nil)

	_, err := service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("RespondPublic without wired actions = %v, want ErrNotConfigured", err)
	}
}

func TestQuoteDeliveryService_ResolvePublic_AttachesTheCustomerAnswer(t *testing.T) {
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.clientActions.existing = &domain.ClientAction{
		ID: uuid.New(), VersionID: testVersionID, Type: domain.ClientActionAccept,
		CreatedAt: fixedNow,
	}
	h.representations.resolve = &domain.PublicQuoteRepresentation{
		Status: "ACTIVE", ExpiresAt: *send.ExpiresAt,
	}

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if result.CustomerStatus == nil || *result.CustomerStatus != domain.ClientActionAccept {
		t.Errorf("customer status = %v, want ACCEPT", result.CustomerStatus)
	}
	if len(h.clientActions.reads) != 1 || h.clientActions.reads[0] != send.ID {
		t.Errorf("customer status reads = %v, want [%v]", h.clientActions.reads, send.ID)
	}
	if h.representations.versionID != testVersionID {
		t.Errorf("resolved version = %v, want %v", h.representations.versionID, testVersionID)
	}
	if !strings.Contains(h.representations.publicURL, h.token) {
		t.Errorf("public URL = %q, want it to carry the token", h.representations.publicURL)
	}
}

func TestQuoteDeliveryService_ResolvePublic_LeavesTheAnswerAbsentBeforeItArrives(t *testing.T) {
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.representations.resolve = &domain.PublicQuoteRepresentation{
		Status: "ACTIVE", ExpiresAt: *send.ExpiresAt,
	}

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if result.CustomerStatus != nil {
		t.Errorf("customer status = %v, want nil before any answer", result.CustomerStatus)
	}
}

func TestQuoteDeliveryService_ResolvePublic_ExpiredLinkIsExpired(t *testing.T) {
	h := newQuoteActionHarness()
	expired := h.now.Add(-time.Hour)
	h.sends.send = &domain.QuoteSend{ID: uuid.New(), VersionID: testVersionID,
		PublicToken: h.token, ExpiresAt: &expired}

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if result.Status != "EXPIRED" {
		t.Errorf("status = %q, want EXPIRED", result.Status)
	}
	if h.representations.calls != 0 {
		t.Errorf("representation resolved %d times for an expired link", h.representations.calls)
	}
}

func TestNormalizeClientAction(t *testing.T) {
	longComment := strings.Repeat("x", maxClientActionComment)
	tooLongComment := strings.Repeat("x", maxClientActionComment+1)

	tests := []struct {
		name    string
		input   domain.ClientActionInput
		wantNil bool
		wantErr bool
	}{
		{name: "accept without a comment", input: domain.ClientActionInput{Type: domain.ClientActionAccept}},
		{name: "accept with a blank comment", input: domain.ClientActionInput{
			Type: domain.ClientActionAccept, Message: strPtr("   ")}, wantNil: true},
		{name: "accept with an oversized comment", input: domain.ClientActionInput{
			Type: domain.ClientActionAccept, Message: strPtr(tooLongComment)}, wantErr: true},
		{name: "request change needs a message", input: domain.ClientActionInput{
			Type: domain.ClientActionRequestChange}, wantErr: true},
		{name: "request change with a maximum message", input: domain.ClientActionInput{
			Type: domain.ClientActionRequestChange, Message: strPtr(longComment)}},
		{name: "request change counts accented characters", input: domain.ClientActionInput{
			Type:    domain.ClientActionRequestChange,
			Message: strPtr(strings.Repeat("á", maxClientActionComment))}},
		{name: "request change with an oversized message", input: domain.ClientActionInput{
			Type: domain.ClientActionRequestChange, Message: strPtr(tooLongComment)}, wantErr: true},
		{name: "reject without a comment", input: domain.ClientActionInput{Type: domain.ClientActionReject}},
		{name: "comment cannot be submitted", input: domain.ClientActionInput{
			Type: domain.ClientActionComment}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.input
			err := normalizeClientAction(&in)
			if tt.wantErr != (err != nil) {
				t.Fatalf("normalizeClientAction = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantNil && in.Message != nil {
				t.Errorf("message = %q, want nil", *in.Message)
			}
		})
	}
}

func TestStatusForClientAction(t *testing.T) {
	tests := []struct {
		action domain.ClientActionType
		want   domain.QuoteStatus
		ok     bool
	}{
		{action: domain.ClientActionAccept, want: domain.QuoteStatusAccepted, ok: true},
		{action: domain.ClientActionReject, want: domain.QuoteStatusRejected, ok: true},
		{action: domain.ClientActionRequestChange, want: domain.QuoteStatusChangeRequested, ok: true},
		{action: domain.ClientActionComment, want: "", ok: false},
	}
	for _, tt := range tests {
		got, ok := statusForClientAction(tt.action)
		if got != tt.want || ok != tt.ok {
			t.Errorf("statusForClientAction(%q) = %q, %v; want %q, %v", tt.action, got, ok,
				tt.want, tt.ok)
		}
	}
}
