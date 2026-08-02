//go:build integration

package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestProductPriceRepository_ApplyImport_VersionsPricesAtomically(t *testing.T) {
	appURL := os.Getenv("TEST_DATABASE_URL")
	adminURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("integration database URLs are not configured")
	}
	ctx := context.Background()
	appPool, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer appPool.Close()
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminPool.Close()

	accountID, branchID, userID, productID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	setupStatements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO account (id, name) VALUES ($1, 'Price import test')`, []any{accountID}},
		{`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Test branch')`, []any{branchID, accountID}},
		{`INSERT INTO app_user (id, account_id, name, email, password_hash, role)
		  VALUES ($1, $2, 'Test admin', $3, 'unused', 'ADMIN')`, []any{userID, accountID, userID.String() + "@test.invalid"}},
		{`INSERT INTO product (id, account_id, code, canonical_name)
		  VALUES ($1, $2, 'TEST-PRICE', 'Test product')`, []any{productID, accountID}},
		{`INSERT INTO branch_product (account_id, branch_id, product_id)
		  VALUES ($1, $2, $3)`, []any{accountID, branchID, productID}},
	}
	for _, statement := range setupStatements {
		if _, err := adminPool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupQueries := []string{
			`DELETE FROM product_price WHERE account_id = $1`,
			`DELETE FROM branch_product WHERE account_id = $1`,
			`DELETE FROM product WHERE account_id = $1`,
			`DELETE FROM app_user WHERE account_id = $1`,
			`DELETE FROM branch WHERE account_id = $1`,
			`DELETE FROM account WHERE id = $1`,
		}
		for _, query := range cleanupQueries {
			_, _ = adminPool.Exec(context.Background(), query, accountID)
		}
	})

	db := &DB{app: appPool, admin: adminPool}
	repo := NewProductPriceRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: userID, Role: domain.UserRoleAdmin}
	firstAt := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	apply := func(at time.Time, price string) error {
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.ApplyImport(ctx, q, tenant, at, []domain.ProductPriceUpdate{{
				ProductID: productID, Price: price, Currency: "ARS",
			}})
		})
	}
	if err := apply(firstAt, "100.00"); err != nil {
		t.Fatalf("first ApplyImport() = %v", err)
	}
	if err := apply(secondAt, "120.00"); err != nil {
		t.Fatalf("second ApplyImport() = %v", err)
	}

	rows, err := adminPool.Query(ctx,
		`SELECT price::text, valid_from, valid_to
		 FROM product_price
		 WHERE account_id = $1 AND branch_id = $2 AND product_id = $3
		 ORDER BY valid_from`, accountID, branchID, productID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type version struct {
		price     string
		validFrom time.Time
		validTo   *time.Time
	}
	var versions []version
	for rows.Next() {
		var current version
		if err := rows.Scan(&current.price, &current.validFrom, &current.validTo); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, current)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	if versions[0].price != "100.00" || versions[0].validTo == nil || !versions[0].validTo.Equal(secondAt) {
		t.Errorf("first version = %#v, want closed at second import", versions[0])
	}
	if versions[1].price != "120.00" || versions[1].validTo != nil {
		t.Errorf("second version = %#v, want current price", versions[1])
	}

	var exported *domain.ProductPriceExport
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var exportErr error
		exported, exportErr = repo.ListCurrentForExport(ctx, q, accountID, branchID)
		return exportErr
	}); err != nil {
		t.Fatalf("ListCurrentForExport() = %v", err)
	}
	if exported.BranchName != "Test branch" || len(exported.Rows) != 1 {
		t.Fatalf("exported = %#v, want one row for Test branch", exported)
	}
	if exported.Rows[0].Code != "TEST-PRICE" || exported.Rows[0].Price != "120.00" {
		t.Errorf("exported row = %#v, want the current price only", exported.Rows[0])
	}
}
