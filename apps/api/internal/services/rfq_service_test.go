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

var manualChannelID = uuid.MustParse("66666666-6666-4666-8666-666666666666")
var knownProduct = uuid.MustParse("77777777-7777-4777-8777-777777777777")
var foreignProduct = uuid.MustParse("88888888-8888-4888-8888-888888888888")

var foreignSellerID = uuid.MustParse("99999999-9999-4999-8999-999999999999")

type fakeRfqRepoManual struct {
	channelID  uuid.UUID
	channelErr error
	owned      int
	creation   *domain.RfqCreation
	createErr  error
	receivedAt time.Time
	created    []domain.NewRfq
	listItems  []domain.RfqListItem
}

func (f *fakeRfqRepoManual) Create(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ domain.NewRFQ,
) (*domain.RFQ, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) UpdateStatus(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, _ domain.RFQStatus,
) (*domain.RFQ, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) AppendStatusChange(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, _ *domain.RFQStatus, _ domain.RFQStatus, _ *uuid.UUID,
) (*domain.RFQStatusChange, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) ListStatusChanges(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
) ([]domain.RFQStatusChange, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) ListByTenant(
	_ context.Context, _ repository.Querier, _ domain.Tenant,
) ([]domain.RfqListItem, error) {
	return f.listItems, nil
}

func (f *fakeRfqRepoManual) GetByRFQID(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID,
) (*domain.RfqListItem, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) AssignSeller(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID,
) (*domain.Quote, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) SetSeller(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID, _ *uuid.UUID,
) (*domain.Quote, error) {
	return nil, errors.New("not implemented in manual fake")
}

func (f *fakeRfqRepoManual) GetManualEntryChannelID(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (uuid.UUID, error) {
	if f.channelErr != nil {
		return uuid.Nil, f.channelErr
	}
	return f.channelID, nil
}

func (f *fakeRfqRepoManual) CountProductsInAccount(
	_ context.Context, _ repository.Querier, _ uuid.UUID, productIDs []uuid.UUID,
) (int, error) {
	if len(productIDs) == 0 {
		return 0, nil
	}
	return f.owned, nil
}

func (f *fakeRfqRepoManual) CreateManualEntry(
	_ context.Context, _ repository.Querier, tenant domain.Tenant, channelID uuid.UUID,
	in domain.NewRfq, now time.Time,
) (*domain.RfqCreation, error) {
	f.created = append(f.created, in)
	f.receivedAt = now
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.creation != nil {
		return f.creation, nil
	}
	return &domain.RfqCreation{
		Rfq: domain.RFQ{
			ID: uuid.New(), BranchID: tenant.BranchID, ChannelID: channelID,
			RawText: in.RawText, WorkType: in.WorkType, ClientLabel: in.ClientLabel,
			Status: domain.RFQStatusGenerated, ReceivedAt: now,
		},
		Quote: domain.Quote{
			ID: uuid.New(), BranchID: tenant.BranchID, SellerID: &tenant.UserID,
			CurrentStatus: domain.QuoteStatusDraft,
		},
	}, nil
}

// fakeSellerReach answers the seller-assignment check the manual entry runs before naming a
// seller, letting a test steer both the happy path and the rejection. It also records every
// check, so a test can assert which branches and users the service asked about.
type fakeSellerReach struct {
	serves     bool
	err        error
	calls      int
	branchSets [][]uuid.UUID
	userIDs    []uuid.UUID
}

// SellerServesBranches reports the configured verdict.
func (f *fakeSellerReach) SellerServesBranches(
	_ context.Context, _ repository.Querier, _ uuid.UUID,
	branchIDs []uuid.UUID, userID uuid.UUID,
) (bool, error) {
	f.calls++
	f.branchSets = append(f.branchSets, branchIDs)
	f.userIDs = append(f.userIDs, userID)
	if f.err != nil {
		return false, f.err
	}
	return f.serves, nil
}

func manualHarness(repo *fakeRfqRepoManual) (*RFQService, *fakeDB) {
	db := &fakeDB{}
	svc := NewRFQService(db, repo, nil, nil, nil, nil, &fakeSellerReach{serves: true},
		nil, nil, nil, config.RFQConfig{})
	svc.now = func() time.Time { return fixedNow }
	return svc, db
}

func manualItems() []domain.NewRfqItem {
	return []domain.NewRfqItem{{
		ProductID:            &knownProduct,
		RequestedDescription: "Cemento Loma Negra x50",
		Quantity:             decimal.RequireFromString("2.5"),
		Unit:                 strPtr("bolsa"),
	}}
}

func strPtr(s string) *string { return &s }

func TestRfqService_CreateManual_BornGeneratedAndDraft(t *testing.T) {
	raw := strPtr("  pedido de hoy  ")
	repo := &fakeRfqRepoManual{channelID: manualChannelID, owned: 1}
	svc, db := manualHarness(repo)

	creation, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{
		RawText:     raw,
		ClientLabel: strPtr("  Pérez  "),
		Items:       manualItems(),
	})
	if err != nil {
		t.Fatalf("CreateManual returned an unexpected error: %v", err)
	}
	if creation.Rfq.Status != domain.RFQStatusGenerated {
		t.Errorf("RFQ status = %s, want GENERATED", creation.Rfq.Status)
	}
	if creation.Quote.CurrentStatus != domain.QuoteStatusDraft {
		t.Errorf("quote status = %s, want DRAFT", creation.Quote.CurrentStatus)
	}
	if !creation.Rfq.ReceivedAt.Equal(fixedNow) {
		t.Errorf("received_at = %v, want the injected clock %v", creation.Rfq.ReceivedAt, fixedNow)
	}
	if creation.Quote.SellerID == nil || *creation.Quote.SellerID != testUserID {
		t.Errorf("quote seller_id = %v, want the caller %v", creation.Quote.SellerID, testUserID)
	}
	if creation.Rfq.ChannelID != manualChannelID {
		t.Errorf("rfq channel = %v, want the manual-entry channel %v", creation.Rfq.ChannelID, manualChannelID)
	}

	if got := *creation.Rfq.ClientLabel; got != "Pérez" {
		t.Errorf("client label was not trimmed: %q", got)
	}
	if got := *creation.Rfq.RawText; got != "pedido de hoy" {
		t.Errorf("raw_text was not trimmed: %q", got)
	}
	if len(repo.created) != 1 {
		t.Fatalf("CreateManualEntry called %d times, want 1", len(repo.created))
	}
	item := repo.created[0].Items[0]
	if item.Quantity.String() != "2.5" {
		t.Errorf("quantity = %s, want 2.5", item.Quantity)
	}
	if item.Unit == nil || *item.Unit != "bolsa" {
		t.Errorf("unit = %v, want bolsa", item.Unit)
	}
	if item.ProductID == nil || *item.ProductID != knownProduct {
		t.Errorf("product_id = %v, want the known product", item.ProductID)
	}

	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("transaction scoped to %v, want [%v]", db.scopes, testAccountID)
	}
}

func TestRfqService_CreateManual_NeedsAnActiveBranch(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID}
	svc, _ := manualHarness(repo)
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	_, err := svc.CreateManual(context.Background(), tenant, domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestRfqService_CreateManual_NeedsTextOrItems(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID}
	svc, _ := manualHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if got := len(repo.created); got != 0 {
		t.Fatalf("CreateManualEntry called %d times, want 0", got)
	}
}

func TestRfqService_CreateManual_RejectsBadItems(t *testing.T) {
	cases := []struct {
		name string
		item domain.NewRfqItem
	}{
		{"empty description", domain.NewRfqItem{
			RequestedDescription: "   ", Quantity: decimal.NewFromInt(1),
		}},
		{"zero quantity", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.Zero,
		}},
		{"negative quantity", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.NewFromInt(-3),
		}},
		{"too many decimals", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.RequireFromString("2.500"),
		}},
		{"over moneyMax", domain.NewRfqItem{
			RequestedDescription: "Cemento", Quantity: decimal.RequireFromString("999999999999.99").Add(decimal.NewFromInt(1)),
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRfqRepoManual{channelID: manualChannelID}
			svc, _ := manualHarness(repo)

			_, err := svc.CreateManual(context.Background(), branchTenant(),
				domain.NewRfq{Items: []domain.NewRfqItem{tc.item}})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRfqService_CreateManual_ProductOutsideAccountFailsClosed(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID, owned: 1}
	svc, _ := manualHarness(repo)

	items := append(manualItems(), domain.NewRfqItem{
		ProductID: &foreignProduct, RequestedDescription: "Ajeno", Quantity: decimal.NewFromInt(1),
	})
	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: items})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := len(repo.created); got != 0 {
		t.Fatalf("CreateManualEntry called %d times, want 0", got)
	}
}

func TestRfqService_CreateManual_MissingChannelIsPropagated(t *testing.T) {
	repo := &fakeRfqRepoManual{channelErr: domain.ErrNotFound}
	svc, _ := manualHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRfqService_CreateManual_RepositoryErrorRollsBack(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID, owned: 1, createErr: domain.ErrConflict}
	svc, _ := manualHarness(repo)

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{Items: manualItems()})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestRfqService_CreateManual_SellerOutsideReachIsRejected(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID, owned: 1}
	db := &fakeDB{}
	svc := NewRFQService(db, repo, nil, nil, nil, nil, &fakeSellerReach{serves: false},
		nil, nil, nil, config.RFQConfig{})

	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{
		SellerID: &foreignSellerID,
		Items:    manualItems(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if got := len(repo.created); got != 0 {
		t.Fatalf("CreateManualEntry called %d times, want 0", got)
	}
}

func TestRfqService_CreateManual_NamedSellerIsPersisted(t *testing.T) {
	repo := &fakeRfqRepoManual{channelID: manualChannelID, owned: 1}
	db := &fakeDB{}
	svc := NewRFQService(db, repo, nil, nil, nil, nil, &fakeSellerReach{serves: true},
		nil, nil, nil, config.RFQConfig{})

	caller := testUserID
	named := &caller
	_, err := svc.CreateManual(context.Background(), branchTenant(), domain.NewRfq{
		SellerID: named,
		Items:    manualItems(),
	})
	if err != nil {
		t.Fatalf("CreateManual returned an unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("CreateManualEntry called %d times, want 1", len(repo.created))
	}
	if got := repo.created[0].SellerID; got == nil || *got != testUserID {
		t.Errorf("recorded seller_id = %v, want %v", got, testUserID)
	}
}

// ---------- AI pipeline fakes & tests ----------

var (
	testRFQID     = uuid.MustParse("a1111111-1111-4111-8111-111111111111")
	testQuoteID   = uuid.MustParse("a2222222-2222-4222-8222-222222222222")
	testVersionID = uuid.MustParse("a3333333-3333-4333-8333-333333333333")
	testChannelID = uuid.MustParse("a4444444-4444-4444-8444-444444444444")
)

func testRFQConfig() config.RFQConfig {
	return config.RFQConfig{
		MaxTextCharacters: 200, MaxItems: 3, MaxSpreadsheetRows: 50, PipelineTimeout: time.Minute,
	}
}

type fakeRFQDB struct {
	scopes             []uuid.UUID
	activeTransactions int
}

func (f *fakeRFQDB) InTenantTx(
	ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error,
) error {
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
	blocks          []domain.Content
	db              *fakeRFQDB
	calledOutsideTx bool
}

func (f *fakeRFQExtractor) Extract(
	_ context.Context, raw string,
) (*domain.RFQExtraction, error) {
	f.calls++
	f.raw = raw
	if f.db != nil {
		f.calledOutsideTx = f.db.activeTransactions == 0
	}
	if f.err != nil {
		return nil, f.err
	}
	return &domain.RFQExtraction{
		Lines: f.lines,
		Usage: domain.GenerationUsage{
			Provider: "test-provider", Model: "test-model", InputTokens: 21, OutputTokens: 13,
			CacheReadTokens: 8, CacheWriteTokens: 5,
		},
		PromptVersion: "test-prompt-v1", SchemaVersion: "test-schema-v1",
	}, nil
}

func (f *fakeRFQExtractor) ExtractFromContent(
	ctx context.Context, blocks []domain.Content, _ []domain.RFQInterpretationExample,
) (*domain.RFQExtraction, error) {
	f.blocks = blocks
	// The text half of the blocks stands in for the order, so a caller asserting on `raw`
	// reads the same thing whichever intake produced it.
	var raw []string
	for _, block := range blocks {
		if block.Kind == domain.ContentKindText {
			raw = append(raw, block.Text)
		}
	}
	return f.Extract(ctx, strings.Join(raw, "\n"))
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
	created          []domain.NewRFQ
	updatedStatus    []domain.RFQStatus
	statusChanges    []rfqStatusChangeCall
	statusHistory    []domain.RFQStatusChange
	statusHistoryErr error
	rfqByID          *domain.RfqListItem
	rfqByIDErr       error
	assigned         *domain.Quote
	assignErr        error
	set              *domain.Quote
	setErr           error
	setSellerIDs     []*uuid.UUID
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

func (f *fakeRFQs) ListStatusChanges(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
) ([]domain.RFQStatusChange, error) {
	if f.statusHistoryErr != nil {
		return nil, f.statusHistoryErr
	}
	return f.statusHistory, nil
}

func (f *fakeRFQs) ListByTenant(
	_ context.Context, _ repository.Querier, _ domain.Tenant,
) ([]domain.RfqListItem, error) {
	return nil, errors.New("not implemented in pipeline fake")
}

func (f *fakeRFQs) GetByRFQID(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID,
) (*domain.RfqListItem, error) {
	if f.rfqByIDErr != nil {
		return nil, f.rfqByIDErr
	}
	return f.rfqByID, nil
}

func (f *fakeRFQs) AssignSeller(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID,
) (*domain.Quote, error) {
	if f.assignErr != nil {
		return nil, f.assignErr
	}
	return f.assigned, nil
}

func (f *fakeRFQs) SetSeller(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID, sellerID *uuid.UUID,
) (*domain.Quote, error) {
	f.setSellerIDs = append(f.setSellerIDs, sellerID)
	if f.setErr != nil {
		return nil, f.setErr
	}
	return f.set, nil
}

func (f *fakeRFQs) GetManualEntryChannelID(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (uuid.UUID, error) {
	return uuid.Nil, errors.New("not implemented in pipeline fake")
}

func (f *fakeRFQs) CountProductsInAccount(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ []uuid.UUID,
) (int, error) {
	return 0, errors.New("not implemented in pipeline fake")
}

func (f *fakeRFQs) CreateManualEntry(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ uuid.UUID,
	_ domain.NewRfq, _ time.Time,
) (*domain.RfqCreation, error) {
	return nil, errors.New("not implemented in pipeline fake")
}

type quoteStatusChangeCall struct {
	quoteID        uuid.UUID
	previousStatus *domain.QuoteStatus
	newStatus      domain.QuoteStatus
	userID         *uuid.UUID
}

type fakeQuoteDrafts struct {
	created                []domain.NewQuote
	currentVersion         []uuid.UUID
	versions               []domain.NewQuoteVersion
	itemBatches            [][]domain.NewQuoteItem
	alternativeBatches     [][]domain.NewQuoteItemAlternative
	alternativeReads       [][]uuid.UUID
	storedAlternatives     []domain.QuoteItemAlternative
	alternativesErr        error
	statusChanges          []quoteStatusChangeCall
	quoteByID              *domain.Quote
	quoteByIDErr           error
	currentVersionData     *domain.QuoteVersion
	currentVersionErr      error
	currentVersionBranches []uuid.UUID
	itemsByVersionID       []domain.QuoteItem
	itemsByVersionErr      error
	quoteStatusHistory     []domain.QuoteStatusChange
	statusHistoryErr       error
	previousVersionData    *domain.QuoteVersion
	previousVersionErr     error
	frozenItems            []domain.QuoteItem
	getPreviousCalls       int
	versionTotals          []decimal.Decimal
}

type fakeQuoteAIGenerations struct {
	created []domain.NewQuoteAIGeneration
	items   [][]domain.NewQuoteAIGenerationItem
	err     error
}

// fakeQuoteDiscounts is the in-memory quote_discount surface for service tests.
type fakeQuoteDiscounts struct {
	stored   []domain.QuoteDiscount
	links    map[uuid.UUID][]uuid.UUID // discountID -> itemIDs
	nextErr  error
	updateIn []domain.QuoteDiscountUpdate
	createIn []domain.QuoteDiscountCreate
}

func (f *fakeQuoteDiscounts) ListByVersionID(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteDiscount, error) {
	if f.nextErr != nil {
		return nil, f.nextErr
	}
	var out []domain.QuoteDiscount
	for _, d := range f.stored {
		if d.AccountID == accountID && d.QuoteVersionID == versionID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeQuoteDiscounts) Create(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	in domain.QuoteDiscountCreate,
) (*domain.QuoteDiscount, error) {
	f.createIn = append(f.createIn, in)
	discount := domain.QuoteDiscount{
		ID: uuid.New(), AccountID: accountID, QuoteVersionID: versionID,
		ConditionType: *conditionTypeFor(in.Scope), Scope: in.Scope,
		Origin:      domain.DiscountOriginManualSeller,
		Amount:      in.Amount,
		ActionType:  in.ActionType,
		ActionValue: &in.Value,
		Description: &in.Description,
		CreatedAt:   fixedNow,
	}
	f.stored = append(f.stored, discount)
	return &discount, nil
}

func (f *fakeQuoteDiscounts) GetByID(
	_ context.Context, _ repository.Querier, accountID, versionID, discountID uuid.UUID,
) (*domain.QuoteDiscount, error) {
	for i := range f.stored {
		d := &f.stored[i]
		if d.ID == discountID && d.AccountID == accountID && d.QuoteVersionID == versionID {
			return d, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeQuoteDiscounts) UpdateByID(
	_ context.Context, _ repository.Querier, accountID, versionID, discountID uuid.UUID,
	in domain.QuoteDiscountUpdate,
) (*domain.QuoteDiscount, error) {
	f.updateIn = append(f.updateIn, in)
	d, err := f.GetByID(context.Background(), nil, accountID, versionID, discountID)
	if err != nil {
		return nil, err
	}
	if in.Value != nil {
		d.ActionValue = in.Value
	}
	if in.ActionType != nil {
		d.ActionType = *in.ActionType
	}
	if in.Scope != nil {
		d.Scope = *in.Scope
	}
	if in.ConditionType != nil {
		d.ConditionType = *in.ConditionType
	}
	if in.Description != nil {
		d.Description = in.Description
	}
	if in.SuppressedBySeller != nil {
		d.SuppressedBySeller = *in.SuppressedBySeller
	}
	return d, nil
}

func (f *fakeQuoteDiscounts) DeleteByID(
	_ context.Context, _ repository.Querier, accountID, versionID, discountID uuid.UUID,
) error {
	for i, d := range f.stored {
		if d.ID == discountID && d.AccountID == accountID && d.QuoteVersionID == versionID {
			f.stored = append(f.stored[:i], f.stored[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeQuoteDiscounts) UpdateAmount(
	_ context.Context, _ repository.Querier, accountID, versionID, discountID uuid.UUID,
	amount decimal.Decimal,
) error {
	d, err := f.GetByID(context.Background(), nil, accountID, versionID, discountID)
	if err != nil {
		return err
	}
	d.Amount = amount
	return nil
}

func (f *fakeQuoteDiscounts) CreateItemLinks(
	_ context.Context, _ repository.Querier, accountID, versionID, discountID uuid.UUID,
	itemIDs []uuid.UUID,
) error {
	if f.links == nil {
		f.links = make(map[uuid.UUID][]uuid.UUID)
	}
	f.links[discountID] = append([]uuid.UUID{}, itemIDs...)
	return nil
}

func (f *fakeQuoteDiscounts) ReplaceItemLinks(
	ctx context.Context, q repository.Querier, accountID, versionID, discountID uuid.UUID,
	itemIDs []uuid.UUID,
) error {
	return f.CreateItemLinks(ctx, q, accountID, versionID, discountID, itemIDs)
}

func (f *fakeQuoteDiscounts) ListItemIDs(
	_ context.Context, _ repository.Querier, accountID, discountID uuid.UUID,
) ([]uuid.UUID, error) {
	for _, d := range f.stored {
		if d.ID == discountID && d.AccountID == accountID {
			return f.links[discountID], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeQuoteDiscounts) ListItemIDsByDiscountIDs(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	discountIDs []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	linked := make(map[uuid.UUID][]uuid.UUID, len(discountIDs))
	for _, discountID := range discountIDs {
		if f.links[discountID] != nil {
			linked[discountID] = f.links[discountID]
		}
	}
	return linked, nil
}

func (f *fakeQuoteAIGenerations) Create(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
	in domain.NewQuoteAIGeneration, items []domain.NewQuoteAIGenerationItem,
) (*domain.QuoteAIGeneration, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, in)
	f.items = append(f.items, items)
	return &domain.QuoteAIGeneration{
		ID: uuid.New(), AccountID: accountID, QuoteID: in.QuoteID,
		QuoteVersionID: in.QuoteVersionID, Provider: in.Provider, Model: in.Model,
		PromptVersion: in.PromptVersion, SchemaVersion: in.SchemaVersion,
		InputTokens: in.InputTokens, OutputTokens: in.OutputTokens,
		CacheReadTokens: in.CacheReadTokens, CacheWriteTokens: in.CacheWriteTokens,
	}, nil
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

func (f *fakeQuoteDrafts) UpdateVersionTotal(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID, total decimal.Decimal,
) (*domain.QuoteVersion, error) {
	f.versionTotals = append(f.versionTotals, total)
	return &domain.QuoteVersion{ID: testVersionID, Total: total}, nil
}

func (f *fakeQuoteDrafts) CreateItems(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	items []domain.NewQuoteItem,
) ([]domain.QuoteItem, error) {
	f.itemBatches = append(f.itemBatches, items)
	created := make([]domain.QuoteItem, 0, len(items))
	for i, item := range items {
		created = append(created, domain.QuoteItem{
			ID: item.ID, AccountID: accountID, VersionID: versionID, ProductID: item.ProductID,
			RequestedDescription: item.RequestedDescription, Quantity: item.Quantity,
			Unit: item.Unit, ConfidenceScore: item.ConfidenceScore, MatchStatus: item.MatchStatus,
			QuantityRationale: item.QuantityRationale, CreatedAt: fixedNow.AddDate(0, 0, i),
		})
	}
	return created, nil
}

func (f *fakeQuoteDrafts) CreateAlternatives(
	_ context.Context, _ repository.Querier, accountID uuid.UUID,
	alternatives []domain.NewQuoteItemAlternative,
) error {
	f.alternativeBatches = append(f.alternativeBatches, alternatives)
	if f.alternativesErr != nil {
		return f.alternativesErr
	}
	for _, alternative := range alternatives {
		f.storedAlternatives = append(f.storedAlternatives, domain.QuoteItemAlternative{
			ID: uuid.New(), AccountID: accountID, QuoteItemID: alternative.QuoteItemID,
			ProductID: alternative.ProductID, ComboID: alternative.ComboID,
			Type: alternative.Type, Origin: alternative.Origin, Rank: alternative.Rank,
			ConfidenceScore: alternative.ConfidenceScore, PriceSnapshot: alternative.PriceSnapshot,
		})
	}
	return nil
}

func (f *fakeQuoteDrafts) ListAlternativesByItemIDs(
	_ context.Context, _ repository.Querier, _ uuid.UUID, itemIDs []uuid.UUID,
) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	f.alternativeReads = append(f.alternativeReads, itemIDs)
	asked := make(map[uuid.UUID]struct{}, len(itemIDs))
	for _, itemID := range itemIDs {
		asked[itemID] = struct{}{}
	}
	byItem := make(map[uuid.UUID][]domain.QuoteItemAlternative)
	for _, alternative := range f.storedAlternatives {
		if _, ok := asked[alternative.QuoteItemID]; !ok {
			continue
		}
		byItem[alternative.QuoteItemID] = append(byItem[alternative.QuoteItemID], alternative)
	}
	return byItem, nil
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

func (f *fakeQuoteDrafts) ListStatusChanges(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID,
) ([]domain.QuoteStatusChange, error) {
	if f.statusHistoryErr != nil {
		return nil, f.statusHistoryErr
	}
	return f.quoteStatusHistory, nil
}

func (f *fakeQuoteDrafts) GetByRFQID(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID,
) (*domain.Quote, error) {
	if f.quoteByIDErr != nil {
		return nil, f.quoteByIDErr
	}
	return f.quoteByID, nil
}

func (f *fakeQuoteDrafts) GetByID(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID,
) (*domain.Quote, error) {
	if f.quoteByIDErr != nil {
		return nil, f.quoteByIDErr
	}
	return f.quoteByID, nil
}

func (f *fakeQuoteDrafts) GetCurrentVersion(
	_ context.Context, _ repository.Querier, _, branchID, _ uuid.UUID,
) (*domain.QuoteVersion, error) {
	f.currentVersionBranches = append(f.currentVersionBranches, branchID)
	if f.currentVersionErr != nil {
		return nil, f.currentVersionErr
	}
	return f.currentVersionData, nil
}

func (f *fakeQuoteDrafts) ListItems(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID,
) ([]domain.QuoteItem, error) {
	if f.itemsByVersionErr != nil {
		return nil, f.itemsByVersionErr
	}
	return f.itemsByVersionID, nil
}

func (f *fakeQuoteDrafts) GetPreviousVersion(
	_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, _ int,
) (*domain.QuoteVersion, error) {
	f.getPreviousCalls++
	if f.previousVersionErr != nil {
		return nil, f.previousVersionErr
	}
	return f.previousVersionData, nil
}

func (f *fakeQuoteDrafts) ListItemsWithProduct(
	ctx context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	if f.previousVersionData != nil && versionID == f.previousVersionData.ID {
		return f.frozenItems, nil
	}
	return f.ListItems(ctx, nil, accountID, versionID)
}

func (f *fakeQuoteDrafts) GetItem(
	_ context.Context, _ repository.Querier, accountID, versionID, itemID uuid.UUID,
) (*domain.QuoteItem, error) {
	for _, item := range f.itemsByVersionID {
		if item.ID == itemID {
			return &item, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeQuoteDrafts) UpdateItem(
	_ context.Context, _ repository.Querier, accountID, versionID, itemID uuid.UUID,
	in domain.QuoteItemUpdate,
) (*domain.QuoteItem, error) {
	item, err := f.GetItem(context.Background(), nil, accountID, versionID, itemID)
	if err != nil {
		return nil, err
	}
	if in.ProductID != nil {
		item.ProductID = in.ProductID
	}
	if in.RequestedDescription != nil {
		item.RequestedDescription = *in.RequestedDescription
	}
	if in.Quantity != nil {
		item.Quantity = *in.Quantity
	}
	if in.Unit != nil {
		item.Unit = in.Unit
	}
	return item, nil
}

func (f *fakeQuoteDrafts) DeleteItem(
	_ context.Context, _ repository.Querier, accountID, versionID, itemID uuid.UUID,
) error {
	for i, item := range f.itemsByVersionID {
		if item.ID == itemID {
			f.itemsByVersionID = append(f.itemsByVersionID[:i], f.itemsByVersionID[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeQuoteDrafts) CreateSingleItem(
	_ context.Context, _ repository.Querier, accountID, versionID uuid.UUID,
	in domain.QuoteItemCreate,
) (*domain.QuoteItem, error) {
	item := domain.QuoteItem{
		ID: uuid.New(), AccountID: accountID, VersionID: versionID,
		ProductID: in.ProductID, RequestedDescription: in.RequestedDescription,
		Quantity: in.Quantity, Unit: in.Unit,
		MatchStatus: domain.ItemMatchStatusNoMatch,
	}
	if in.ProductID != nil {
		item.MatchStatus = domain.ItemMatchStatusMatched
	}
	f.itemsByVersionID = append(f.itemsByVersionID, item)
	return &item, nil
}

type fakeQuoteSends struct {
	deliveries []domain.QuoteSend
	err        error
	reads      []uuid.UUID
}

func (f *fakeQuoteSends) ListByQuote(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ uuid.UUID, quoteID uuid.UUID,
) ([]domain.QuoteSend, error) {
	f.reads = append(f.reads, quoteID)
	if f.err != nil {
		return nil, f.err
	}
	return f.deliveries, nil
}

type rfqHarness struct {
	service     *RFQService
	db          *fakeRFQDB
	extractor   *fakeRFQExtractor
	matcher     *fakeCatalogMatcher
	rfqs        *fakeRFQs
	quotes      *fakeQuoteDrafts
	discounts   *fakeQuoteDiscounts
	sends       *fakeQuoteSends
	generations *fakeQuoteAIGenerations
	channels    *fakeRFQChannels
}

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
		db:          db,
		extractor:   &fakeRFQExtractor{lines: lines, db: db},
		matcher:     &fakeCatalogMatcher{matches: matches, db: db},
		rfqs:        &fakeRFQs{},
		quotes:      &fakeQuoteDrafts{},
		discounts:   &fakeQuoteDiscounts{},
		sends:       &fakeQuoteSends{},
		generations: &fakeQuoteAIGenerations{},
		channels:    &fakeRFQChannels{},
	}
	channel := domain.Channel{
		ID: testChannelID, AccountID: testAccountID, BranchID: testBranchID,
		Type: domain.ChannelTypeWhatsApp, IsActive: true,
	}
	h.channels.channel = &channel
	h.channels.channelsByType = []domain.Channel{channel}
	h.service = NewRFQService(h.db, h.rfqs, h.quotes, h.sends, h.generations, h.channels,
		&fakeSellerReach{serves: true}, h.extractor, h.matcher, nil, testRFQConfig()).
		WithDiscounts(h.discounts)
	return h
}

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

// scoredCandidate stages one offer the matcher weighed. Distance is what confidenceOf reads, but
// these tests stage the confidence directly: what a candidate scored is the matcher's own test.
func scoredCandidate(productID uuid.UUID, name, confidence string) domain.ScoredCandidate {
	return domain.ScoredCandidate{
		CatalogCandidate: domain.CatalogCandidate{ProductID: productID, CanonicalName: name},
		Confidence:       decimal.RequireFromString(confidence),
	}
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

	if h.extractor.raw != "10 bolsas de cemento" {
		t.Errorf("extractor read %q, want the stored order", h.extractor.raw)
	}
	if !h.extractor.calledOutsideTx {
		t.Error("the extractor ran inside a transaction")
	}
	if !h.matcher.calledOutsideTx {
		t.Error("matching ran inside a transaction")
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
		t.Error("version 1 is frozen")
	}
	if !version.Total.IsZero() {
		t.Errorf("version total %s, want zero", version.Total)
	}
	if len(h.quotes.currentVersion) != 1 || h.quotes.currentVersion[0] != testVersionID {
		t.Errorf("current version pointer %v, want the new version", h.quotes.currentVersion)
	}
	if len(h.generations.created) != 1 || len(h.generations.items) != 1 ||
		len(h.generations.items[0]) != 1 {
		t.Fatalf("AI generation writes = %v / %v, want one generation with one item",
			h.generations.created, h.generations.items)
	}
	generation := h.generations.created[0]
	if generation.QuoteID != testQuoteID || generation.QuoteVersionID != testVersionID {
		t.Errorf("AI generation targets quote/version %s/%s, want %s/%s", generation.QuoteID,
			generation.QuoteVersionID, testQuoteID, testVersionID)
	}
	if generation.Provider != "test-provider" || generation.Model != "test-model" ||
		generation.PromptVersion != "test-prompt-v1" ||
		generation.SchemaVersion != "test-schema-v1" {
		t.Errorf("AI generation identity = %+v, want the extractor metadata", generation)
	}
	if generation.InputTokens != 21 || generation.OutputTokens != 13 ||
		generation.CacheReadTokens != 8 || generation.CacheWriteTokens != 5 {
		t.Errorf("AI generation usage = %+v, want 21/13/8/5", generation)
	}
	generatedItem := h.generations.items[0][0]
	if generatedItem.Position != 0 ||
		generatedItem.QuantitySource != domain.QuantitySourceExplicit ||
		generatedItem.ProductID == nil || *generatedItem.ProductID != testProductID ||
		generatedItem.MatchStatus != domain.ItemMatchStatusMatched ||
		!generatedItem.ConfidenceScore.Decimal.Equal(decimal.RequireFromString("0.9100")) {
		t.Errorf("AI generation item = %+v, want the original matched proposal", generatedItem)
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
		t.Error("the first quote status change has a previous status")
	}

	if draft.Quote == nil || draft.Version == nil || len(draft.Items) != 1 {
		t.Fatalf("draft returned %+v, want the quote, its version and its line", draft)
	}
	if draft.RFQ.Status != domain.RFQStatusGenerated {
		t.Errorf("returned RFQ status %q, want GENERATED", draft.RFQ.Status)
	}
	if len(h.db.scopes) != 2 {
		t.Errorf("opened %d transactions, want 2", len(h.db.scopes))
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
	if len(h.rfqs.created) != 1 {
		t.Fatalf("created %d RFQs, want the order stored before the read", len(h.rfqs.created))
	}
	if len(h.quotes.created) != 0 {
		t.Errorf("created %d quotes, want none", len(h.quotes.created))
	}
}

func TestRFQService_CreateTextDraft_KeepsTheOrderWhenNoMaterialIsRead(t *testing.T) {
	h := newRFQHarness(nil)

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "hola, están abiertos?"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v", err)
	}
	if draft.Quote != nil || draft.Version != nil || len(draft.Items) != 0 {
		t.Errorf("draft returned %+v, want the RFQ alone", draft)
	}
	// The pipeline is finished and produced nothing, so the order is the seller's to load. Left
	// RECEIVED it would read as one still being processed, and the spinner would never resolve.
	if draft.RFQ.Status != domain.RFQStatusFailed {
		t.Errorf("RFQ status %q, want FAILED", draft.RFQ.Status)
	}
}

func TestRFQService_CreateTextDraft_FlagsEveryLineWhenMatchingCannotAnswer(t *testing.T) {
	cases := []struct {
		name  string
		stage func(*rfqHarness)
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
				t.Fatalf("CreateTextDraft returned %v", err)
			}
			if len(draft.Items) != 2 {
				t.Fatalf("persisted %d lines, want both", len(draft.Items))
			}
			for i, item := range h.quotes.itemBatches[0] {
				if item.MatchStatus != domain.ItemMatchStatusNoMatch {
					t.Errorf("line %d status %q, want NO_MATCH", i, item.MatchStatus)
				}
				if item.ProductID != nil {
					t.Errorf("line %d carries product %v, want none", i, item.ProductID)
				}
				if item.ConfidenceScore.Valid {
					t.Errorf("line %d carries confidence %v, want null", i, item.ConfidenceScore)
				}
			}
			if len(h.quotes.alternativeBatches) != 0 {
				t.Errorf("wrote candidates %v, want none", h.quotes.alternativeBatches)
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
		t.Fatalf("CreateTextDraft returned %v", err)
	}
	if len(draft.Items) != 1 {
		t.Fatalf("persisted %d lines, want the material kept", len(draft.Items))
	}
	item := h.quotes.itemBatches[0][0]
	if !item.Quantity.IsZero() {
		t.Errorf("line quantity %s, want zero", item.Quantity)
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
	if h.extractor.calls != 0 {
		t.Errorf("the extractor ran %d times for an unreachable channel", h.extractor.calls)
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
	if h.quotes.created[0].SellerID != nil {
		t.Errorf("quote seller %v, want none on an inbound message",
			h.quotes.created[0].SellerID)
	}
	if h.quotes.versions[0].AuthorID != nil {
		t.Errorf("version author %v, want none on an inbound message",
			h.quotes.versions[0].AuthorID)
	}
	if draft.Quote == nil {
		t.Error("the mock produced no quote")
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

// ---------- GetDetail tests ----------

func getDetailHarness() (*rfqHarness, *domain.RfqListItem, *domain.Quote, *domain.QuoteVersion, []domain.QuoteItem) {
	h := newRFQHarness(nil)

	rfqItem := &domain.RfqListItem{
		ID:          testRFQID,
		ClientLabel: strPtr("Obra Norte"),
		Channel:     "whatsapp",
		SellerName:  "Juan Pérez",
		BranchName:  "Matriz",
		ItemCount:   2,
		Status:      string(domain.QuoteStatusDraft),
	}
	quote := &domain.Quote{
		ID: testQuoteID, RFQID: testRFQID, BranchID: testBranchID,
		CurrentStatus:    domain.QuoteStatusDraft,
		CurrentVersionID: &testVersionID,
	}
	version := &domain.QuoteVersion{
		ID: testVersionID, QuoteID: testQuoteID, VersionNumber: 1,
		Total: decimal.RequireFromString("5000.00"),
	}
	items := []domain.QuoteItem{
		{
			ID: uuid.New(), VersionID: testVersionID,
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("10"),
			MatchStatus:          domain.ItemMatchStatusMatched,
		},
		{
			ID: uuid.New(), VersionID: testVersionID,
			RequestedDescription: "2 rollos de membrana",
			Quantity:             decimal.RequireFromString("2"),
			MatchStatus:          domain.ItemMatchStatusNoMatch,
		},
	}

	h.rfqs.rfqByID = rfqItem
	h.quotes.quoteByID = quote
	h.quotes.currentVersionData = version
	h.quotes.itemsByVersionID = items
	h.rfqs.statusHistory = []domain.RFQStatusChange{{
		ID: uuid.New(), RFQID: testRFQID, NewStatus: domain.RFQStatusGenerated,
		ChangedAt: fixedNow.Add(-2 * time.Hour), CreatedAt: fixedNow.Add(-2 * time.Hour),
	}}
	previous := domain.QuoteStatusQuoted
	h.quotes.quoteStatusHistory = []domain.QuoteStatusChange{{
		ID: uuid.New(), QuoteID: testQuoteID, PreviousStatus: &previous,
		NewStatus: domain.QuoteStatusSent, ChangedAt: fixedNow.Add(-time.Hour),
		CreatedAt: fixedNow.Add(-time.Hour),
	}}
	sentAt := fixedNow.Add(-time.Hour)
	expiresAt := fixedNow.AddDate(0, 0, 7)
	h.sends.deliveries = []domain.QuoteSend{{
		ID: uuid.New(), VersionID: testVersionID, ChannelType: domain.ChannelTypeWhatsApp,
		Destination: "+5491155550101", Format: domain.SendFormatWebAppLink,
		TrackingStatus: domain.SendTrackingStatusViewed, SentAt: &sentAt,
		ExpiresAt: &expiresAt, CreatedAt: sentAt,
	}}

	return h, rfqItem, quote, version, items
}

func TestRFQService_GetDetail_ReturnsFullDetailWhenAllDataExists(t *testing.T) {
	h, rfqItem, quote, version, items := getDetailHarness()

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail == nil {
		t.Fatal("GetDetail returned nil detail")
	}
	if detail.Rfq.ID != rfqItem.ID {
		t.Errorf("rfq ID = %v, want %v", detail.Rfq.ID, rfqItem.ID)
	}
	if detail.Quote == nil || detail.Quote.ID != quote.ID {
		t.Errorf("quote = %v, want %v", detail.Quote, quote)
	}
	if detail.Version == nil || detail.Version.ID != version.ID {
		t.Errorf("version = %v, want %v", detail.Version, version)
	}
	if len(detail.Items) != len(items) {
		t.Errorf("items = %d, want %d", len(detail.Items), len(items))
	}
	if len(h.quotes.currentVersionBranches) != 1 ||
		h.quotes.currentVersionBranches[0] != quote.BranchID {
		t.Errorf("current version branch = %v, want [%v]",
			h.quotes.currentVersionBranches, quote.BranchID)
	}
	if len(detail.RFQStatusChanges) != 1 ||
		detail.RFQStatusChanges[0].NewStatus != domain.RFQStatusGenerated {
		t.Errorf("RFQ status history = %+v, want one GENERATED transition",
			detail.RFQStatusChanges)
	}
	if len(detail.QuoteStatusChanges) != 1 ||
		detail.QuoteStatusChanges[0].NewStatus != domain.QuoteStatusSent {
		t.Errorf("quote status history = %+v, want one SENT transition",
			detail.QuoteStatusChanges)
	}
	if len(detail.Deliveries) != 1 ||
		detail.Deliveries[0].TrackingStatus != domain.SendTrackingStatusViewed {
		t.Errorf("deliveries = %+v, want one viewed send", detail.Deliveries)
	}
	if len(h.sends.reads) != 1 || h.sends.reads[0] != quote.ID {
		t.Errorf("delivery reads = %v, want [%v]", h.sends.reads, quote.ID)
	}
	if len(h.db.scopes) != 1 || h.db.scopes[0] != testAccountID {
		t.Errorf("transaction scoped to %v, want [%v]", h.db.scopes, testAccountID)
	}
}

func TestRFQService_GetDetail_LoadsQuoteVersionWhenTenantHasNoActiveBranch(t *testing.T) {
	h, _, quote, version, items := getDetailHarness()
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	detail, err := h.service.GetDetail(context.Background(), tenant, testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.Version == nil || detail.Version.ID != version.ID {
		t.Fatalf("version = %v, want %v", detail.Version, version)
	}
	if len(detail.Items) != len(items) {
		t.Errorf("items = %d, want %d", len(detail.Items), len(items))
	}
	if len(h.quotes.currentVersionBranches) != 1 ||
		h.quotes.currentVersionBranches[0] != quote.BranchID {
		t.Errorf("current version branch = %v, want [%v]",
			h.quotes.currentVersionBranches, quote.BranchID)
	}
}

func TestRFQService_GetDetail_RfqNotFound(t *testing.T) {
	h := newRFQHarness(nil)
	h.rfqs.rfqByIDErr = domain.ErrNotFound

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetDetail returned %v, want ErrNotFound", err)
	}
	if detail != nil {
		t.Errorf("detail = %v, want nil", detail)
	}
}

func TestRFQService_GetDetail_QuoteNotFoundReturnsRfqWithoutQuote(t *testing.T) {
	h := newRFQHarness(nil)
	h.rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID}
	h.quotes.quoteByIDErr = domain.ErrNotFound

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.Rfq.ID != testRFQID {
		t.Errorf("rfq ID = %v, want %v", detail.Rfq.ID, testRFQID)
	}
	if detail.Quote != nil {
		t.Errorf("quote = %v, want nil", detail.Quote)
	}
	if detail.Version != nil {
		t.Errorf("version = %v, want nil", detail.Version)
	}
	if detail.Items != nil {
		t.Errorf("items = %v, want nil", detail.Items)
	}
}

func TestRFQService_CreateTextDraft_FailsTheDraftWhenTheAIBaselineCannotBeWritten(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
	})
	h.generations.err = domain.ErrNotFound

	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "10 bolsas de cemento"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("CreateTextDraft returned %v, want ErrNotFound", err)
	}
	if len(h.rfqs.updatedStatus) != 0 || len(h.quotes.statusChanges) != 0 {
		t.Errorf("completed status writes RFQ=%v quote=%v, want none when the baseline failed",
			h.rfqs.updatedStatus, h.quotes.statusChanges)
	}
}

func TestRFQService_CreateTextDraft_OffersNoCandidateThatScoredNothing(t *testing.T) {
	nearMiss := uuid.MustParse("c1111111-1111-4111-8111-111111111111")
	leader := uuid.MustParse("c2222222-2222-4222-8222-222222222222")
	// What a catalog smaller than the top-K returns: the offers that resemble the line, then
	// products the search reached without them resembling it at all.
	orthogonal := []domain.ScoredCandidate{
		scoredCandidate(uuid.New(), "Cemento Portland 50kg", "0"),
		scoredCandidate(uuid.New(), "Cal hidratada 25kg", "0"),
	}
	cases := []struct {
		name      string
		match     domain.LineMatch
		wantOffer uuid.UUID
		wantRank  int
	}{
		{
			name: "a line nothing matched offers its near miss alone",
			match: domain.LineMatch{
				MatchStatus: domain.ItemMatchStatusNoMatch,
				Confidence:  decimal.RequireFromString("0.5500"),
				Candidates: append([]domain.ScoredCandidate{
					scoredCandidate(nearMiss, "Membrana asfáltica 4mm", "0.5500"),
				}, orthogonal...),
			},
			wantOffer: nearMiss,
			wantRank:  1,
		},
		{
			// The same rule on an AMBIGUOUS line, which the table in rfq-pipeline.md calls the
			// one exception to "every candidate but the one it kept".
			name: "an ambiguous line offers neither the leader nor a zero",
			match: domain.LineMatch{
				ProductID:   &leader,
				MatchStatus: domain.ItemMatchStatusAmbiguous,
				Confidence:  decimal.RequireFromString("0.8200"),
				Candidates: append([]domain.ScoredCandidate{
					scoredCandidate(leader, "Cemento Portland 50kg", "0.8200"),
					scoredCandidate(nearMiss, "Cemento Avellaneda 50kg", "0.8000"),
				}, orthogonal...),
			},
			wantOffer: nearMiss,
			wantRank:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRFQHarness([]domain.ExtractedRFQLine{
				explicitLine("membrana rara", "2", "rollo", "pidió 2"),
			})
			h.matcher.matches[0] = tc.match

			draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
				domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "membrana rara"})
			if err != nil {
				t.Fatalf("CreateTextDraft returned %v", err)
			}

			offered := draft.Alternatives[draft.Items[0].ID]
			if len(offered) != 1 {
				t.Fatalf("offered %d candidates, want one: a candidate at zero scored no "+
					"similarity and would bury the one the seller is looking for", len(offered))
			}
			if offered[0].ProductID == nil || *offered[0].ProductID != tc.wantOffer {
				t.Errorf("offer = %v, want %v", offered[0].ProductID, tc.wantOffer)
			}
			// The rank is the candidate's own place in the matcher's ranking, so dropping a zero
			// does not renumber what survives.
			if offered[0].Rank != tc.wantRank {
				t.Errorf("offer rank = %d, want %d", offered[0].Rank, tc.wantRank)
			}
		})
	}
}

func TestRFQService_CreateTextDraft_OffersTheCandidatesOfEveryFlaggedLine(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
		explicitLine("membrana rara", "2", "rollo", "pidió 2"),
	})
	leader := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
	runnerUp := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
	third := uuid.MustParse("b3333333-3333-4333-8333-333333333333")
	nearMiss := uuid.MustParse("b4444444-4444-4444-8444-444444444444")
	longShot := uuid.MustParse("b5555555-5555-4555-8555-555555555555")
	h.matcher.matches = []domain.LineMatch{
		{
			ProductID:   &leader,
			MatchStatus: domain.ItemMatchStatusAmbiguous,
			Confidence:  decimal.RequireFromString("0.8200"),
			Candidates: []domain.ScoredCandidate{
				scoredCandidate(leader, "Cemento Loma Negra 50kg", "0.8200"),
				scoredCandidate(runnerUp, "Cemento Avellaneda 50kg", "0.8000"),
				scoredCandidate(third, "Cemento Holcim 50kg", "0.7100"),
			},
		},
		{
			MatchStatus: domain.ItemMatchStatusNoMatch,
			Confidence:  decimal.RequireFromString("0.5500"),
			Candidates: []domain.ScoredCandidate{
				scoredCandidate(nearMiss, "Membrana asfáltica 4mm", "0.5500"),
				scoredCandidate(longShot, "Membrana geotextil", "0.3100"),
			},
		},
	}

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "cemento y membrana rara"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v", err)
	}

	if len(h.quotes.alternativeBatches) != 1 {
		t.Fatalf("wrote %d candidate batches, want 1 alongside the lines",
			len(h.quotes.alternativeBatches))
	}
	// Every candidate is written in the same statement, and the lines' candidates never travel in
	// separate round trips: four rows, not one query per flagged line.
	if got := len(h.quotes.alternativeBatches[0]); got != 4 {
		t.Fatalf("wrote %d candidates, want 4: the leader is on its line already", got)
	}
	// One transaction for the order, one for the draft. The candidates ride the draft's.
	if len(h.db.scopes) != 2 {
		t.Errorf("opened %d transactions, want 2: the candidates are part of the draft",
			len(h.db.scopes))
	}

	if len(draft.Items) != 2 {
		t.Fatalf("persisted %d lines, want both", len(draft.Items))
	}
	ambiguous, noMatch := draft.Items[0], draft.Items[1]

	// The AMBIGUOUS line keeps the leader and offers the two it might have been instead. Rank is
	// the matcher's own ranking, so its offers start at two.
	offered := draft.Alternatives[ambiguous.ID]
	if len(offered) != 2 {
		t.Fatalf("ambiguous line offers %d candidates, want the two it is not", len(offered))
	}
	wantAmbiguous := []struct {
		productID  uuid.UUID
		rank       int
		confidence string
	}{{runnerUp, 2, "0.8000"}, {third, 3, "0.7100"}}
	for i, want := range wantAmbiguous {
		got := offered[i]
		if got.ProductID == nil || *got.ProductID != want.productID {
			t.Errorf("offer %d product = %v, want %v", i, got.ProductID, want.productID)
		}
		if got.Rank != want.rank {
			t.Errorf("offer %d rank = %d, want %d", i, got.Rank, want.rank)
		}
		if !got.ConfidenceScore.Valid ||
			!got.ConfidenceScore.Decimal.Equal(decimal.RequireFromString(want.confidence)) {
			t.Errorf("offer %d confidence = %v, want %s", i, got.ConfidenceScore, want.confidence)
		}
		if got.Type != domain.QuoteItemAlternativeTypeProduct {
			t.Errorf("offer %d type = %q, want PRODUCT", i, got.Type)
		}
		if got.Origin != domain.QuoteItemAlternativeOriginAI {
			t.Errorf("offer %d origin = %q, want AI", i, got.Origin)
		}
		// Nothing has been priced when matching runs, and a zero would read as free.
		if got.PriceSnapshot.Valid {
			t.Errorf("offer %d carries price %v, want none", i, got.PriceSnapshot)
		}
	}
	if offered[0].ProductID != nil && ambiguous.ProductID != nil &&
		*offered[0].ProductID == *ambiguous.ProductID {
		t.Error("the leading candidate is offered as an alternative to itself")
	}

	// The NO_MATCH line points at nothing, so every candidate is on offer — the near miss included,
	// which is the one thing that tells the seller how close the catalog came.
	rejected := draft.Alternatives[noMatch.ID]
	if len(rejected) != 2 {
		t.Fatalf("no-match line offers %d candidates, want both rejected ones", len(rejected))
	}
	if rejected[0].ProductID == nil || *rejected[0].ProductID != nearMiss || rejected[0].Rank != 1 {
		t.Errorf("closest offer = %+v, want %v at rank 1", rejected[0], nearMiss)
	}
	if rejected[1].ProductID == nil || *rejected[1].ProductID != longShot || rejected[1].Rank != 2 {
		t.Errorf("second offer = %+v, want %v at rank 2", rejected[1], longShot)
	}

	// Each line's offers name that line and no other. Pairing them by insert order would rest on
	// an order Postgres does not promise, and a line offering another line's products is a wrong
	// answer nothing downstream could notice.
	for itemID, alternatives := range draft.Alternatives {
		for _, alternative := range alternatives {
			if alternative.QuoteItemID != itemID {
				t.Errorf("line %v was handed an offer belonging to %v", itemID,
					alternative.QuoteItemID)
			}
		}
	}
	for _, alternative := range draft.Alternatives[ambiguous.ID] {
		if alternative.ProductID != nil &&
			(*alternative.ProductID == nearMiss || *alternative.ProductID == longShot) {
			t.Errorf("the cemento line was offered %v, which belongs to the membrana line",
				*alternative.ProductID)
		}
	}
}

func TestRFQService_CreateTextDraft_OffersNothingOnADecidedLine(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
	})
	// The harness answers MATCHED, and a decided line had candidates too — the runner-up simply
	// lost by enough that there is nothing to choose between.
	h.matcher.matches[0].Candidates = []domain.ScoredCandidate{
		scoredCandidate(testProductID, "Cemento Loma Negra 50kg", "0.9100"),
		scoredCandidate(uuid.New(), "Cal hidratada 25kg", "0.4000"),
	}

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "10 bolsas de cemento"})
	if err != nil {
		t.Fatalf("CreateTextDraft returned %v", err)
	}
	if len(h.quotes.alternativeBatches) != 0 {
		t.Errorf("wrote %v, want nothing: a decided line asks the seller to choose nothing",
			h.quotes.alternativeBatches)
	}
	// And nothing is read back either: the round trip is skipped, not merely empty.
	if len(h.quotes.alternativeReads) != 0 {
		t.Errorf("read candidates %v times, want none", len(h.quotes.alternativeReads))
	}
	if len(draft.Alternatives) != 0 {
		t.Errorf("draft offers %v, want none", draft.Alternatives)
	}
}

func TestRFQService_CreateTextDraft_FailsTheDraftWhenCandidatesCannotBeWritten(t *testing.T) {
	h := newRFQHarness([]domain.ExtractedRFQLine{
		explicitLine("cemento", "10", "bolsa", "pidió 10"),
	})
	h.matcher.matches[0] = domain.LineMatch{
		MatchStatus: domain.ItemMatchStatusNoMatch,
		Confidence:  decimal.RequireFromString("0.4000"),
		Candidates: []domain.ScoredCandidate{
			scoredCandidate(uuid.New(), "Cemento Loma Negra 50kg", "0.4000"),
		},
	}
	// A line of another account would answer this way: the join matched no row, so the write is
	// refused rather than landing a candidate in the wrong tenant.
	h.quotes.alternativesErr = domain.ErrNotFound

	_, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "10 bolsas de cemento"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("CreateTextDraft returned %v, want ErrNotFound", err)
	}
	// The whole transition rolls back with it: a quote whose lines are flagged while the candidates
	// they were flagged against are missing is the half-written draft the transaction prevents.
	if len(h.rfqs.updatedStatus) != 0 {
		t.Errorf("wrote RFQ statuses %v, want none: the draft never completed",
			h.rfqs.updatedStatus)
	}
}

func TestRFQService_GetDetail_VersionNotFoundGracefullySkipped(t *testing.T) {
	h := newRFQHarness(nil)
	h.rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID}
	quote := &domain.Quote{
		ID: testQuoteID, RFQID: testRFQID, BranchID: testBranchID,
		CurrentVersionID: &testVersionID,
	}
	h.quotes.quoteByID = quote
	h.quotes.currentVersionErr = domain.ErrNotFound

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.Quote == nil || detail.Quote.ID != quote.ID {
		t.Errorf("quote = %v, want %v", detail.Quote, quote)
	}
	if detail.Version != nil {
		t.Errorf("version = %v, want nil when GetCurrentVersion returns ErrNotFound", detail.Version)
	}
}

func TestRFQService_GetDetail_QuoteRepositoryErrorIsPropagated(t *testing.T) {
	h := newRFQHarness(nil)
	h.rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID}
	h.quotes.quoteByIDErr = errors.New("database timeout")

	_, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err == nil || err.Error() != "database timeout" {
		t.Fatalf("GetDetail returned %v, want database timeout", err)
	}
}

func TestRFQService_GetDetail_QuoteWithoutVersionReturnsRfqAndQuoteOnly(t *testing.T) {
	h := newRFQHarness(nil)
	h.rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID}
	h.quotes.quoteByID = &domain.Quote{
		ID: testQuoteID, RFQID: testRFQID, BranchID: testBranchID,
		CurrentStatus:    domain.QuoteStatusDraft,
		CurrentVersionID: nil,
	}

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.Quote == nil {
		t.Fatal("quote is nil, want present")
	}
	if detail.Version != nil {
		t.Errorf("version = %v, want nil when quote has no version", detail.Version)
	}
	if detail.Items != nil {
		t.Errorf("items = %v, want nil when no version", detail.Items)
	}
}

// ---------- Discount tests ----------

func discountHarness() *rfqHarness {
	h := newRFQHarness(nil)
	h.quotes.quoteByID = &domain.Quote{
		ID: testQuoteID, RFQID: testRFQID, BranchID: testBranchID,
		CurrentStatus:    domain.QuoteStatusQuoted,
		CurrentVersionID: &testVersionID,
	}
	h.quotes.currentVersionData = &domain.QuoteVersion{
		ID: testVersionID, QuoteID: testQuoteID, VersionNumber: 1,
		Total: decimal.RequireFromString("2000.00"),
	}
	h.quotes.itemsByVersionID = []domain.QuoteItem{{
		ID:        uuid.New(),
		VersionID: testVersionID,
		Quantity:  decimal.RequireFromString("10"),
		Subtotal:  decimal.NewNullDecimal(decimal.RequireFromString("2000.00")),
	}}
	return h
}

func TestRFQService_AddDiscount_AppliesAndRecomputesTotal(t *testing.T) {
	h := discountHarness()

	got, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Descuento por pago contado",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("250.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	if got.Origin != domain.DiscountOriginManualSeller {
		t.Errorf("origin = %q, want MANUAL_SELLER", got.Origin)
	}
	if got.Scope != domain.DiscountScopeTotal {
		t.Errorf("scope = %q, want TOTAL", got.Scope)
	}
	if got.ActionType != domain.PromotionActionFixedAmount {
		t.Errorf("action_type = %q, want FIXED_AMOUNT", got.ActionType)
	}
	if got.ActionValue == nil || got.ActionValue.String() != "250" {
		t.Errorf("action_value = %v, want 250", got.ActionValue)
	}
	if got.Description == nil || *got.Description != "Descuento por pago contado" {
		t.Errorf("description = %v, want the seller's text", got.Description)
	}
	if len(h.discounts.stored) != 1 {
		t.Fatalf("stored discounts = %d, want 1", len(h.discounts.stored))
	}
	if len(h.quotes.versionTotals) != 1 {
		t.Fatalf("version total updates = %d, want 1", len(h.quotes.versionTotals))
	}
	if want := "1750.00"; h.quotes.versionTotals[0].StringFixed(domain.MoneyScale) != want {
		t.Errorf("recomputed total = %v, want %q", h.quotes.versionTotals[0], want)
	}
}

func TestRFQService_AddDiscount_ComputesAPercentageAgainstTheScope(t *testing.T) {
	h := discountHarness()

	got, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "10% pago contado",
			ActionType:  domain.PromotionActionPercentage,
			Value:       decimal.RequireFromString("10.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	if got.ActionValue == nil || got.ActionValue.String() != "10" {
		t.Errorf("action_value = %v, want 10", got.ActionValue)
	}
	if want := "200.00"; got.Amount.StringFixed(domain.MoneyScale) != want {
		t.Errorf("amount = %v, want %q", got.Amount, want)
	}
	if want := "1800.00"; h.quotes.versionTotals[0].StringFixed(domain.MoneyScale) != want {
		t.Errorf("recomputed total = %v, want %q", h.quotes.versionTotals[0], want)
	}
}

func TestRFQService_AddDiscount_ScopesToAnItem(t *testing.T) {
	h := discountHarness()
	itemID := h.quotes.itemsByVersionID[0].ID

	got, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Bono por ítem",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("100.00"),
			Scope:       domain.DiscountScopeItem,
			ItemIDs:     []uuid.UUID{itemID},
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	if got.Scope != domain.DiscountScopeItem {
		t.Errorf("scope = %q, want ITEM", got.Scope)
	}
	if want := "100.00"; got.Amount.StringFixed(domain.MoneyScale) != want {
		t.Errorf("amount = %v, want %q", got.Amount, want)
	}
	if len(h.discounts.links[got.ID]) != 1 || h.discounts.links[got.ID][0] != itemID {
		t.Errorf("links = %v, want [%s]", h.discounts.links[got.ID], itemID)
	}
	if want := "1900.00"; h.quotes.versionTotals[0].StringFixed(domain.MoneyScale) != want {
		t.Errorf("recomputed total = %v, want %q", h.quotes.versionTotals[0], want)
	}
}

func TestRFQService_AddDiscount_RejectsZeroValue(t *testing.T) {
	h := discountHarness()

	_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Bienvenida",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.Zero,
			Scope:       domain.DiscountScopeTotal,
		})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AddDiscount returned %v, want ErrInvalidInput", err)
	}
	if len(h.discounts.stored) != 0 {
		t.Errorf("stored discounts = %d, want none", len(h.discounts.stored))
	}
}

func TestRFQService_AddDiscount_RejectsAnEmptyDescription(t *testing.T) {
	h := discountHarness()

	_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "  ",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("100"),
			Scope:       domain.DiscountScopeTotal,
		})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AddDiscount returned %v, want ErrInvalidInput", err)
	}
}

func TestRFQService_AddDiscount_RejectsAValueAboveTheScope(t *testing.T) {
	h := discountHarness()

	_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Excesivo",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("2000.01"),
			Scope:       domain.DiscountScopeTotal,
		})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AddDiscount returned %v, want ErrInvalidInput", err)
	}
	if len(h.discounts.stored) != 0 {
		t.Errorf("stored discounts = %d, want none", len(h.discounts.stored))
	}
}

func TestRFQService_AddDiscount_RejectsAStringItemID(t *testing.T) {
	h := discountHarness()

	_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Item fantasma",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("100"),
			Scope:       domain.DiscountScopeItem,
			ItemIDs:     []uuid.UUID{uuid.New()},
		})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AddDiscount returned %v, want ErrInvalidInput", err)
	}
	if len(h.discounts.stored) != 0 {
		t.Errorf("stored discounts = %d, want none", len(h.discounts.stored))
	}
}

func TestRFQService_AddDiscount_RefusesAFrozenStatus(t *testing.T) {
	h := discountHarness()
	h.quotes.quoteByID.CurrentStatus = domain.QuoteStatusSent

	_, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Fuera de tiempo",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("100"),
			Scope:       domain.DiscountScopeTotal,
		})
	if !errors.Is(err, domain.ErrImmutable) {
		t.Fatalf("AddDiscount returned %v, want ErrImmutable", err)
	}
	if len(h.discounts.stored) != 0 {
		t.Errorf("stored discounts = %d, want none", len(h.discounts.stored))
	}
}

func TestRFQService_UpdateDiscount_TogglingSuppressionRecomputesTheTotal(t *testing.T) {
	h := discountHarness()
	created, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Contado",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("200.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	suppress := true
	updated, err := h.service.UpdateDiscount(context.Background(), rfqTenant(), testQuoteID,
		created.ID, domain.QuoteDiscountUpdate{SuppressedBySeller: &suppress})
	if err != nil {
		t.Fatalf("UpdateDiscount returned %v", err)
	}
	if !updated.SuppressedBySeller {
		t.Error("discount is not suppressed, want suppressed")
	}
	last := h.quotes.versionTotals[len(h.quotes.versionTotals)-1]
	if want := "2000.00"; last.StringFixed(domain.MoneyScale) != want {
		t.Errorf("total after suppressing = %v, want %q", last, want)
	}
}

func TestRFQService_UpdateDiscount_ChangesTheValueWithinTheScope(t *testing.T) {
	h := discountHarness()
	created, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Bono",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("200.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	value := decimal.RequireFromString("500.00")
	updated, err := h.service.UpdateDiscount(context.Background(), rfqTenant(), testQuoteID,
		created.ID, domain.QuoteDiscountUpdate{Value: &value})
	if err != nil {
		t.Fatalf("UpdateDiscount returned %v", err)
	}
	if updated.Amount.StringFixed(domain.MoneyScale) != "500.00" {
		t.Errorf("amount = %v, want 500.00", updated.Amount)
	}
	last := h.quotes.versionTotals[len(h.quotes.versionTotals)-1]
	if want := "1500.00"; last.StringFixed(domain.MoneyScale) != want {
		t.Errorf("total after editing = %v, want %q", last, want)
	}
}

func TestRFQService_UpdateDiscount_RejectsAValueAboveTheScope(t *testing.T) {
	h := discountHarness()
	created, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Bono",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("200.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	value := decimal.RequireFromString("2000.01")
	_, err = h.service.UpdateDiscount(context.Background(), rfqTenant(), testQuoteID,
		created.ID, domain.QuoteDiscountUpdate{Value: &value})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("UpdateDiscount returned %v, want ErrInvalidInput", err)
	}
}

func TestRFQService_UpdateDiscount_SwitchesToAPercentage(t *testing.T) {
	h := discountHarness()
	created, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Bono",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("200.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	actionType := domain.PromotionActionPercentage
	value := decimal.RequireFromString("25")
	updated, err := h.service.UpdateDiscount(context.Background(), rfqTenant(), testQuoteID,
		created.ID, domain.QuoteDiscountUpdate{ActionType: &actionType, Value: &value})
	if err != nil {
		t.Fatalf("UpdateDiscount returned %v", err)
	}
	if updated.ActionType != domain.PromotionActionPercentage {
		t.Errorf("action_type = %q, want PERCENTAGE", updated.ActionType)
	}
	if updated.Amount.StringFixed(domain.MoneyScale) != "500.00" {
		t.Errorf("amount = %v, want 500.00 (25%% of 2000)", updated.Amount)
	}
	last := h.quotes.versionTotals[len(h.quotes.versionTotals)-1]
	if want := "1500.00"; last.StringFixed(domain.MoneyScale) != want {
		t.Errorf("total after editing = %v, want %q", last, want)
	}
}

func TestRFQService_DeleteDiscount_RemovesASellerTypedDiscount(t *testing.T) {
	h := discountHarness()
	created, err := h.service.AddDiscount(context.Background(), rfqTenant(), testQuoteID,
		domain.QuoteDiscountCreate{
			Description: "Fuera",
			ActionType:  domain.PromotionActionFixedAmount,
			Value:       decimal.RequireFromString("200.00"),
			Scope:       domain.DiscountScopeTotal,
		})
	if err != nil {
		t.Fatalf("AddDiscount returned %v", err)
	}
	if err := h.service.DeleteDiscount(context.Background(), rfqTenant(), testQuoteID, created.ID); err != nil {
		t.Fatalf("DeleteDiscount returned %v", err)
	}
	if len(h.discounts.stored) != 0 {
		t.Errorf("stored discounts = %d, want none after delete", len(h.discounts.stored))
	}
	last := h.quotes.versionTotals[len(h.quotes.versionTotals)-1]
	if want := "2000.00"; last.StringFixed(domain.MoneyScale) != want {
		t.Errorf("total after deleting = %v, want %q", last, want)
	}
}

func TestRFQService_DeleteDiscount_RefusesAnAutomaticOne(t *testing.T) {
	h := discountHarness()
	h.discounts.stored = append(h.discounts.stored, domain.QuoteDiscount{
		ID: uuid.New(), AccountID: testAccountID, QuoteVersionID: testVersionID,
		ConditionType: domain.PromotionConditionOnTotal, Scope: domain.DiscountScopeTotal,
		Origin: domain.DiscountOriginAutomatic, Amount: decimal.RequireFromString("150.00"),
	})

	err := h.service.DeleteDiscount(context.Background(), rfqTenant(), testQuoteID,
		h.discounts.stored[0].ID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("DeleteDiscount returned %v, want ErrInvalidInput", err)
	}
	if len(h.discounts.stored) != 1 {
		t.Errorf("stored discounts = %d, want the automatic one kept", len(h.discounts.stored))
	}
}

func TestRFQService_GetDetail_CarriesTheVersionDiscounts(t *testing.T) {
	h, _, _, _, _ := getDetailHarness()
	h.discounts.stored = append(h.discounts.stored, domain.QuoteDiscount{
		ID: uuid.New(), AccountID: testAccountID, QuoteVersionID: testVersionID,
		ConditionType: domain.PromotionConditionOnTotal, Scope: domain.DiscountScopeTotal,
		Origin:      domain.DiscountOriginManualSeller,
		Amount:      decimal.RequireFromString("250.00"),
		Description: strPtr("Descuento contado"),
	})

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if len(detail.Discounts) != 1 {
		t.Fatalf("discounts = %d, want 1", len(detail.Discounts))
	}
	if detail.Discounts[0].Amount.String() != "250" {
		t.Errorf("discount amount = %v, want 250", detail.Discounts[0].Amount)
	}
}

func TestRFQService_GetDetail_PopulatesChangeRequestDiffFromFrozenPredecessor(t *testing.T) {
	h, _, _, _, _ := getDetailHarness()

	frozenID := uuid.New()
	h.quotes.currentVersionData = &domain.QuoteVersion{
		ID: testVersionID, QuoteID: testQuoteID, VersionNumber: 2, Total: decimal.NewFromFloat(17000),
		IsImmutable: false, Comment: strPtr("necesito entrega el viernes"),
	}
	h.quotes.previousVersionData = &domain.QuoteVersion{
		ID: frozenID, QuoteID: testQuoteID, VersionNumber: 1,
		Total: decimal.NewFromFloat(16000), IsImmutable: true,
	}
	h.quotes.itemsByVersionID = []domain.QuoteItem{
		{
			ID: uuid.New(), VersionID: testVersionID,
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("12"),
			MatchStatus:          domain.ItemMatchStatusMatched,
		},
	}
	h.quotes.frozenItems = []domain.QuoteItem{
		{
			ID: uuid.New(), VersionID: frozenID,
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("10"),
			MatchStatus:          domain.ItemMatchStatusMatched,
		},
	}
	h.quotes.quoteByID.CurrentStatus = domain.QuoteStatusChangeRequested
	h.discounts.stored = []domain.QuoteDiscount{
		{
			ID: uuid.New(), AccountID: testAccountID, QuoteVersionID: frozenID,
			PromotionName: strPtr("Descuento obra"), Amount: decimal.RequireFromString("500.00"),
		},
		{
			ID: uuid.New(), AccountID: testAccountID, QuoteVersionID: testVersionID,
			PromotionName: strPtr("Descuento obra"), Amount: decimal.RequireFromString("800.00"),
		},
	}

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.ChangesRequested == nil {
		t.Fatal("ChangesRequested is nil, want the frozen-vs-draft comparison")
	}
	if detail.ChangesRequested.Reason == nil || *detail.ChangesRequested.Reason != "necesito entrega el viernes" {
		t.Errorf("reason = %v, want the client's request", detail.ChangesRequested.Reason)
	}
	if len(detail.ChangesRequested.Original.Items) != 1 || len(detail.ChangesRequested.Requested.Items) != 1 {
		t.Fatalf("items = original %d requested %d, want 1 and 1",
			len(detail.ChangesRequested.Original.Items), len(detail.ChangesRequested.Requested.Items))
	}
	if !detail.ChangesRequested.Requested.Items[0].Changed {
		t.Error("quantity change not flagged")
	}
	if detail.ChangesRequested.Requested.Items[0].ChangeType != "modified" {
		t.Errorf("change type = %q, want modified", detail.ChangesRequested.Requested.Items[0].ChangeType)
	}
	if len(detail.ChangesRequested.Requested.Discounts) != 1 || !detail.ChangesRequested.Requested.Discounts[0].Changed {
		t.Error("discount amount change not flagged on the requested side")
	}
	if !detail.ChangesRequested.Original.Total.Equal(decimal.NewFromFloat(16000)) {
		t.Errorf("original total = %v, want 16000", detail.ChangesRequested.Original.Total)
	}
	if !detail.ChangesRequested.Requested.Total.Equal(decimal.NewFromFloat(17000)) {
		t.Errorf("requested total = %v, want 17000", detail.ChangesRequested.Requested.Total)
	}
}

func TestRFQService_GetDetail_NoDiffWithoutAFrozenPredecessor(t *testing.T) {
	h, _, _, _, _ := getDetailHarness()
	h.quotes.currentVersionData = &domain.QuoteVersion{
		ID: testVersionID, QuoteID: testQuoteID, VersionNumber: 1,
		Total: decimal.NewFromFloat(16000), IsImmutable: false,
	}
	h.quotes.quoteByID.CurrentStatus = domain.QuoteStatusChangeRequested
	h.quotes.previousVersionErr = domain.ErrNotFound

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.ChangesRequested != nil {
		t.Error("ChangesRequested is set without a frozen predecessor")
	}
}

func TestRFQService_GetDetail_SkipsTheDiffOutsideChangeRequested(t *testing.T) {
	h, _, _, _, _ := getDetailHarness()
	h.quotes.previousVersionData = &domain.QuoteVersion{
		ID: uuid.New(), QuoteID: testQuoteID, VersionNumber: 1,
		Total: decimal.NewFromFloat(16000), IsImmutable: true,
	}

	detail, err := h.service.GetDetail(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("GetDetail returned %v", err)
	}
	if detail.ChangesRequested != nil {
		t.Error("ChangesRequested is set for a non-CHANGE_REQUESTED quote")
	}
	if h.quotes.getPreviousCalls != 0 {
		t.Errorf("GetPreviousVersion called %d times, want 0", h.quotes.getPreviousCalls)
	}
}

func TestRFQService_AssignSeller_ClaimsTheRFQ(t *testing.T) {
	h := newRFQHarness(nil)
	claimed := &domain.Quote{
		ID: uuid.New(), RFQID: testRFQID, SellerID: &testUserID,
		CurrentStatus: domain.QuoteStatusDraft,
	}
	h.rfqs.assigned = claimed

	got, err := h.service.AssignSeller(context.Background(), rfqTenant(), testRFQID)
	if err != nil {
		t.Fatalf("AssignSeller returned %v, want no error", err)
	}
	if got != claimed {
		t.Errorf("AssignSeller returned %v, want the claimed quote %v", got, claimed)
	}
	if len(h.db.scopes) != 1 || h.db.scopes[0] != testAccountID {
		t.Errorf("AssignSeller scoped accounts = %v, want exactly [%v]", h.db.scopes, testAccountID)
	}
}

func TestRFQService_AssignSeller_AdminsNeverClaim(t *testing.T) {
	h := newRFQHarness(nil)
	admin := rfqTenant()
	admin.Role = domain.UserRoleAdmin

	if _, err := h.service.AssignSeller(context.Background(), admin, testRFQID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("AssignSeller as admin = %v, want ErrForbidden", err)
	}
	if len(h.db.scopes) != 0 {
		t.Errorf("AssignSeller as admin opened %d transactions, want none", len(h.db.scopes))
	}
}

func TestRFQService_AssignSeller_ConflictsAndHiddenOrdersSurface(t *testing.T) {
	for name, want := range map[string]error{
		"already claimed": domain.ErrConflict,
		"out of scope":    domain.ErrNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			h := newRFQHarness(nil)
			h.rfqs.assignErr = want

			if _, err := h.service.AssignSeller(context.Background(), rfqTenant(), testRFQID); !errors.Is(err, want) {
				t.Fatalf("AssignSeller = %v, want %v", err, want)
			}
		})
	}
}

// setSellerHarness wires a service whose rfq repository is the assign fake and whose seller
// reach answers the given verdict, so SetSeller tests can steer both sides of the write.
func setSellerHarness(serves bool, sellerErr error) (*RFQService, *fakeRFQDB, *fakeRFQs, *fakeSellerReach) {
	db := &fakeRFQDB{}
	rfqs := &fakeRFQs{}
	reach := &fakeSellerReach{serves: serves, err: sellerErr}
	svc := NewRFQService(db, rfqs, nil, nil, nil, nil, reach, nil, nil, nil, testRFQConfig())
	return svc, db, rfqs, reach
}

// An admin delegates an order to an active seller of the order's own branch: the target is
// checked against that branch and then written, all inside the tenant transaction.
func TestRFQService_SetSeller_AdminReassignsToBranchSeller(t *testing.T) {
	svc, db, rfqs, reach := setSellerHarness(true, nil)
	target := uuid.New()
	rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID, BranchID: testBranchID}
	rfqs.set = &domain.Quote{ID: uuid.New(), RFQID: testRFQID, BranchID: testBranchID, SellerID: &target}

	got, err := svc.SetSeller(context.Background(), adminTenant(), testRFQID, &target)
	if err != nil {
		t.Fatalf("SetSeller returned %v, want no error", err)
	}
	if got != rfqs.set {
		t.Errorf("SetSeller returned %v, want the repo quote %v", got, rfqs.set)
	}
	if len(rfqs.setSellerIDs) != 1 || rfqs.setSellerIDs[0] == nil || *rfqs.setSellerIDs[0] != target {
		t.Fatalf("repo wrote seller ids %v, want exactly [%v]", rfqs.setSellerIDs, target)
	}
	if reach.calls != 1 || len(reach.branchSets[0]) != 1 || reach.branchSets[0][0] != testBranchID {
		t.Errorf("seller check = %d calls on branches %v, want one call on the order's branch",
			reach.calls, reach.branchSets)
	}
	if reach.userIDs[0] != target {
		t.Errorf("seller check user = %v, want %v", reach.userIDs[0], target)
	}
	if len(db.scopes) != 1 || db.scopes[0] != testAccountID {
		t.Errorf("SetSeller scoped accounts = %v, want exactly [%v]", db.scopes, testAccountID)
	}
}

// Assigning the order to themselves never consults the seller reach: the admin is already
// inside the account, so no branch membership check applies.
func TestRFQService_SetSeller_SelfSkipsTheServesCheck(t *testing.T) {
	svc, _, rfqs, reach := setSellerHarness(false, nil)
	rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID, BranchID: testBranchID}
	rfqs.set = &domain.Quote{ID: uuid.New(), RFQID: testRFQID, SellerID: &testUserID}

	if _, err := svc.SetSeller(context.Background(), adminTenant(), testRFQID, &testUserID); err != nil {
		t.Fatalf("self assignment returned %v, want no error", err)
	}
	if reach.calls != 0 {
		t.Errorf("self assignment ran the seller reach %d times, want none", reach.calls)
	}
}

// A nil id leaves the order unassigned, and no branch check runs for a removal.
func TestRFQService_SetSeller_ClearNeverChecksTheSeller(t *testing.T) {
	svc, _, rfqs, reach := setSellerHarness(false, nil)
	rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID, BranchID: testBranchID}
	rfqs.set = &domain.Quote{ID: uuid.New(), RFQID: testRFQID, BranchID: testBranchID}

	if _, err := svc.SetSeller(context.Background(), adminTenant(), testRFQID, nil); err != nil {
		t.Fatalf("clear returned %v, want no error", err)
	}
	if len(rfqs.setSellerIDs) != 1 || rfqs.setSellerIDs[0] != nil {
		t.Fatalf("repo wrote seller ids %v, want exactly [nil]", rfqs.setSellerIDs)
	}
	if reach.calls != 0 {
		t.Errorf("clear ran the seller reach %d times, want none", reach.calls)
	}
}

// Only an admin steers assignments; a seller keeps the self-claim route and nothing else.
func TestRFQService_SetSeller_OnlyAdminsWriteTheSeller(t *testing.T) {
	svc, db, _, _ := setSellerHarness(true, nil)

	if _, err := svc.SetSeller(context.Background(), rfqTenant(), testRFQID, &testUserID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("SetSeller as seller = %v, want ErrForbidden", err)
	}
	if len(db.scopes) != 0 {
		t.Errorf("SetSeller as seller opened %d transactions, want none", len(db.scopes))
	}
}

// A named seller who does not serve the order's branch is refused before anything is written;
// so is one the reach could not examine. Both are the same refusal a manual entry hits.
func TestRFQService_SetSeller_RejectsForeignAndFailedChecks(t *testing.T) {
	for name, reachCfg := range map[string]*fakeSellerReach{
		"does not serve": &fakeSellerReach{serves: false},
		"check failed":   &fakeSellerReach{serves: true, err: errors.New("reach boom")},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _, rfqs, _ := setSellerHarness(reachCfg.serves, reachCfg.err)
			rfqs.rfqByID = &domain.RfqListItem{ID: testRFQID, BranchID: testBranchID}
			foreign := uuid.New()

			_, err := svc.SetSeller(context.Background(), adminTenant(), testRFQID, &foreign)
			if name == "does not serve" && !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("SetSeller = %v, want ErrInvalidInput", err)
			}
			if name == "check failed" && (err == nil || err.Error() != "reach boom") {
				t.Fatalf("SetSeller = %v, want the reach error", err)
			}
			if len(rfqs.setSellerIDs) != 0 {
				t.Errorf("repo wrote seller ids %v, want none on a refused target", rfqs.setSellerIDs)
			}
		})
	}
}

// A hidden order never reaches the seller check: the read refuses first, like every other
// tenant-scoped path.
func TestRFQService_SetSeller_HiddenOrderIsNotFound(t *testing.T) {
	svc, _, rfqs, reach := setSellerHarness(true, nil)
	rfqs.rfqByIDErr = domain.ErrNotFound

	if _, err := svc.SetSeller(context.Background(), adminTenant(), testRFQID, &testUserID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetSeller = %v, want ErrNotFound", err)
	}
	if reach.calls != 0 {
		t.Errorf("hidden order ran the seller reach %d times, want none", reach.calls)
	}
}
