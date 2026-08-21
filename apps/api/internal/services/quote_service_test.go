package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeQuoteDB struct {
	scopes             []uuid.UUID
	activeTransactions int
	transactions       int
}

func (f *fakeQuoteDB) InTenantTx(
	ctx context.Context, tenant domain.Tenant, fn func(repository.Querier) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}
	f.scopes = append(f.scopes, tenant.AccountID)
	f.transactions++
	f.activeTransactions++
	defer func() { f.activeTransactions-- }()
	return fn(nil)
}

type quoteStatusUpdate struct {
	branchID uuid.UUID
	quoteID  uuid.UUID
	from     domain.QuoteStatus
	to       domain.QuoteStatus
}

// fakeQuoteRepo records what the service asked of persistence, so a test can assert on the
// writes without a database.
type fakeQuoteRepo struct {
	quote        *domain.Quote
	version      *domain.QuoteVersion
	items        []domain.QuoteItem
	alternatives map[uuid.UUID][]domain.QuoteItemAlternative

	updateStatusErr error
	applyPricingErr error

	branchesAskedFor []uuid.UUID
	versionBranches  []uuid.UUID
	listedVersionIDs []uuid.UUID
	alternativeReads [][]uuid.UUID
	pricedVersionIDs []uuid.UUID
	appliedPricings  [][]domain.QuoteItemPricing
	writtenTotals    []decimal.Decimal
	statusUpdates    []quoteStatusUpdate
	statusChanges    []quoteStatusChangeCall
	inTransaction    *fakeQuoteDB
	writesInsideTx   []bool
}

func (f *fakeQuoteRepo) GetByID(
	_ context.Context, _ repository.Querier, _, branchID, _ uuid.UUID,
) (*domain.Quote, error) {
	f.branchesAskedFor = append(f.branchesAskedFor, branchID)
	if f.quote == nil {
		return nil, domain.ErrNotFound
	}
	quote := *f.quote
	return &quote, nil
}

func (f *fakeQuoteRepo) UpdateStatus(
	_ context.Context, _ repository.Querier, _, branchID, quoteID uuid.UUID,
	from, to domain.QuoteStatus,
) (*domain.Quote, error) {
	f.statusUpdates = append(f.statusUpdates, quoteStatusUpdate{
		branchID: branchID, quoteID: quoteID, from: from, to: to,
	})
	if f.updateStatusErr != nil {
		return nil, f.updateStatusErr
	}
	updated := *f.quote
	updated.CurrentStatus = to
	return &updated, nil
}

func (f *fakeQuoteRepo) GetCurrentVersion(
	_ context.Context, _ repository.Querier, _, branchID, _ uuid.UUID,
) (*domain.QuoteVersion, error) {
	f.versionBranches = append(f.versionBranches, branchID)
	if f.version == nil {
		return nil, domain.ErrNotFound
	}
	version := *f.version
	return &version, nil
}

func (f *fakeQuoteRepo) UpdateVersionTotal(
	_ context.Context, _ repository.Querier, _, versionID uuid.UUID, total decimal.Decimal,
) (*domain.QuoteVersion, error) {
	f.writtenTotals = append(f.writtenTotals, total)
	updated := *f.version
	updated.ID = versionID
	updated.Total = total
	return &updated, nil
}

func (f *fakeQuoteRepo) ListItems(
	_ context.Context, _ repository.Querier, _, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	f.listedVersionIDs = append(f.listedVersionIDs, versionID)
	items := make([]domain.QuoteItem, len(f.items))
	copy(items, f.items)
	return items, nil
}

func (f *fakeQuoteRepo) ListAlternativesByItemIDs(
	_ context.Context, _ repository.Querier, _ uuid.UUID, itemIDs []uuid.UUID,
) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	f.alternativeReads = append(f.alternativeReads, itemIDs)
	byItem := make(map[uuid.UUID][]domain.QuoteItemAlternative, len(f.alternatives))
	for _, itemID := range itemIDs {
		if offered, ok := f.alternatives[itemID]; ok {
			byItem[itemID] = offered
		}
	}
	return byItem, nil
}

func (f *fakeQuoteRepo) ApplyPricing(
	_ context.Context, _ repository.Querier, _, versionID uuid.UUID,
	pricings []domain.QuoteItemPricing,
) error {
	f.pricedVersionIDs = append(f.pricedVersionIDs, versionID)
	f.appliedPricings = append(f.appliedPricings, pricings)
	if f.inTransaction != nil {
		f.writesInsideTx = append(f.writesInsideTx, f.inTransaction.activeTransactions > 0)
	}
	return f.applyPricingErr
}

func (f *fakeQuoteRepo) AppendStatusChange(
	_ context.Context, _ repository.Querier, _, quoteID uuid.UUID,
	previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID,
) (*domain.QuoteStatusChange, error) {
	f.statusChanges = append(f.statusChanges, quoteStatusChangeCall{
		quoteID: quoteID, previousStatus: previousStatus, newStatus: newStatus, userID: userID,
	})
	return &domain.QuoteStatusChange{QuoteID: quoteID, NewStatus: newStatus}, nil
}

type fakeBranchPrices struct {
	prices   map[uuid.UUID]domain.BranchPrice
	err      error
	calls    int
	branches []uuid.UUID
	askedFor [][]uuid.UUID
}

func (f *fakeBranchPrices) GetCurrentByProductIDs(
	_ context.Context, _ repository.Querier, _, branchID uuid.UUID, productIDs []uuid.UUID,
) (map[uuid.UUID]domain.BranchPrice, error) {
	f.calls++
	f.branches = append(f.branches, branchID)
	f.askedFor = append(f.askedFor, productIDs)
	if f.err != nil {
		return nil, f.err
	}
	prices := make(map[uuid.UUID]domain.BranchPrice, len(f.prices))
	for id, price := range f.prices {
		prices[id] = price
	}
	return prices, nil
}

type quoteFixture struct {
	service  *QuoteService
	db       *fakeQuoteDB
	quotes   *fakeQuoteRepo
	prices   *fakeBranchPrices
	tenant   domain.Tenant
	quoteID  uuid.UUID
	branchID uuid.UUID
}

// newQuoteFixture stages a DRAFT quote whose current version holds the lines given.
func newQuoteFixture(items []domain.QuoteItem, prices map[uuid.UUID]domain.BranchPrice) quoteFixture {
	accountID, branchID, sellerID := uuid.New(), uuid.New(), uuid.New()
	quoteID, versionID := uuid.New(), uuid.New()
	db := &fakeQuoteDB{}
	quotes := &fakeQuoteRepo{
		quote: &domain.Quote{
			ID: quoteID, AccountID: accountID, BranchID: branchID, RFQID: uuid.New(),
			CurrentVersionID: &versionID, CurrentStatus: domain.QuoteStatusDraft,
		},
		version: &domain.QuoteVersion{
			ID: versionID, AccountID: accountID, QuoteID: quoteID, VersionNumber: 1,
			Total: decimal.Zero, IsImmutable: false,
		},
		items:         items,
		inTransaction: db,
	}
	branchPrices := &fakeBranchPrices{prices: prices}
	return quoteFixture{
		service: NewQuoteService(db, quotes, branchPrices,
			slog.New(slog.NewTextHandler(io.Discard, nil))),
		db:     db,
		quotes: quotes,
		prices: branchPrices,
		tenant: domain.Tenant{
			AccountID: accountID, BranchID: branchID, UserID: sellerID,
			Role: domain.UserRoleSeller,
		},
		quoteID:  quoteID,
		branchID: branchID,
	}
}

func TestQuoteService_AcceptMaterials_FreezesPricesAndMovesToQuoted(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	floor := "900.00"
	items := []domain.QuoteItem{pricedLine(product, "10")}
	f := newQuoteFixture(items, map[uuid.UUID]domain.BranchPrice{
		product: branchPrice(product, "1200.50", &floor),
	})

	priced, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	if priced.Quote.CurrentStatus != domain.QuoteStatusQuoted {
		t.Errorf("status = %q, want QUOTED", priced.Quote.CurrentStatus)
	}
	// 10 × 1200.50, by hand.
	if !priced.Version.Total.Equal(decimal.RequireFromString("12005.00")) {
		t.Errorf("total = %s, want 12005.00", priced.Version.Total)
	}
	// The version is not frozen by pricing: the seller still edits the draft, and freezing
	// belongs to sending it.
	if priced.Version.IsImmutable {
		t.Error("version is immutable, want a draft the seller can still edit")
	}
	if len(priced.Items) != 1 {
		t.Fatalf("returned %d lines, want 1", len(priced.Items))
	}
	assertAmount(t, "line unit price", priced.Items[0].UnitPriceSnapshot, "1200.50")
	assertAmount(t, "line floor", priced.Items[0].MinPriceSnapshot, "900.00")
	assertAmount(t, "line subtotal", priced.Items[0].Subtotal, "12005.00")

	// Everything reads and writes the version the quote points at, not some other version of it.
	versionID := *f.quotes.quote.CurrentVersionID
	if len(f.quotes.listedVersionIDs) != 1 || f.quotes.listedVersionIDs[0] != versionID {
		t.Errorf("listed versions %v, want [%v]", f.quotes.listedVersionIDs, versionID)
	}
	if len(f.quotes.pricedVersionIDs) != 1 || f.quotes.pricedVersionIDs[0] != versionID {
		t.Errorf("priced versions %v, want [%v]", f.quotes.pricedVersionIDs, versionID)
	}

	// The whole transition is one transaction: half-written, the lines carry prices while the
	// status still says they were never accepted.
	if f.db.transactions != 1 {
		t.Errorf("transactions = %d, want 1", f.db.transactions)
	}
	for i, inside := range f.quotes.writesInsideTx {
		if !inside {
			t.Errorf("write %d ran outside the transaction", i)
		}
	}
	if len(f.quotes.statusUpdates) != 1 {
		t.Fatalf("status updates = %d, want 1", len(f.quotes.statusUpdates))
	}
	update := f.quotes.statusUpdates[0]
	if update.from != domain.QuoteStatusDraft || update.to != domain.QuoteStatusQuoted {
		t.Errorf("status update = %q to %q, want DRAFT to QUOTED", update.from, update.to)
	}
	if len(f.quotes.statusChanges) != 1 {
		t.Fatalf("status changes = %d, want 1", len(f.quotes.statusChanges))
	}
	change := f.quotes.statusChanges[0]
	if change.previousStatus == nil || *change.previousStatus != domain.QuoteStatusDraft {
		t.Errorf("history previous = %v, want DRAFT", change.previousStatus)
	}
	if change.newStatus != domain.QuoteStatusQuoted {
		t.Errorf("history new = %q, want QUOTED", change.newStatus)
	}
	if change.userID == nil || *change.userID != f.tenant.UserID {
		t.Errorf("history user = %v, want the caller %v", change.userID, f.tenant.UserID)
	}
}

func TestQuoteService_AcceptMaterials_PricesEveryLineInOneQuery(t *testing.T) {
	t.Parallel()
	first, second := uuid.New(), uuid.New()
	items := []domain.QuoteItem{
		pricedLine(first, "1"), pricedLine(second, "2"), flaggedLine("3"), pricedLine(first, "4"),
	}
	f := newQuoteFixture(items, map[uuid.UUID]domain.BranchPrice{
		first:  branchPrice(first, "10.00", nil),
		second: branchPrice(second, "20.00", nil),
	})

	if _, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID); err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	// One lookup for four lines. A call per line is the N+1 that turns a 200-line order into 200
	// round trips.
	if f.prices.calls != 1 {
		t.Fatalf("price lookups = %d, want 1 for the whole quote", f.prices.calls)
	}
	if got := len(f.prices.askedFor[0]); got != 2 {
		t.Errorf("asked for %d products, want 2: the repeat and the flagged line add none", got)
	}
	// The price belongs to the branch the order arrived at, which is the quote's own.
	if f.prices.branches[0] != f.branchID {
		t.Errorf("priced against branch %v, want the quote's %v", f.prices.branches[0], f.branchID)
	}

	// Every line is written, the empty ones included: the row count is what proves the
	// account-scoped join matched, so a line left out of the payload would go unchecked.
	if len(f.quotes.appliedPricings) != 1 {
		t.Fatalf("pricing writes = %d, want 1", len(f.quotes.appliedPricings))
	}
	if got := len(f.quotes.appliedPricings[0]); got != len(items) {
		t.Errorf("wrote %d pricings, want %d — one per line", got, len(items))
	}
	if len(f.quotes.writtenTotals) != 1 {
		t.Fatalf("total writes = %d, want 1", len(f.quotes.writtenTotals))
	}
	// 1×10 + 2×20 + 4×10, by hand; the flagged line contributes nothing.
	if got := f.quotes.writtenTotals[0]; !got.Equal(decimal.RequireFromString("90.00")) {
		t.Errorf("total = %s, want 90.00", got)
	}
}

func TestQuoteService_AcceptMaterials_RefusesAQuoteThatIsNotADraft(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   domain.QuoteStatus
		archived bool
		wantCode domain.ErrorCode
	}{
		{"already quoted", domain.QuoteStatusQuoted, false, domain.CodeQuoteNotDraft},
		{"sent", domain.QuoteStatusSent, false, domain.CodeQuoteNotDraft},
		{"change requested", domain.QuoteStatusChangeRequested, false, domain.CodeQuoteNotDraft},
		{"accepted", domain.QuoteStatusAccepted, false, domain.CodeQuoteNotDraft},
		{"rejected", domain.QuoteStatusRejected, false, domain.CodeQuoteNotDraft},
		{"archived draft", domain.QuoteStatusDraft, true, domain.CodeQuoteArchived},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			product := uuid.New()
			f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1")},
				map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})
			f.quotes.quote.CurrentStatus = tc.status
			if tc.archived {
				archivedAt := time.Now()
				f.quotes.quote.ArchivedAt = &archivedAt
			}

			_, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("AcceptMaterials() = %v, want ErrConflict", err)
			}
			if code := domain.CodeOf(err); code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
			// Validated before anything is written: the matrix decides whether the transition
			// may happen, not whether it can be undone afterwards.
			if len(f.quotes.appliedPricings) != 0 || len(f.quotes.writtenTotals) != 0 ||
				len(f.quotes.statusUpdates) != 0 || len(f.quotes.statusChanges) != 0 {
				t.Error("the refused transition still wrote something")
			}
			if f.prices.calls != 0 {
				t.Errorf("price lookups = %d, want none before the state is validated",
					f.prices.calls)
			}
		})
	}
}

func TestQuoteService_AcceptMaterials_RefusesAQuoteThatLeftDraftMidTransition(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1")},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})
	// What a second concurrent call sees: the read said DRAFT, the conditional update found the
	// row already moved on.
	f.quotes.updateStatusErr = domain.ErrConflict

	_, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("AcceptMaterials() = %v, want ErrConflict", err)
	}
	// Same refusal as the up-front check, so the seller reads one message either way.
	if code := domain.CodeOf(err); code != domain.CodeQuoteNotDraft {
		t.Errorf("code = %q, want %q", code, domain.CodeQuoteNotDraft)
	}
	// And the history is not appended for a transition that did not happen.
	if len(f.quotes.statusChanges) != 0 {
		t.Errorf("status changes = %d, want none", len(f.quotes.statusChanges))
	}
}

func TestQuoteService_AcceptMaterials_KeepsAFlaggedLineWithNothingFrozenOnIt(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	items := []domain.QuoteItem{pricedLine(product, "3"), flaggedLine("8")}
	f := newQuoteFixture(items, map[uuid.UUID]domain.BranchPrice{
		product: branchPrice(product, "150.00", nil),
	})

	priced, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	if len(priced.Items) != 2 {
		t.Fatalf("kept %d lines, want both", len(priced.Items))
	}
	flagged := priced.Items[1]
	if flagged.UnitPriceSnapshot.Valid || flagged.MinPriceSnapshot.Valid || flagged.Subtotal.Valid {
		t.Errorf("flagged line = %+v, want nothing frozen on it", flagged)
	}
	if flagged.MatchStatus != domain.ItemMatchStatusNoMatch {
		t.Errorf("flagged status = %q, want NO_MATCH", flagged.MatchStatus)
	}
	// 3 × 150.00, by hand: the flagged line is neither dropped nor valued at zero.
	if !priced.Version.Total.Equal(decimal.RequireFromString("450.00")) {
		t.Errorf("total = %s, want 450.00", priced.Version.Total)
	}
}

func TestQuoteService_AcceptMaterials_NeedsAnActiveBranch(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1")},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})
	// An admin reaching the whole account has no branch selected, and a price belongs to one
	// branch: guessing which would freeze the wrong number.
	f.tenant.BranchID = uuid.Nil
	f.tenant.Role = domain.UserRoleAdmin

	_, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AcceptMaterials() = %v, want ErrInvalidInput", err)
	}
	if f.db.transactions != 0 {
		t.Errorf("transactions = %d, want none: the request never reached the database",
			f.db.transactions)
	}
}

func TestQuoteService_AcceptMaterials_ScopesTheReadToTheCallersAccountAndBranch(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1")},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})

	if _, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID); err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	if len(f.db.scopes) != 1 || f.db.scopes[0] != f.tenant.AccountID {
		t.Errorf("transaction scopes = %v, want [%v]", f.db.scopes, f.tenant.AccountID)
	}
	// Row level security guards the account boundary only, so the branch has to be in the
	// predicate or a caller reads another branch of their own account. Every call that takes the
	// request's quote id carries it, the write included.
	if len(f.quotes.branchesAskedFor) != 1 || f.quotes.branchesAskedFor[0] != f.branchID {
		t.Errorf("read scoped to branches %v, want [%v]", f.quotes.branchesAskedFor, f.branchID)
	}
	if len(f.quotes.versionBranches) != 1 || f.quotes.versionBranches[0] != f.branchID {
		t.Errorf("version read scoped to branches %v, want [%v]", f.quotes.versionBranches,
			f.branchID)
	}
	if len(f.quotes.statusUpdates) != 1 {
		t.Fatalf("status writes = %d, want 1", len(f.quotes.statusUpdates))
	}
	if f.quotes.statusUpdates[0].branchID != f.branchID {
		t.Errorf("status write scoped to branch %v, want %v",
			f.quotes.statusUpdates[0].branchID, f.branchID)
	}
}

func TestQuoteService_AcceptMaterials_StopsWhenPricesCannotBeRead(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1")}, nil)
	f.prices.err = errors.New("connection reset")

	if _, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID); err == nil {
		t.Fatal("AcceptMaterials() = nil, want the read error")
	}
	// Nothing is written on a price list nobody could read: a quote valued at zero because the
	// lookup failed is the one outcome worse than no quote.
	if len(f.quotes.appliedPricings) != 0 || len(f.quotes.statusUpdates) != 0 {
		t.Error("the failed lookup still moved the quote on")
	}
}

func TestQuoteService_AcceptMaterials_LeavesTheStatusAloneWhenAWriteFails(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "2")},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "50.00", nil)})
	f.quotes.applyPricingErr = errors.New("deadlock detected")

	if _, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID); err == nil {
		t.Fatal("AcceptMaterials() = nil, want the write error")
	}
	// A quote whose status says the prices were accepted while its lines carry none is the one
	// outcome the single transaction exists to prevent.
	if len(f.quotes.statusUpdates) != 0 || len(f.quotes.statusChanges) != 0 {
		t.Errorf("status updates = %d and history rows = %d, want none",
			len(f.quotes.statusUpdates), len(f.quotes.statusChanges))
	}
}

func TestQuoteService_AcceptMaterials_QuotesAtZeroWhenNoLineCanBePriced(t *testing.T) {
	t.Parallel()
	// What the pipeline leaves when matching could not run at all: every line flagged. The seller
	// still accepts the materials, and the arithmetic over lines that contribute nothing is zero.
	// Refusing would block a transition the rules allow; the flagged lines are what they review.
	f := newQuoteFixture([]domain.QuoteItem{flaggedLine("4"), flaggedLine("9")}, nil)

	priced, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}
	if !priced.Version.Total.IsZero() {
		t.Errorf("total = %s, want 0", priced.Version.Total)
	}
	if priced.Quote.CurrentStatus != domain.QuoteStatusQuoted {
		t.Errorf("status = %q, want QUOTED", priced.Quote.CurrentStatus)
	}
	if len(priced.Items) != 2 {
		t.Errorf("kept %d lines, want both", len(priced.Items))
	}
	// No product on any line, so there is no unpriced product to report either.
	if f.prices.calls != 1 || len(f.prices.askedFor[0]) != 0 {
		t.Errorf("price lookup asked for %v, want one call for no products", f.prices.askedFor)
	}
}

func TestQuoteService_AcceptMaterials_NamesTheLinesTheBranchCannotPrice(t *testing.T) {
	t.Parallel()
	priced, unpriceable := uuid.New(), uuid.New()
	// Three lines, three reasons a valuation can be empty: one priced, one whose product the
	// branch has no price in force for, and one that matched nothing at all.
	pricedItem := pricedLine(priced, "3")
	unpriceableItem := pricedLine(unpriceable, "5")
	flagged := flaggedLine("2")
	f := newQuoteFixture([]domain.QuoteItem{pricedItem, unpriceableItem, flagged},
		map[uuid.UUID]domain.BranchPrice{priced: branchPrice(priced, "80.00", nil)})

	result, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	// Only the matched-but-unpriceable line is named. A line with no product is already flagged
	// NO_MATCH, so reporting it again would say the same thing twice; the priced one is fine.
	if len(result.UnpricedItemIDs) != 1 {
		t.Fatalf("named %v, want the one line the branch cannot price", result.UnpricedItemIDs)
	}
	if result.UnpricedItemIDs[0] != unpriceableItem.ID {
		t.Errorf("named line %v, want %v", result.UnpricedItemIDs[0], unpriceableItem.ID)
	}
	// It is neither NO_MATCH nor AMBIGUOUS: the catalog decided it, and the gap is elsewhere.
	if result.Items[1].MatchStatus != domain.ItemMatchStatusMatched {
		t.Errorf("unpriceable line status = %q, want MATCHED: match_status answers a different "+
			"question", result.Items[1].MatchStatus)
	}
	if result.Items[1].Subtotal.Valid {
		t.Errorf("unpriceable line subtotal = %v, want none", result.Items[1].Subtotal)
	}
	// 3 × 80.00, by hand: neither empty line contributes.
	if !result.Version.Total.Equal(decimal.RequireFromString("240.00")) {
		t.Errorf("total = %s, want 240.00", result.Version.Total)
	}
}

func TestQuoteService_AcceptMaterials_NamesNoLineWhenEveryPriceIsInForce(t *testing.T) {
	t.Parallel()
	product := uuid.New()
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1"), flaggedLine("2")},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})

	result, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}
	if len(result.UnpricedItemIDs) != 0 {
		t.Errorf("named %v, want none: a line with no product is not a pricing gap",
			result.UnpricedItemIDs)
	}
}

func TestQuoteService_AcceptMaterials_CarriesTheFlaggedLinesCandidates(t *testing.T) {
	t.Parallel()
	product, candidate := uuid.New(), uuid.New()
	flagged := flaggedLine("2")
	name := "Membrana asfáltica 4mm"
	f := newQuoteFixture([]domain.QuoteItem{pricedLine(product, "1"), flagged},
		map[uuid.UUID]domain.BranchPrice{product: branchPrice(product, "10.00", nil)})
	f.quotes.alternatives = map[uuid.UUID][]domain.QuoteItemAlternative{
		flagged.ID: {{
			ID: uuid.New(), QuoteItemID: flagged.ID, ProductID: &candidate,
			Type:   domain.QuoteItemAlternativeTypeProduct,
			Origin: domain.QuoteItemAlternativeOriginAI, Rank: 1,
			ConfidenceScore: decimal.NewNullDecimal(decimal.RequireFromString("0.5500")),
			CanonicalName:   &name,
		}},
	}

	result, err := f.service.AcceptMaterials(context.Background(), f.tenant, f.quoteID)
	if err != nil {
		t.Fatalf("AcceptMaterials() = %v, want no error", err)
	}

	// Valuation does not change which line is flagged, so the seller reviews the prices and the
	// choices on one screen rather than fetching the candidates separately.
	offered := result.Alternatives[flagged.ID]
	if len(offered) != 1 {
		t.Fatalf("flagged line offers %d candidates, want the one that was considered",
			len(offered))
	}
	if offered[0].CanonicalName == nil || *offered[0].CanonicalName != name {
		t.Errorf("offer name = %v, want %q: a bare id is barely better than a bare flag",
			offered[0].CanonicalName, name)
	}
	// One read for every line, not one per line.
	if len(f.quotes.alternativeReads) != 1 {
		t.Fatalf("read candidates %d times, want once for the whole version",
			len(f.quotes.alternativeReads))
	}
	if got := len(f.quotes.alternativeReads[0]); got != 2 {
		t.Errorf("asked for %d lines, want both of them", got)
	}
}
