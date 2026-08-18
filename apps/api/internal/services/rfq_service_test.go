package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var (
	testRFQID     = uuid.MustParse("a1111111-1111-4111-8111-111111111111")
	testQuoteID   = uuid.MustParse("a2222222-2222-4222-8222-222222222222")
	testVersionID = uuid.MustParse("a3333333-3333-4333-8333-333333333333")
	testChannelID = uuid.MustParse("a4444444-4444-4444-8444-444444444444")
)

func testRFQConfig() config.RFQConfig {
	return config.RFQConfig{MaxTextCharacters: 200, MaxItems: 3, PipelineTimeout: time.Minute}
}

type fakeRFQDB struct {
	scopes             []uuid.UUID
	activeTransactions int
}

func (f *fakeRFQDB) InTenantTx(
	ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error,
) error {
	// A real pool cannot begin a transaction on a context that is already done, so neither does
	// the fake: it is what makes a write handed an expired deadline observable.
	if err := ctx.Err(); err != nil {
		return err
	}
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}
	f.scopes = append(f.scopes, tenant.AccountID)
	f.activeTransactions++
	defer func() { f.activeTransactions-- }()
	return fn(nil)
}

type fakeRFQExtractor struct {
	lines           []domain.ExtractedRFQLine
	err             error
	calls           int
	raw             string
	db              *fakeRFQDB
	calledOutsideTx bool
}

func (f *fakeRFQExtractor) Extract(
	_ context.Context, raw string,
) ([]domain.ExtractedRFQLine, error) {
	f.calls++
	f.raw = raw
	if f.db != nil {
		f.calledOutsideTx = f.db.activeTransactions == 0
	}
	return f.lines, f.err
}

type fakeCatalogMatcher struct {
	matches         []domain.LineMatch
	err             error
	calls           int
	descriptions    []string
	db              *fakeRFQDB
	calledOutsideTx bool
}

func (f *fakeCatalogMatcher) Match(
	_ context.Context, _ domain.Tenant, descriptions []string,
) ([]domain.LineMatch, error) {
	f.calls++
	f.descriptions = descriptions
	if f.db != nil {
		f.calledOutsideTx = f.db.activeTransactions == 0
	}
	return f.matches, f.err
}

// blockingMatcher waits for its context to end, the way a provider call that outlives the
// pipeline's deadline does.
type blockingMatcher struct{}

func (blockingMatcher) Match(
	ctx context.Context, _ domain.Tenant, _ []string,
) ([]domain.LineMatch, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type fakeRFQChannels struct {
	channel        *domain.Channel
	channelsByType []domain.Channel
	getErr         error
	listErr        error
	getCalls       int
	listCalls      int
}

func (f *fakeRFQChannels) ListActiveByType(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, _ domain.ChannelType,
) ([]domain.Channel, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	channels := make([]domain.Channel, len(f.channelsByType))
	copy(channels, f.channelsByType)
	return channels, nil
}

func (f *fakeRFQChannels) GetActiveByID(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
) (*domain.Channel, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.channel == nil {
		return nil, domain.ErrNotFound
	}
	channel := *f.channel
	return &channel, nil
}

type rfqStatusChangeCall struct {
	rfqID          uuid.UUID
	previousStatus *domain.RFQStatus
	newStatus      domain.RFQStatus
	userID         *uuid.UUID
}

type fakeRFQs struct {
	created       []domain.NewRFQ
	updatedStatus []domain.RFQStatus
	statusChanges []rfqStatusChangeCall
}

func (f *fakeRFQs) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewRFQ,
) (*domain.RFQ, error) {
	f.created = append(f.created, in)
	return &domain.RFQ{
		ID: testRFQID, AccountID: accountID, BranchID: in.BranchID, ClientID: in.ClientID,
		ChannelID: in.ChannelID, RawText: in.RawText, Status: in.Status, WorkType: in.WorkType,
		ClientLabel: in.ClientLabel,
	}, nil
}

func (f *fakeRFQs) UpdateStatus(
	_ context.Context, _ repository.Querier, accountID, id uuid.UUID, status domain.RFQStatus,
) (*domain.RFQ, error) {
	f.updatedStatus = append(f.updatedStatus, status)
	return &domain.RFQ{
		ID: id, AccountID: accountID, BranchID: testBranchID, ChannelID: testChannelID,
		Status: status,
	}, nil
}

func (f *fakeRFQs) AppendStatusChange(
	_ context.Context, _ repository.Querier, accountID, rfqID uuid.UUID,
	previousStatus *domain.RFQStatus, newStatus domain.RFQStatus, userID *uuid.UUID,
) (*domain.RFQStatusChange, error) {
	f.statusChanges = append(f.statusChanges, rfqStatusChangeCall{
		rfqID: rfqID, previousStatus: previousStatus, newStatus: newStatus, userID: userID,
	})
	return &domain.RFQStatusChange{
		AccountID: accountID, RFQID: rfqID, PreviousStatus: previousStatus,
		NewStatus: newStatus, UserID: userID,
	}, nil
}

type quoteStatusChangeCall struct {
	quoteID        uuid.UUID
	previousStatus *domain.QuoteStatus
	newStatus      domain.QuoteStatus
	userID         *uuid.UUID
}

type fakeQuoteDrafts struct {
	created        []domain.NewQuote
	currentVersion []uuid.UUID
	versions       []domain.NewQuoteVersion
	itemBatches    [][]domain.NewQuoteItem
	statusChanges  []quoteStatusChangeCall
}

func (f *fakeQuoteDrafts) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewQuote,
) (*domain.Quote, error) {
	f.created = append(f.created, in)
	return &domain.Quote{
		ID: testQuoteID, AccountID: accountID, BranchID: in.BranchID, ClientID: in.ClientID,
		RFQID: in.RFQID, SellerID: in.SellerID, CurrentStatus: in.CurrentStatus,
		ExpiresAt: in.ExpiresAt,
	}, nil
}

func (f *fakeQuoteDrafts) UpdateCurrentVersion(
	_ context.Context, _ repository.Querier, accountID, quoteID, versionID uuid.UUID,
) (*domain.Quote, error) {
	f.currentVersion = append(f.currentVersion, versionID)
	var sellerID *uuid.UUID
	if len(f.created) > 0 {
		sellerID = f.created[len(f.created)-1].SellerID
	}
	return &domain.Quote{
		ID: quoteID, AccountID: accountID, BranchID: testBranchID, RFQID: testRFQID,
		SellerID: sellerID, CurrentVersionID: &versionID, CurrentStatus: domain.QuoteStatusDraft,
	}, nil
}

func (f *fakeQuoteDrafts) CreateVersion(
	_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewQuoteVersion,
) (*domain.QuoteVersion, error) {
	f.versions = append(f.versions, in)
	return &domain.QuoteVersion{
		ID: testVersionID, AccountID: accountID, QuoteID: in.QuoteID, AuthorID: in.AuthorID,
		VersionNumber: in.VersionNumber, Total: in.Total, IsImmutable: in.IsImmutable,
		Comment: in.Comment,
	}, nil
}

func (f *fakeQuoteDrafts) CreateItems(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	items []domain.NewQuoteItem,
) ([]domain.QuoteItem, error) {
	f.itemBatches = append(f.itemBatches, items)
	created := make([]domain.QuoteItem, 0, len(items))
	for i, item := range items {
		created = append(created, domain.QuoteItem{
			ID: uuid.New(), AccountID: accountID, VersionID: versionID, ProductID: item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity,
			Unit: item.Unit, ConfidenceScore: item.ConfidenceScore, MatchStatus: item.MatchStatus,
			QuantityRationale: item.QuantityRationale, CreatedAt: fixedNow.AddDate(0, 0, i),
		})
	}
	return created, nil
}

func (f *fakeQuoteDrafts) AppendStatusChange(
	_ context.Context, _ repository.Querier, accountID, quoteID uuid.UUID,
	previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID,
) (*domain.QuoteStatusChange, error) {
	f.statusChanges = append(f.statusChanges, quoteStatusChangeCall{
		quoteID: quoteID, previousStatus: previousStatus, newStatus: newStatus, userID: userID,
	})
	return &domain.QuoteStatusChange{
		AccountID: accountID, QuoteID: quoteID, PreviousStatus: previousStatus,
		NewStatus: newStatus, UserID: userID,
	}, nil
}

type rfqHarness struct {
	service   *RFQService
	db        *fakeRFQDB
	extractor *fakeRFQExtractor
	matcher   *fakeCatalogMatcher
	rfqs      *fakeRFQs
	quotes    *fakeQuoteDrafts
	channels  *fakeRFQChannels
}

// newRFQHarness wires the service to fakes, with matching answering MATCHED for every line so a
// test only stages the part it is about.
func newRFQHarness(lines []domain.ExtractedRFQLine) *rfqHarness {
	db := &fakeRFQDB{}
	matches := make([]domain.LineMatch, len(lines))
	for i := range lines {
		productID := testProductID
		matches[i] = domain.LineMatch{
			ProductID:   &productID,
			MatchStatus: domain.ItemMatchStatusMatched,
			Confidence:  decimal.RequireFromString("0.9100"),
		}
	}
	h := &rfqHarness{
		db:        db,
		extractor: &fakeRFQExtractor{lines: lines, db: db},
		matcher:   &fakeCatalogMatcher{matches: matches, db: db},
		rfqs:      &fakeRFQs{},
		quotes:    &fakeQuoteDrafts{},
		channels:  &fakeRFQChannels{},
	}
	channel := domain.Channel{
		ID: testChannelID, AccountID: testAccountID, BranchID: testBranchID,
		Type: domain.ChannelTypeWhatsApp, IsActive: true,
	}
	h.channels.channel = &channel
	h.channels.channelsByType = []domain.Channel{channel}
	h.service = NewRFQService(h.db, h.rfqs, h.quotes, h.channels, h.extractor, h.matcher, nil,
		testRFQConfig())
	return h
}

// rfqTenant carries a branch: the whole pipeline is branch-scoped, and the package's other
// tenant helper deliberately has none.
func rfqTenant() domain.Tenant {
	return domain.Tenant{AccountID: testAccountID, BranchID: testBranchID, UserID: testUserID}
}

func explicitLine(description, quantity, unit, rationale string) domain.ExtractedRFQLine {
	line := domain.ExtractedRFQLine{
		RequestedDescription: description,
		Quantity:             decimal.RequireFromString(quantity),
		Source:               domain.QuantitySourceExplicit,
		QuantityRationale:    rationale,
	}
	if unit != "" {
		line.Unit = &unit
	}
	return line
}

func TestRFQService_CreateTextDraft_PersistsGeneratedDraft(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine(" 10 bolsas de cemento ", "10", " bolsa ", " el cliente pidió 10 bolsas "),
	})
	clientLabel := " Obra Norte "

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{
			ChannelID:   testChannelID,
			ClientLabel: &clientLabel,
			RawText:     "  10 bolsas de cemento  ",
		})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v", err)
	}

	if len(h.rfqs.created) != 1 {
		t.Fatalf("created %d RFQs, want 1", len(h.rfqs.created))
	}
	created := h.rfqs.created[0]
	if created.Status != domain.RFQStatusReceived {
		t.Errorf("RFQ created with status %q, want RECEIVED", created.Status)
	}
	if created.RawText == nil || *created.RawText != "10 bolsas de cemento" {
		t.Errorf("stored raw text %v, want the trimmed order", created.RawText)
	}
	if created.ClientLabel == nil || *created.ClientLabel != "Obra Norte" {
		t.Errorf("stored client label %v, want %q", created.ClientLabel, "Obra Norte")
	}
	if created.BranchID != testBranchID {
		t.Errorf("RFQ branch %s, want the tenant's %s", created.BranchID, testBranchID)
	}

	// The extractor reads what the client wrote, not a normalised copy of it.
	if h.extractor.raw != "10 bolsas de cemento" {
		t.Errorf("extractor read %q, want the stored order", h.extractor.raw)
	}
	if !h.extractor.calledOutsideTx {
		t.Error("the extractor ran inside a transaction; a provider call must not hold one open")
	}
	if !h.matcher.calledOutsideTx {
		t.Error("matching ran inside a transaction; it opens its own")
	}
	if len(h.matcher.descriptions) != 1 || h.matcher.descriptions[0] != "10 bolsas de cemento" {
		t.Errorf("matched %v, want the line's own description", h.matcher.descriptions)
	}

	if len(h.quotes.itemBatches) != 1 || len(h.quotes.itemBatches[0]) != 1 {
		t.Fatalf("persisted item batches %v, want one line", h.quotes.itemBatches)
	}
	item := h.quotes.itemBatches[0][0]
	if item.RequestedDescription != "10 bolsas de cemento" {
		t.Errorf("line description %q, want the client's own words", item.RequestedDescription)
	}
	if !item.Quantity.Equal(decimal.RequireFromString("10")) {
		t.Errorf("line quantity %s, want 10", item.Quantity)
	}
	if item.Unit == nil || *item.Unit != "bolsa" {
		t.Errorf("line unit %v, want %q", item.Unit, "bolsa")
	}
	if item.QuantityRationale == nil || *item.QuantityRationale != "el cliente pidió 10 bolsas" {
		t.Errorf("line rationale %v, want the trimmed explanation", item.QuantityRationale)
	}
	if item.MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("line match status %q, want MATCHED", item.MatchStatus)
	}
	if item.ProductID == nil || *item.ProductID != testProductID {
		t.Errorf("line product %v, want the matched one", item.ProductID)
	}
	if !item.ConfidenceScore.Valid ||
		!item.ConfidenceScore.Decimal.Equal(decimal.RequireFromString("0.9100")) {
		t.Errorf("line confidence %v, want 0.9100", item.ConfidenceScore)
	}

	if len(h.quotes.created) != 1 || h.quotes.created[0].CurrentStatus != domain.QuoteStatusDraft {
		t.Fatalf("created quotes %v, want one DRAFT", h.quotes.created)
	}
	if h.quotes.created[0].SellerID == nil || *h.quotes.created[0].SellerID != testUserID {
		t.Errorf("quote seller %v, want the caller", h.quotes.created[0].SellerID)
	}
	if len(h.quotes.versions) != 1 {
		t.Fatalf("created %d versions, want 1", len(h.quotes.versions))
	}
	version := h.quotes.versions[0]
	if version.VersionNumber != 1 {
		t.Errorf("version number %d, want 1", version.VersionNumber)
	}
	if version.IsImmutable {
		t.Error("version 1 is frozen; the seller has not reviewed it yet")
	}
	if !version.Total.IsZero() {
		t.Errorf("version total %s, want zero: pricing is the next stage", version.Total)
	}
	if len(h.quotes.currentVersion) != 1 || h.quotes.currentVersion[0] != testVersionID {
		t.Errorf("current version pointer %v, want the new version", h.quotes.currentVersion)
	}

	if len(h.rfqs.updatedStatus) != 1 || h.rfqs.updatedStatus[0] != domain.RFQStatusGenerated {
		t.Errorf("RFQ statuses written %v, want one GENERATED", h.rfqs.updatedStatus)
	}
	if len(h.rfqs.statusChanges) != 1 {
		t.Fatalf("appended %d RFQ status changes, want 1", len(h.rfqs.statusChanges))
	}
	change := h.rfqs.statusChanges[0]
	if change.previousStatus == nil || *change.previousStatus != domain.RFQStatusReceived {
		t.Errorf("RFQ change came from %v, want RECEIVED", change.previousStatus)
	}
	if change.newStatus != domain.RFQStatusGenerated {
		t.Errorf("RFQ change went to %q, want GENERATED", change.newStatus)
	}
	if len(h.quotes.statusChanges) != 1 ||
		h.quotes.statusChanges[0].newStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status changes %v, want one into DRAFT", h.quotes.statusChanges)
	}
	if h.quotes.statusChanges[0].previousStatus != nil {
		t.Error("the first quote status change has a previous status; the quote did not exist")
	}

	if draft.Quote == nil || draft.Version == nil || len(draft.Items) != 1 {
		t.Fatalf("draft returned %+v, want the quote, its version and its line", draft)
	}
	if draft.RFQ.Status != domain.RFQStatusGenerated {
		t.Errorf("returned RFQ status %q, want GENERATED", draft.RFQ.Status)
	}
	// One transaction for the order, one for the draft. Anything more means a read that could
	// have travelled with a write.
	if len(h.db.scopes) != 2 {
		t.Errorf("opened %d transactions, want 2", len(h.db.scopes))
	}
	for _, scope := range h.db.scopes {
		if scope != testAccountID {
			t.Errorf("transaction scoped to %s, want the tenant's account", scope)
		}
	}
}

func TestRFQService_CreateTextDraft_StoresTheOrderBeforeReadingIt(t *testing.T) {
	h := newRFQHarness(nil)
	h.extractor.err = errors.New("the model timed out")

	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "10 bolsas de cemento"})
	if err == nil {
		t.Fatal("CreateTextDraft succeeded with a failing extractor")
	}
	// The order survives the failure: without it the client's message is gone and nothing can be
	// retried.
	if len(h.rfqs.created) != 1 {
		t.Fatalf("created %d RFQs, want the order stored before the read", len(h.rfqs.created))
	}
	if len(h.quotes.created) != 0 {
		t.Errorf("created %d quotes, want none", len(h.quotes.created))
	}
	if len(h.rfqs.updatedStatus) != 0 {
		t.Errorf("wrote RFQ statuses %v, want none: it never reached GENERATED",
			h.rfqs.updatedStatus)
	}
}

func TestRFQService_CreateTextDraft_KeepsTheOrderWhenNoMaterialIsRead(t *testing.T) {
	h := newRFQHarness(nil)

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "hola, están abiertos?"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v, want the order kept rather than an error", err)
	}
	if draft.Quote != nil || draft.Version != nil || len(draft.Items) != 0 {
		t.Errorf("draft returned %+v, want the RFQ alone", draft)
	}
	if draft.RFQ.Status != domain.RFQStatusReceived {
		t.Errorf("RFQ status %q, want RECEIVED: generation produced no materials", draft.RFQ.Status)
	}
	if len(h.quotes.created) != 0 || len(h.rfqs.statusChanges) != 0 {
		t.Error("a quote or a transition was written for an order with no materials")
	}
	if h.matcher.calls != 0 {
		t.Errorf("matching ran %d times with nothing to match", h.matcher.calls)
	}
}

func TestRFQService_CreateTextDraft_FlagsEveryLineWhenMatchingCannotAnswer(t *testing.T) {
	cases := []struct {
		name    string
		stage   func(*rfqHarness)
		wantErr bool
	}{
		{
			name:  "matching refuses",
			stage: func(h *rfqHarness) { h.matcher.err = domain.ErrAIUnavailable },
		},
		{
			name: "matching answers for a different number of lines",
			stage: func(h *rfqHarness) {
				h.matcher.matches = []domain.LineMatch{{MatchStatus: domain.ItemMatchStatusMatched}}
			},
		},
		{
			name:  "no matcher is wired",
			stage: func(h *rfqHarness) { h.service.matcher = nil },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRFQHarness([]domain.ExtractedRFQLine{
				explicitLine("cemento", "10", "bolsa", "pidió 10"),
				explicitLine("arena", "2", "m3", "pidió 2"),
			})
			tc.stage(h)

			draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
				domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cemento y arena"})
			if err != nil {
				t.Fatalf("CreateTextDraft returned %v; losing the extraction over a match is worse "+
					"than flagging the lines", err)
			}
			if len(draft.Items) != 2 {
				t.Fatalf("persisted %d lines, want both of them", len(draft.Items))
			}
			for i, item := range h.quotes.itemBatches[0] {
				if item.MatchStatus != domain.ItemMatchStatusNoMatch {
					t.Errorf("line %d status %q, want NO_MATCH", i, item.MatchStatus)
				}
				if item.ProductID != nil {
					t.Errorf("line %d carries product %v, want none", i, item.ProductID)
				}
				// Null rather than zero: nothing scored this line, which is a different fact
				// from a candidate that scored badly.
				if item.ConfidenceScore.Valid {
					t.Errorf("line %d carries confidence %v, want null", i, item.ConfidenceScore)
				}
			}
		})
	}
}

func TestRFQService_CreateTextDraft_KeepsAnUnresolvedQuantityAtZero(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{{
		RequestedDescription: "cemento",
		Quantity:             decimal.Zero,
		Source:               domain.QuantitySourceUnresolved,
		QuantityRationale:    "el cliente no indicó cuántas bolsas",
	}})

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "necesito cemento"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v; an unresolved quantity is a valid answer", err)
	}
	// The material still reaches the quote: the seller types the number, and dropping the line
	// would lose what the client asked for.
	if len(draft.Items) != 1 {
		t.Fatalf("persisted %d lines, want the material kept", len(draft.Items))
	}
	item := h.quotes.itemBatches[0][0]
	if !item.Quantity.IsZero() {
		t.Errorf("line quantity %s, want zero", item.Quantity)
	}
	if item.QuantityRationale == nil || *item.QuantityRationale == "" {
		t.Error("an unresolved line carries no rationale, so nothing says what is missing")
	}
}

func TestRFQService_CreateTextDraft_RejectsAContradictoryLine(t *testing.T) {
	cases := []struct {
		name    string
		line    domain.ExtractedRFQLine
		wantSub string
	}{
		{
			name: "a stated quantity of zero",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.Zero,
				Source: domain.QuantitySourceExplicit, QuantityRationale: "pidió cemento",
			},
			wantSub: "must be positive",
		},
		{
			name: "an unresolved line carrying a number",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.RequireFromString("1"),
				Source: domain.QuantitySourceUnresolved, QuantityRationale: "no dijo cuántas",
			},
			wantSub: "must be zero",
		},
		{
			name: "a source outside the closed set",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.RequireFromString("1"),
				Source: domain.QuantitySource("GUESSED"), QuantityRationale: "estimado",
			},
			wantSub: "is not a known source",
		},
		{
			name: "a negative quantity",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.RequireFromString("-1"),
				Source: domain.QuantitySourceExplicit, QuantityRationale: "pidió cemento",
			},
			wantSub: "must be positive",
		},
		{
			name: "a quantity finer than the column",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.RequireFromString("1.005"),
				Source: domain.QuantitySourceExplicit, QuantityRationale: "pidió 1,005",
			},
			wantSub: "at most 2 decimals",
		},
		{
			name: "no description",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "   ", Quantity: decimal.RequireFromString("1"),
				Source: domain.QuantitySourceExplicit, QuantityRationale: "pidió uno",
			},
			wantSub: "cannot be blank",
		},
		{
			name: "no rationale",
			line: domain.ExtractedRFQLine{
				RequestedDescription: "cemento", Quantity: decimal.RequireFromString("1"),
				Source: domain.QuantitySourceExplicit, QuantityRationale: "  ",
			},
			wantSub: "quantity_rationale cannot be blank",
		},
		{
			name: "a description longer than the column",
			line: domain.ExtractedRFQLine{
				RequestedDescription: strings.Repeat("a", 513),
				Quantity:             decimal.RequireFromString("1"),
				Source:               domain.QuantitySourceExplicit, QuantityRationale: "pidió uno",
			},
			wantSub: "cannot exceed 512 characters",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRFQHarness([]domain.ExtractedRFQLine{tc.line})

			_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
				domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cemento"})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("CreateTextDraft returned %v, want ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
			if len(h.quotes.created) != 0 {
				t.Error("a quote was created from a line the service refused")
			}
		})
	}
}

func TestRFQService_CreateTextDraft_RejectsMoreLinesThanTheCap(t *testing.T) {
	lines := make([]domain.ExtractedRFQLine, testRFQConfig().MaxItems+1)
	for i := range lines {
		lines[i] = explicitLine("cemento", "1", "bolsa", "pidió uno")
	}
	h := newRFQHarness(lines)

	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "una lista larga"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateTextDraft returned %v, want ErrInvalidInput", err)
	}
	if len(h.quotes.created) != 0 {
		t.Error("a quote was created from an order over the line cap")
	}
}

func TestRFQService_CreateTextDraft_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		tenant  domain.Tenant
		in      domain.TextRFQDraftInput
		wantSub string
	}{
		{
			name:    "no active branch",
			tenant:  domain.Tenant{AccountID: testAccountID, UserID: testUserID},
			in:      domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cemento"},
			wantSub: "needs an active branch",
		},
		{
			name:    "no channel",
			tenant:  rfqTenant(),
			in:      domain.TextRFQDraftInput{RawText: "cemento"},
			wantSub: "channel_id is required",
		},
		{
			name:    "a blank order",
			tenant:  rfqTenant(),
			in:      domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "   "},
			wantSub: "raw_text cannot be blank",
		},
		{
			name:   "an order longer than the cap",
			tenant: rfqTenant(),
			in: domain.TextRFQDraftInput{
				ChannelID: testChannelID,
				RawText:   strings.Repeat("a", testRFQConfig().MaxTextCharacters+1),
			},
			wantSub: "raw_text cannot exceed 200 characters",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRFQHarness([]domain.ExtractedRFQLine{
				explicitLine("cemento", "1", "bolsa", "pidió uno"),
			})

			_, err := h.service.CreateTextDraft(context.Background(), tc.tenant, tc.in)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("CreateTextDraft returned %v, want ErrInvalidInput", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
			// Refused before anything was written, and before a model was paid.
			if len(h.rfqs.created) != 0 {
				t.Error("an RFQ was stored for input the service refused")
			}
			if h.extractor.calls != 0 {
				t.Errorf("the extractor ran %d times on refused input", h.extractor.calls)
			}
		})
	}
}

func TestRFQService_CreateTextDraft_RejectsAnUnreachableChannelBeforeReading(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "1", "bolsa", "pidió uno"),
	})
	h.channels.getErr = domain.ErrNotFound

	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cemento"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("CreateTextDraft returned %v, want ErrNotFound", err)
	}
	// A channel of another branch is not this branch's to receive on, and the check is what
	// stops the order being attributed to it.
	if h.extractor.calls != 0 {
		t.Errorf("the extractor ran %d times for an unreachable channel", h.extractor.calls)
	}
}

func TestRFQService_CreateWhatsAppMockDraft_ResolvesTheChannelAndLabelsTheSender(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
	})
	profileName := " Corralón Pueblo "

	draft, err := h.service.CreateWhatsAppMockDraft(context.Background(), rfqTenant(),
		domain.WhatsAppMockRFQInput{
			From: " +5491122334455 ", ProfileName: &profileName, Text: " 10 bolsas de cemento ",
		})
	if err != nil {
		t.Fatalf("CreateWhatsAppMockDraft returned %v", err)
	}
	if h.channels.listCalls != 1 {
		t.Errorf("listed channels %d times, want once", h.channels.listCalls)
	}
	created := h.rfqs.created[0]
	if created.ChannelID != testChannelID {
		t.Errorf("RFQ channel %s, want the branch's WhatsApp channel", created.ChannelID)
	}
	want := "Corralón Pueblo (+5491122334455)"
	if created.ClientLabel == nil || *created.ClientLabel != want {
		t.Errorf("client label %v, want %q", created.ClientLabel, want)
	}
	// Nobody has taken the order from the inbox yet, so it has no seller and no author.
	if h.quotes.created[0].SellerID != nil {
		t.Errorf("quote seller %v, want none on an inbound message",
			h.quotes.created[0].SellerID)
	}
	if h.quotes.versions[0].AuthorID != nil {
		t.Errorf("version author %v, want none on an inbound message",
			h.quotes.versions[0].AuthorID)
	}
	if draft.Quote == nil {
		t.Error("the mock produced no quote; it has to run the same pipeline as the text route")
	}
}

func TestRFQService_CreateWhatsAppMockDraft_RefusesToGuessAmongChannels(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "1", "bolsa", "pidió uno"),
	})
	second := *h.channels.channel
	second.ID = uuid.New()
	h.channels.channelsByType = []domain.Channel{*h.channels.channel, second}

	_, err := h.service.CreateWhatsAppMockDraft(context.Background(), rfqTenant(),
		domain.WhatsAppMockRFQInput{From: "+5491122334455", Text: "cemento"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateWhatsAppMockDraft returned %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "channel_id is required") {
		t.Errorf("error %q does not ask for a channel", err)
	}
}

func TestRFQService_CreateWhatsAppMockDraft_RejectsANonWhatsAppChannel(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "1", "bolsa", "pidió uno"),
	})
	channel := *h.channels.channel
	channel.Type = domain.ChannelTypeManualEntry
	h.channels.channel = &channel
	channelID := channel.ID

	_, err := h.service.CreateWhatsAppMockDraft(context.Background(), rfqTenant(),
		domain.WhatsAppMockRFQInput{
			ChannelID: &channelID, From: "+5491122334455", Text: "cemento",
		})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateWhatsAppMockDraft returned %v, want ErrInvalidInput", err)
	}
	if h.extractor.calls != 0 {
		t.Errorf("the extractor ran %d times for the wrong channel type", h.extractor.calls)
	}
}

func TestRFQService_CreateWhatsAppMockDraft_RequiresASender(t *testing.T) {
	h := newRFQHarness(nil)

	_, err := h.service.CreateWhatsAppMockDraft(context.Background(), rfqTenant(),
		domain.WhatsAppMockRFQInput{From: "  ", Text: "cemento"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateWhatsAppMockDraft returned %v, want ErrInvalidInput", err)
	}
	if h.channels.listCalls != 0 {
		t.Error("a channel was resolved for a message with no sender")
	}
}

func TestRFQService_CreateTextDraft_PersistsTheDraftAfterThePipelineRunsOutOfTime(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
	})
	h.service.matcher = blockingMatcher{}
	h.service.cfg.PipelineTimeout = 20 * time.Millisecond

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "10 bolsas de cemento"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v; the deadline bounds the model, not the writes", err)
	}
	// The extraction was already paid for. Persisting it on the deadline that just expired would
	// throw it away at the last step.
	if len(draft.Items) != 1 {
		t.Fatalf("persisted %d lines, want the extraction kept", len(draft.Items))
	}
	if draft.Items[0].MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Errorf("line status %q, want NO_MATCH: matching never answered",
			draft.Items[0].MatchStatus)
	}
}
