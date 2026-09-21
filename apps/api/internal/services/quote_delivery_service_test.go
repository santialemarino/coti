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
	getErr       error
	statusErr    error
	statusCalls  []quoteStatusCall
	appendCalls  int
	appendUserID []*uuid.UUID
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
	return &updated, nil
}

func (f *fakePublicQuoteQuotes) AppendStatusChange(_ context.Context, _ repository.Querier,
	_ uuid.UUID, quoteID uuid.UUID, previous *domain.QuoteStatus, newStatus domain.QuoteStatus,
	userID *uuid.UUID) (*domain.QuoteStatusChange, error) {
	f.appendCalls++
	f.appendUserID = append(f.appendUserID, userID)
	return &domain.QuoteStatusChange{ID: uuid.New(), QuoteID: quoteID,
		PreviousStatus: previous, NewStatus: newStatus, UserID: userID,
		ChangedAt: fixedNow, CreatedAt: fixedNow}, nil
}

func (f *fakePublicQuoteQuotes) GetByID(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, uuid.UUID) (*domain.Quote, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteQuotes) GetByIDForUpdate(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Quote, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (f *fakePublicQuoteQuotes) GetCurrentVersion(context.Context, repository.Querier,
	uuid.UUID, uuid.UUID, uuid.UUID) (*domain.QuoteVersion, error) {
	return nil, errors.New("not used by public quote action tests")
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

func (unusedDeliveryDeps) Create(context.Context, repository.Querier, uuid.UUID,
	domain.NewClient) (*domain.Client, error) {
	return nil, errors.New("not used by public quote action tests")
}

func (unusedDeliveryDeps) UpdateContact(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID, domain.ClientContact) (*domain.Client, error) {
	return nil, errors.New("not used by public quote action tests")
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
	clientActions   *fakePublicQuoteClientActions
	representations *fakeQuoteDeliverRepresentations
	token           string
	now             time.Time
}

func newQuoteActionHarness() *quoteActionHarness {
	h := &quoteActionHarness{
		db:              &fakePublicQuoteDB{},
		sends:           &fakePublicQuoteSends{accountID: testAccountID},
		quotes:          &fakePublicQuoteQuotes{},
		clientActions:   &fakePublicQuoteClientActions{},
		representations: &fakeQuoteDeliverRepresentations{},
		token:           "public-token-quote-action",
		now:             fixedNow,
	}
	h.service = NewQuoteDeliveryService(h.db, h.sends, h.quotes, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, unusedDeliveryDeps{}, unusedDeliveryDeps{}, unusedDeliveryDeps{},
		unusedDeliveryDeps{}, nil, "https://app.coti.ar", func() time.Time { return h.now }, nil).
		WithClientActions(h.clientActions).WithRepresentationService(h.representations)
	return h
}

func (h *quoteActionHarness) seedActiveSend(versionID uuid.UUID) *domain.QuoteSend {
	expiry := h.now.AddDate(0, 0, 7)
	send := &domain.QuoteSend{ID: uuid.New(), VersionID: versionID, PublicToken: h.token,
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

	message := "bajale al precio"
	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
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

func TestQuoteDeliveryService_RespondPublic_RemembersTheAnswerOnAnOutdatedVersion(t *testing.T) {
	h := newQuoteActionHarness()
	oldVersion := uuid.New()
	h.seedActiveSend(oldVersion)
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusSent,
	}

	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if err != nil {
		t.Fatalf("RespondPublic returned %v", err)
	}
	if result.CustomerStatus != domain.ClientActionReject {
		t.Errorf("customer status = %q, want REJECT", result.CustomerStatus)
	}
	if result.QuoteStatus != domain.QuoteStatusSent {
		t.Errorf("quote status = %q, want SENT untouched", result.QuoteStatus)
	}
	if len(h.clientActions.created) != 1 {
		t.Fatalf("client actions created = %d, want 1", len(h.clientActions.created))
	}
	if len(h.quotes.statusCalls) != 0 || h.quotes.appendCalls != 0 {
		t.Errorf("quote moved although the answer addressed an outdated version: "+
			"status updates = %d, appends = %d", len(h.quotes.statusCalls), h.quotes.appendCalls)
	}
}

func TestQuoteDeliveryService_RespondPublic_RemembersTheAnswerOnceTheQuoteLeftSent(t *testing.T) {
	h := newQuoteActionHarness()
	h.seedActiveSend(testVersionID)
	h.quotes.quote = &domain.Quote{
		ID: testQuoteID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID, CurrentStatus: domain.QuoteStatusChangeRequested,
	}

	message := "sumar membrana al presupuesto"
	result, err := h.service.RespondPublic(context.Background(), h.token,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
	if err != nil {
		t.Fatalf("RespondPublic returned %v", err)
	}
	if result.QuoteStatus != domain.QuoteStatusChangeRequested {
		t.Errorf("quote status = %q, want CHANGE_REQUESTED untouched", result.QuoteStatus)
	}
	if len(h.quotes.statusCalls) != 0 || h.quotes.appendCalls != 0 {
		t.Errorf("quote moved although it already left SENT: status updates = %d, appends = %d",
			len(h.quotes.statusCalls), h.quotes.appendCalls)
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
		unusedDeliveryDeps{}, unusedDeliveryDeps{}, unusedDeliveryDeps{}, unusedDeliveryDeps{},
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
