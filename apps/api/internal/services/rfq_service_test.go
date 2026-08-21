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

func (f *fakeRfqRepoManual) ListByTenant(
	_ context.Context, _ repository.Querier, _ domain.Tenant,
) ([]domain.RfqListItem, error) {
	return f.listItems, nil
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

func manualHarness(repo *fakeRfqRepoManual) (*RFQService, *fakeDB) {
	db := &fakeDB{}
	svc := NewRFQService(db, repo, nil, nil, nil, nil, nil, config.RFQConfig{})
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

// ---------- AI pipeline fakes & tests ----------

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

func (f *fakeRFQs) ListByTenant(
	_ context.Context, _ repository.Querier, _ domain.Tenant,
) ([]domain.RfqListItem, error) {
	return nil, errors.New("not implemented in pipeline fake")
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
	created            []domain.NewQuote
	currentVersion     []uuid.UUID
	versions           []domain.NewQuoteVersion
	itemBatches        [][]domain.NewQuoteItem
	alternativeBatches [][]domain.NewQuoteItemAlternative
	alternativeReads   [][]uuid.UUID
	storedAlternatives []domain.QuoteItemAlternative
	alternativesErr    error
	statusChanges      []quoteStatusChangeCall
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

type rfqHarness struct {
	service   *RFQService
	db        *fakeRFQDB
	extractor *fakeRFQExtractor
	matcher   *fakeCatalogMatcher
	rfqs      *fakeRFQs
	quotes    *fakeQuoteDrafts
	channels  *fakeRFQChannels
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
	if draft.RFQ.Status != domain.RFQStatusReceived {
		t.Errorf("RFQ status %q, want RECEIVED", draft.RFQ.Status)
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
