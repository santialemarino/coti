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

// These tests exercise the per-branch half of the catalog against a real database: the
// validity-period chain a price update writes, that NUMERIC(14,2) survives the round trip
// as a decimal, and that neither table is reachable from another account.

func priceCleanup(t *testing.T, db *DB, productID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.CrossAccount().Exec(ctx, `DELETE FROM product_price WHERE product_id = $1`, productID)
		_, _ = db.CrossAccount().Exec(ctx, `DELETE FROM branch_product WHERE product_id = $1`, productID)
	})
}

// The ticket's price rule, end to end: the update opens a period and closes the previous
// one at the same instant, and the history survives.
func TestProductPriceRepository_UpdateOpensAPeriodAndClosesThePrevious(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	repo := NewProductPriceRepository()
	tenant := domain.Tenant{AccountID: accountA}
	firstStart := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	secondStart := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
			Price:     decimal.RequireFromString("8500.00"),
			Currency:  domain.DefaultCurrency,
			MinPrice:  decimal.NullDecimal{Decimal: decimal.RequireFromString("7800.50"), Valid: true},
			ValidFrom: firstStart,
		})
		return err
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	// The update, as the service runs it: close then insert, one transaction.
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		closed, closeErr := repo.CloseOpenPeriod(ctx, q, accountA, branchA, productA, secondStart)
		if closeErr != nil {
			return closeErr
		}
		if closed != 1 {
			t.Errorf("periods closed = %d, want 1", closed)
		}
		_, createErr := repo.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
			Price:     decimal.RequireFromString("9100.00"),
			Currency:  domain.DefaultCurrency,
			ValidFrom: secondStart,
		})
		return createErr
	}); err != nil {
		t.Fatalf("update transaction = %v, want no error", err)
	}

	var history []domain.ProductPrice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var listErr error
		history, listErr = repo.ListByProduct(ctx, q, accountA, productA, &branchA)
		return listErr
	}); err != nil {
		t.Fatalf("ListByProduct() = %v, want no error", err)
	}

	if len(history) != 2 {
		t.Fatalf("history rows = %d, want 2 — the previous price must survive", len(history))
	}
	current, previous := history[0], history[1]

	if current.ValidTo != nil {
		t.Errorf("newest period valid_to = %v, want NULL (it is the price in force)", current.ValidTo)
	}
	if !current.Price.Equal(decimal.RequireFromString("9100.00")) {
		t.Errorf("newest price = %s, want 9100.00", current.Price)
	}
	if previous.ValidTo == nil {
		t.Fatal("previous period valid_to = NULL, want it closed")
	}
	if !previous.ValidTo.Equal(secondStart) {
		t.Errorf("previous period closed at %v, want %v — the periods must meet exactly",
			previous.ValidTo, secondStart)
	}
	if !previous.Price.Equal(decimal.RequireFromString("8500.00")) {
		t.Errorf("previous price = %s, want 8500.00 — it must not have been overwritten", previous.Price)
	}
	// min_price is the discount engine's floor, so its exact scale has to survive storage.
	if !previous.MinPrice.Valid || !previous.MinPrice.Decimal.Equal(decimal.RequireFromString("7800.50")) {
		t.Errorf("previous min_price = %v, want 7800.50", previous.MinPrice)
	}

	var open int
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return q.QueryRow(ctx,
			`SELECT count(*) FROM product_price
			 WHERE product_id = $1 AND branch_id = $2 AND valid_to IS NULL`,
			productA, branchA).Scan(&open)
	}); err != nil {
		t.Fatalf("count open periods = %v, want no error", err)
	}
	if open != 1 {
		t.Errorf("open periods = %d, want exactly 1", open)
	}
}

// Two repricings racing on the same product must still leave exactly one open period. The
// lock GetByIDForUpdate takes is what holds the second back; without it both read the same
// open period, the loser's close reaches nothing, and each opens one.
func TestProductPriceRepository_ConcurrentRepricingLeavesOneOpenPeriod(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	products := NewProductRepository()
	prices := NewProductPriceRepository()
	tenant := domain.Tenant{AccountID: accountA}
	firstStart := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	repricedAt := firstStart.Add(24 * time.Hour)

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := prices.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
			Price: decimal.RequireFromString("8500.00"), Currency: domain.DefaultCurrency,
			ValidFrom: firstStart,
		})
		return err
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	// What BranchCatalogService.SetPrice runs, with a pause wide enough that both
	// transactions would read the same open period if nothing held one of them back.
	reprice := func(price string) error {
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			if _, err := products.GetByIDForUpdate(ctx, q, accountA, productA); err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := prices.GetOpenPeriod(ctx, q, accountA, branchA, productA); err != nil {
				return err
			}
			if _, err := prices.CloseOpenPeriod(ctx, q, accountA, branchA, productA, repricedAt); err != nil {
				return err
			}
			_, err := prices.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
				Price: decimal.RequireFromString(price), Currency: domain.DefaultCurrency,
				ValidFrom: repricedAt,
			})
			return err
		})
	}

	errs := make(chan error, 2)
	go func() { errs <- reprice("9100.00") }()
	go func() { errs <- reprice("9200.00") }()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("reprice = %v, want no error", err)
		}
	}

	var open int
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return q.QueryRow(ctx,
			`SELECT count(*) FROM product_price
			 WHERE product_id = $1 AND branch_id = $2 AND valid_to IS NULL`,
			productA, branchA).Scan(&open)
	}); err != nil {
		t.Fatalf("count open periods = %v, want no error", err)
	}
	if open != 1 {
		t.Errorf("open periods after two concurrent repricings = %d, want exactly 1", open)
	}
}

func TestProductPriceRepository_GetOpenPeriod(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	repo := NewProductPriceRepository()
	tenant := domain.Tenant{AccountID: accountA}

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, getErr := repo.GetOpenPeriod(ctx, q, accountA, branchA, productA)
		return getErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetOpenPeriod() with no price = %v, want %v", err, domain.ErrNotFound)
	}

	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
			Price: decimal.RequireFromString("8500.00"), Currency: domain.DefaultCurrency,
			ValidFrom: start,
		})
		return createErr
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	var open *domain.ProductPrice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var getErr error
		open, getErr = repo.GetOpenPeriod(ctx, q, accountA, branchA, productA)
		return getErr
	}); err != nil {
		t.Fatalf("GetOpenPeriod() = %v, want no error", err)
	}
	if !open.ValidFrom.Equal(start) {
		t.Errorf("open period valid_from = %v, want %v", open.ValidFrom, start)
	}
}

// A price belongs to one account, and another one cannot read it — the criterion the ticket
// asks for, on the branch-scoped tables too.
func TestProductPriceRepository_IsInvisibleToAnotherAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	prices := NewProductPriceRepository()
	availability := NewBranchProductRepository()

	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		if _, err := prices.Create(ctx, q, accountA, branchA, productA, nil, domain.NewProductPrice{
			Price: decimal.RequireFromString("8500.00"), Currency: domain.DefaultCurrency,
			ValidFrom: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			return err
		}
		_, err := availability.Save(ctx, q, accountA, branchA, productA, domain.BranchAvailability{
			Stock:    decimal.NullDecimal{Decimal: decimal.RequireFromString("250"), Valid: true},
			IsActive: true,
		})
		return err
	}); err != nil {
		t.Fatalf("seed price and availability = %v, want no error", err)
	}

	scopeB := domain.Tenant{AccountID: accountB}

	var foreignPrices []domain.ProductPrice
	var foreignAvailability []domain.BranchProduct
	if err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
		var err error
		// Account A's own id is passed on purpose: even with the predicate spoofed, the
		// policy is what denies the read.
		foreignPrices, err = prices.ListByProduct(ctx, q, accountA, productA, &branchA)
		if err != nil {
			return err
		}
		foreignAvailability, err = availability.ListByProduct(ctx, q, accountA, productA, &branchA)
		return err
	}); err != nil {
		t.Fatalf("InTenantTx() = %v, want no error", err)
	}

	if len(foreignPrices) != 0 {
		t.Errorf("prices visible to another account = %d, want 0", len(foreignPrices))
	}
	if len(foreignAvailability) != 0 {
		t.Errorf("availability visible to another account = %d, want 0", len(foreignAvailability))
	}

	// And the closing update reaches nothing, so B cannot end A's price period either.
	if err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
		closed, closeErr := prices.CloseOpenPeriod(ctx, q, accountA, branchA, productA, time.Now())
		if closeErr != nil {
			return closeErr
		}
		if closed != 0 {
			t.Errorf("periods closed from another account = %d, want 0", closed)
		}
		return nil
	}); err != nil {
		t.Fatalf("InTenantTx() = %v, want no error", err)
	}
}

func TestBranchProductRepository_SaveUpsertsOneRowPerBranchAndProduct(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	repo := NewBranchProductRepository()
	tenant := domain.Tenant{AccountID: accountA}

	var first, second *domain.BranchProduct
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		first, err = repo.Save(ctx, q, accountA, branchA, productA, domain.BranchAvailability{
			Stock:    decimal.NullDecimal{Decimal: decimal.RequireFromString("250.00"), Valid: true},
			IsActive: true,
		})
		return err
	}); err != nil {
		t.Fatalf("Save() = %v, want no error", err)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		second, err = repo.Save(ctx, q, accountA, branchA, productA, domain.BranchAvailability{
			IsActive: false,
		})
		return err
	}); err != nil {
		t.Fatalf("Save() again = %v, want no error", err)
	}

	if first.ID != second.ID {
		t.Errorf("second Save() created row %v, want it to update %v", second.ID, first.ID)
	}
	if second.IsActive {
		t.Error("is_active = true after deactivating the branch's availability")
	}
	// An absent stock has to land as NULL, not as zero: "not tracked" is not "none left".
	if second.Stock.Valid {
		t.Errorf("stock = %v, want NULL", second.Stock.Decimal)
	}

	var rows int
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return q.QueryRow(ctx,
			`SELECT count(*) FROM branch_product WHERE branch_id = $1 AND product_id = $2`,
			branchA, productA).Scan(&rows)
	}); err != nil {
		t.Fatalf("count availability rows = %v, want no error", err)
	}
	if rows != 1 {
		t.Errorf("availability rows = %d, want exactly 1", rows)
	}
}

// A nil branch reads every branch of the account, which is the account-wide view an admin
// gets when the request carries no X-Branch-Id.
func TestBranchProductRepository_ListByProductSpansBranchesWhenUnfiltered(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	branchA := branchOf(t, db, accountA)
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	priceCleanup(t, db, productA)

	secondBranch := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Corralon A Moron')`,
		secondBranch, accountA); err != nil {
		t.Fatalf("seed second branch: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.CrossAccount().Exec(context.Background(), `DELETE FROM branch WHERE id = $1`, secondBranch)
	})

	repo := NewBranchProductRepository()
	tenant := domain.Tenant{AccountID: accountA}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		for _, branchID := range []uuid.UUID{branchA, secondBranch} {
			if _, err := repo.Save(ctx, q, accountA, branchID, productA,
				domain.BranchAvailability{IsActive: true}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("Save() = %v, want no error", err)
	}

	cases := []struct {
		name     string
		branchID *uuid.UUID
		want     int
	}{
		{"account-wide", nil, 2},
		{"one branch", &branchA, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []domain.BranchProduct
			if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				var listErr error
				rows, listErr = repo.ListByProduct(ctx, q, accountA, productA, tc.branchID)
				return listErr
			}); err != nil {
				t.Fatalf("ListByProduct() = %v, want no error", err)
			}
			if len(rows) != tc.want {
				t.Errorf("rows = %d, want %d", len(rows), tc.want)
			}
		})
	}
}
