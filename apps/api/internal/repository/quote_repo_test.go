//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These tests exercise the SQL the valorization runs: what "the price in force" selects, and the
// guards a fake cannot stand in for — the account and branch predicates on the pricing write and
// the status update, and the conditional update that makes the transition atomic.

// seedQuoteChain writes an order and a DRAFT quote with one unpriced line, and takes the whole
// chain away afterwards, children before parents.
func seedQuoteChain(
	t *testing.T, db *DB, accountID, branchID, productID uuid.UUID,
) (quoteID, versionID, itemID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	channelID, rfqID := uuid.New(), uuid.New()
	quoteID, versionID, itemID = uuid.New(), uuid.New(), uuid.New()

	for _, seed := range []struct {
		what string
		sql  string
		args []any
	}{
		{"channel", `INSERT INTO channel (id, account_id, branch_id, type, identifier)
		             VALUES ($1, $2, $3, 'WHATSAPP', $4)`,
			[]any{channelID, accountID, branchID, uuid.NewString()}},
		{"rfq", `INSERT INTO rfq (id, account_id, branch_id, channel_id, status)
		         VALUES ($1, $2, $3, $4, 'GENERATED')`,
			[]any{rfqID, accountID, branchID, channelID}},
		{"quote", `INSERT INTO quote (id, account_id, branch_id, rfq_id, current_status)
		           VALUES ($1, $2, $3, $4, 'DRAFT')`,
			[]any{quoteID, accountID, branchID, rfqID}},
		{"quote_version", `INSERT INTO quote_version (id, account_id, quote_id, version_number)
		                   VALUES ($1, $2, $3, 1)`,
			[]any{versionID, accountID, quoteID}},
		{"quote_item", `INSERT INTO quote_item (id, account_id, version_id, product_id,
		                                        requested_description, quantity, match_status)
		                VALUES ($1, $2, $3, $4, 'cemento', 10, 'MATCHED')`,
			[]any{itemID, accountID, versionID, productID}},
		{"current version", `UPDATE quote SET current_version_id = $2 WHERE id = $1`,
			[]any{quoteID, versionID}},
	} {
		if _, err := db.CrossAccount().Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed %s: %v", seed.what, err)
		}
	}

	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_item WHERE version_id = $1`, versionID)
		mustCleanup(t, db.CrossAccount(),
			`UPDATE quote SET current_version_id = NULL WHERE id = $1`, quoteID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_version WHERE id = $1`, versionID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_status_change WHERE quote_id = $1`, quoteID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote WHERE id = $1`, quoteID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM rfq WHERE id = $1`, rfqID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM channel WHERE id = $1`, channelID)
	})
	return quoteID, versionID, itemID
}

// carryAtBranch makes the branch stock the product, which every current-price query requires: a
// price on a product the branch does not carry is not a price at that branch.
func carryAtBranch(t *testing.T, db *DB, accountID, branchID, productID uuid.UUID, active bool) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch_product (account_id, branch_id, product_id, is_active)
		 VALUES ($1, $2, $3, $4)`, accountID, branchID, productID, active); err != nil {
		t.Fatalf("seed branch_product: %v", err)
	}
}

func insertPricePeriod(
	t *testing.T, db *DB, accountID, branchID, productID uuid.UUID,
	price string, minPrice *string, validFrom time.Time, validTo *time.Time,
) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product_price (account_id, branch_id, product_id, price, min_price,
		                            valid_from, valid_to)
		 VALUES ($1, $2, $3, $4::numeric, $5::numeric, $6, $7)`,
		accountID, branchID, productID, price, minPrice, validFrom, validTo); err != nil {
		t.Fatalf("seed price period: %v", err)
	}
}

// The predicate the whole valorization rests on: of a product's periods, exactly the one that has
// started and not ended is in force, and a null floor comes back null rather than zero.
func TestProductPriceRepository_GetCurrentByProductIDs_ReadsOnlyThePeriodInForce(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Price in force")
	branchID := branchOf(t, db, accountID)
	current := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	expired := seedProduct(t, db, accountID, "Arena fina")
	future := seedProduct(t, db, accountID, "Cal hidratada")
	floored := seedProduct(t, db, accountID, "Hierro del 8")
	unpriced := seedProduct(t, db, accountID, "Membrana liquida")
	withdrawn := seedProduct(t, db, accountID, "Ladrillo hueco discontinuado")
	unstocked := seedProduct(t, db, accountID, "Pinotea")
	every := []uuid.UUID{current, expired, future, floored, unpriced, withdrawn, unstocked}
	for _, id := range every {
		priceCleanup(t, db, id)
		carryAtBranch(t, db, accountID, branchID, id, id != unstocked)
	}
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE product SET is_active = FALSE WHERE id = $1`, withdrawn); err != nil {
		t.Fatalf("deactivate product: %v", err)
	}

	now := time.Now()
	// Two periods in force at the same moment — an older one that closes in the future, and the
	// open one that started after it. The predicate cannot separate them, so the newest
	// valid_from is what decides; with only one in force that ordering would go unproven.
	insertPricePeriod(t, db, accountID, branchID, current, "900.00", nil,
		now.Add(-48*time.Hour), ptrTime(now.Add(48*time.Hour)))
	insertPricePeriod(t, db, accountID, branchID, current, "1200.50", nil,
		now.Add(-24*time.Hour), nil)
	// Closed and never replaced: the branch has no price for this product now.
	insertPricePeriod(t, db, accountID, branchID, expired, "500.00", nil,
		now.Add(-48*time.Hour), ptrTime(now.Add(-time.Hour)))
	// Agreed for next week: not in force yet.
	insertPricePeriod(t, db, accountID, branchID, future, "700.00", nil,
		now.Add(24*time.Hour), nil)
	floor := "800.00"
	insertPricePeriod(t, db, accountID, branchID, floored, "1000.00", &floor,
		now.Add(-time.Hour), nil)
	// Both priced and both in force, but the branch cannot sell either: one product is
	// deactivated, the other the branch no longer carries.
	insertPricePeriod(t, db, accountID, branchID, withdrawn, "300.00", nil, now.Add(-time.Hour), nil)
	insertPricePeriod(t, db, accountID, branchID, unstocked, "400.00", nil, now.Add(-time.Hour), nil)

	repo := NewProductPriceRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	var prices map[uuid.UUID]domain.BranchPrice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		prices, err = repo.GetCurrentByProductIDs(ctx, q, accountID, branchID, every)
		return err
	}); err != nil {
		t.Fatalf("GetCurrentByProductIDs() = %v, want no error", err)
	}

	if len(prices) != 2 {
		t.Fatalf("priced %d products, want 2: only the open periods are in force, got %+v",
			len(prices), prices)
	}
	if got, ok := prices[current]; !ok ||
		!got.Price.Equal(decimal.RequireFromString("1200.50")) {
		t.Errorf("current = %+v (found %v), want 1200.50 — the newer of the two in force", got, ok)
	}
	// Null, not zero: read as zero, a discount could take the line to nothing.
	if prices[current].MinPrice.Valid {
		t.Errorf("floor = %s, want null", prices[current].MinPrice.Decimal)
	}
	if got := prices[floored]; !got.MinPrice.Valid ||
		!got.MinPrice.Decimal.Equal(decimal.RequireFromString("800.00")) {
		t.Errorf("floor = %+v, want 800.00", got.MinPrice)
	}
	for name, id := range map[string]uuid.UUID{
		"a period that ended": expired, "a period that has not started": future,
		"a product never priced": unpriced, "a deactivated product": withdrawn,
		"a product the branch stopped carrying": unstocked,
	} {
		if _, ok := prices[id]; ok {
			t.Errorf("%s is in force, want absent from the map", name)
		}
	}
}

// The guard PR #82 found the absence of twice: a foreign key resolves across accounts, so a
// statement has to filter by the account itself and count what it matched, or it writes into
// another tenant's version or reports success having written nothing.
func TestQuoteRepository_ApplyPricing_RefusesAnotherAccountsVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Pricing victim")
	intruderAccount := seedAccount(t, db, "Pricing intruder")
	victimBranch := branchOf(t, db, victimAccount)
	victimProduct := seedProduct(t, db, victimAccount, "Cemento Portland 50kg")
	priceCleanup(t, db, victimProduct)
	_, victimVersion, victimItem := seedQuoteChain(t, db, victimAccount, victimBranch, victimProduct)

	repo := NewQuoteRepository()
	pricings := []domain.QuoteItemPricing{{
		ItemID:            victimItem,
		UnitPriceSnapshot: decimal.NewNullDecimal(decimal.RequireFromString("1.00")),
		Subtotal:          decimal.NewNullDecimal(decimal.RequireFromString("10.00")),
	}}

	// The intruder names a real version and a real line, and its own account. Row level security
	// alone would let the statement run and match nothing quietly.
	err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: intruderAccount, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			return repo.ApplyPricing(ctx, q, intruderAccount, victimVersion, pricings)
		})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ApplyPricing() across accounts = %v, want ErrNotFound", err)
	}

	var unitPrice, subtotal decimal.NullDecimal
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT unit_price_snapshot, subtotal FROM quote_item WHERE id = $1`,
		victimItem).Scan(&unitPrice, &subtotal); err != nil {
		t.Fatalf("read the victim's line back: %v", err)
	}
	if unitPrice.Valid || subtotal.Valid {
		t.Errorf("the victim's line = (%v, %v), want untouched", unitPrice, subtotal)
	}
}

// Row level security refuses another account's version before either application predicate is
// reached, so inside a tenant transaction the two cannot be told apart. Running the same statement
// on the owner pool, which is RLS-exempt, leaves the application predicates as the only guard.
// Which of the two refuses is deliberately not pinned: the join and the line predicate each do it
// alone, so removing either one keeps this green. What is pinned is that the statement refuses
// without the database's help.
func TestQuoteRepository_ApplyPricing_RefusesWithoutRowLevelSecurity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	victimAccount := seedAccount(t, db, "Pricing victim no RLS")
	intruderAccount := seedAccount(t, db, "Pricing intruder no RLS")
	victimBranch := branchOf(t, db, victimAccount)
	victimProduct := seedProduct(t, db, victimAccount, "Cemento Portland 50kg")
	priceCleanup(t, db, victimProduct)
	_, victimVersion, victimItem := seedQuoteChain(t, db, victimAccount, victimBranch, victimProduct)

	repo := NewQuoteRepository()
	priced := decimal.NewNullDecimal(decimal.RequireFromString("10.00"))
	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("AdminTx() = %v, want no error", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := repo.ApplyPricing(ctx, tx, intruderAccount, victimVersion,
		[]domain.QuoteItemPricing{{
			ItemID: victimItem, UnitPriceSnapshot: priced, Subtotal: priced,
		}}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ApplyPricing() on the owner pool = %v, want ErrNotFound", err)
	}
}

// A line the payload names that does not belong to the version is refused for the same reason,
// which is what the row count is checking rather than the join alone.
func TestQuoteRepository_ApplyPricing_RefusesALineOutsideTheVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Pricing stray line")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	_, versionID, itemID := seedQuoteChain(t, db, accountID, branchID, productID)
	_, _, strayItem := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	priced := decimal.NewNullDecimal(decimal.RequireFromString("5.00"))
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyPricing(ctx, q, accountID, versionID, []domain.QuoteItemPricing{
			{ItemID: itemID, UnitPriceSnapshot: priced, Subtotal: priced},
			{ItemID: strayItem, UnitPriceSnapshot: priced, Subtotal: priced},
		})
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ApplyPricing() with a stray line = %v, want ErrNotFound", err)
	}

	// Rolled back with the transaction, so the version it did belong to is untouched too.
	var unitPrice decimal.NullDecimal
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT unit_price_snapshot FROM quote_item WHERE id = $1`, itemID).Scan(&unitPrice); err != nil {
		t.Fatalf("read the line back: %v", err)
	}
	if unitPrice.Valid {
		t.Errorf("unit price = %s, want null: the refused batch wrote nothing", unitPrice.Decimal)
	}
}

// What makes the transition atomic: two callers who both read DRAFT cannot both write QUOTED, so
// the history cannot record a previous status the quote had already left.
func TestQuoteRepository_UpdateStatus_OnlyMovesFromTheStatusRead(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Status guard")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}

	var quote *domain.Quote
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		quote, err = repo.UpdateStatus(ctx, q, accountID, branchID, quoteID,
			domain.QuoteStatusDraft, domain.QuoteStatusQuoted)
		return err
	}); err != nil {
		t.Fatalf("first UpdateStatus() = %v, want no error", err)
	}
	if quote.CurrentStatus != domain.QuoteStatusQuoted {
		t.Fatalf("status = %q, want QUOTED", quote.CurrentStatus)
	}

	// The second caller still believes it read DRAFT. The row says otherwise.
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, updateErr := repo.UpdateStatus(ctx, q, accountID, branchID, quoteID,
			domain.QuoteStatusDraft, domain.QuoteStatusQuoted)
		return updateErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second UpdateStatus() = %v, want ErrConflict", err)
	}

	// And another account cannot move it at all.
	intruderAccount := seedAccount(t, db, "Status guard intruder")
	err = db.InTenantTx(ctx, domain.Tenant{AccountID: intruderAccount, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			_, updateErr := repo.UpdateStatus(ctx, q, intruderAccount, branchID, quoteID,
				domain.QuoteStatusQuoted, domain.QuoteStatusSent)
			return updateErr
		})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("UpdateStatus() across accounts = %v, want ErrConflict", err)
	}
	var status string
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT current_status FROM quote WHERE id = $1`, quoteID).Scan(&status); err != nil {
		t.Fatalf("read the quote back: %v", err)
	}
	if status != string(domain.QuoteStatusQuoted) {
		t.Errorf("status = %q, want QUOTED", status)
	}
}

// A quote is branch-scoped and row level security guards only the account boundary, so the branch
// predicate is the only thing keeping one branch of an account out of another's quotes.
func TestQuoteRepository_GetByID_NarrowsToTheQuotesOwnBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Quote branch scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Norte")
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}
	read := func(branch uuid.UUID) (*domain.Quote, error) {
		var quote *domain.Quote
		err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var getErr error
			quote, getErr = repo.GetByID(ctx, q, accountID, branch, quoteID)
			return getErr
		})
		return quote, err
	}

	quote, err := read(branchID)
	if err != nil {
		t.Fatalf("GetByID() in its own branch = %v, want no error", err)
	}
	if quote.ID != quoteID {
		t.Errorf("read %v, want %v", quote.ID, quoteID)
	}

	// Same account, wrong branch. Nothing in the database refuses this on its own.
	if _, err := read(otherBranchID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetByID() from another branch = %v, want ErrNotFound", err)
	}
}

// Every method taking the quote id a request supplied carries the branch, not just the first read:
// a caller that skipped the read would otherwise reach another branch's quote through the version
// or the status write.
func TestQuoteRepository_BranchScopesTheVersionReadAndTheStatusWrite(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Quote branch scope writes")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Sur")
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, Role: domain.UserRoleAdmin}

	var version *domain.QuoteVersion
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		version, err = repo.GetCurrentVersion(ctx, q, accountID, branchID, quoteID)
		return err
	}); err != nil {
		t.Fatalf("GetCurrentVersion() in its own branch = %v, want no error", err)
	}
	if version.ID != versionID {
		t.Errorf("read version %v, want %v", version.ID, versionID)
	}

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, readErr := repo.GetCurrentVersion(ctx, q, accountID, otherBranchID, quoteID)
		return readErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetCurrentVersion() from another branch = %v, want ErrNotFound", err)
	}

	err = db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, updateErr := repo.UpdateStatus(ctx, q, accountID, otherBranchID, quoteID,
			domain.QuoteStatusDraft, domain.QuoteStatusQuoted)
		return updateErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("UpdateStatus() from another branch = %v, want ErrConflict", err)
	}
	var status string
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT current_status FROM quote WHERE id = $1`, quoteID).Scan(&status); err != nil {
		t.Fatalf("read the quote back: %v", err)
	}
	if status != string(domain.QuoteStatusDraft) {
		t.Errorf("status = %q, want DRAFT: the wrong branch moved the quote", status)
	}
}

func ptrTime(at time.Time) *time.Time {
	return &at
}
