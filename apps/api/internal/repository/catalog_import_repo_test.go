//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCatalogImportRepository_ApplyImport_CreatesTheAccountAndBranchRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Catalog import")
	branchID := branchOf(t, db, accountID)
	userID := seedUser(t, db, accountID, "ADMIN")
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: userID}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM product_price WHERE account_id = $1`, accountID)
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM branch_product WHERE account_id = $1`, accountID)
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM product WHERE account_id = $1`, accountID)
	})

	minPrice := "9500.00"
	var familyID, subgroupID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT f.id, s.id
		 FROM product_family f
		 JOIN product_subgroup s ON s.family_id = f.id
		 WHERE f.name = 'MATERIALES DE CONSTRUCCION' AND s.name = 'ARIDOS'`,
	).Scan(&familyID, &subgroupID); err != nil {
		t.Fatal(err)
	}
	rows := []domain.CatalogImportRow{
		{
			Code: "CEM-001", Name: "Cemento Portland", Description: "Cemento Portland 50 kg",
			Unit: "bolsa", FamilyID: familyID, Family: "MATERIALES DE CONSTRUCCION",
			SubgroupID: &subgroupID, Price: "10000.00", MinPrice: &minPrice, IsActive: true,
		},
		{
			Code: "ARE-001", Name: "Arena fina", Description: "Arena fina a granel",
			Unit: "m3", FamilyID: familyID, Family: "MATERIALES DE CONSTRUCCION",
			Price: "5000.00", IsActive: true,
		},
	}

	repo := NewCatalogImportRepository()
	effectiveAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, effectiveAt, rows)
	}); err != nil {
		t.Fatalf("ApplyImport() = %v, want no error", err)
	}

	var products, availability, prices int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT
		   (SELECT count(*) FROM product WHERE account_id = $1),
		   (SELECT count(*) FROM branch_product WHERE account_id = $1 AND branch_id = $2 AND is_active),
		   (SELECT count(*) FROM product_price WHERE account_id = $1 AND branch_id = $2 AND valid_from = $3)`,
		accountID, branchID, effectiveAt).Scan(&products, &availability, &prices); err != nil {
		t.Fatal(err)
	}
	if products != 2 || availability != 2 || prices != 2 {
		t.Fatalf("products, availability, prices = %d, %d, %d; want 2, 2, 2", products, availability, prices)
	}

	var name, description, price string
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT p.canonical_name, p.description, pp.price::text
		 FROM product p
		 JOIN product_price pp ON pp.account_id = p.account_id AND pp.product_id = p.id
		 WHERE p.account_id = $1 AND p.code = 'CEM-001' AND pp.branch_id = $2`,
		accountID, branchID).Scan(&name, &description, &price); err != nil {
		t.Fatal(err)
	}
	if name != "Cemento Portland" || description != "Cemento Portland 50 kg" || price != "10000.00" {
		t.Errorf("stored row = %q, %q, %q; want normalized catalog values", name, description, price)
	}
}

func TestCatalogImportRepository_ApplyImport_UpdatesProductsAndCreatesPriceHistory(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Catalog update")
	branchID := branchOf(t, db, accountID)
	userID := seedUser(t, db, accountID, "ADMIN")
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: userID}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM product_price WHERE account_id = $1`, accountID)
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM branch_product WHERE account_id = $1`, accountID)
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM product WHERE account_id = $1`, accountID)
	})

	var familyID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM product_family WHERE name = 'MATERIALES DE CONSTRUCCION'`,
	).Scan(&familyID); err != nil {
		t.Fatal(err)
	}
	repo := NewCatalogImportRepository()
	firstAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	first := []domain.CatalogImportRow{{
		Code: "CEM-001", Name: "Cemento", Unit: "bolsa", FamilyID: familyID,
		Price: "10000.00", IsActive: true,
	}}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, firstAt, first)
	}); err != nil {
		t.Fatalf("first ApplyImport() = %v", err)
	}

	secondAt := firstAt.Add(time.Hour)
	second := []domain.CatalogImportRow{{
		Code: "CEM-001", Name: "Cemento Portland", Unit: "bolsa", FamilyID: familyID,
		Price: "11000.00", IsActive: true,
	}}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, secondAt, second)
	}); err != nil {
		t.Fatalf("second ApplyImport() = %v", err)
	}

	var products, prices int
	var name, currentPrice string
	var previousValidTo time.Time
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT
		   (SELECT count(*) FROM product WHERE account_id = $1 AND code = 'CEM-001'),
		   (SELECT count(*) FROM product_price WHERE account_id = $1 AND branch_id = $2),
		   p.canonical_name,
		   current_price.price::text,
		   previous_price.valid_to
		 FROM product p
		 JOIN product_price current_price ON current_price.account_id = p.account_id
		   AND current_price.product_id = p.id AND current_price.branch_id = $2
		   AND current_price.valid_from = $3
		 JOIN product_price previous_price ON previous_price.account_id = p.account_id
		   AND previous_price.product_id = p.id AND previous_price.branch_id = $2
		   AND previous_price.valid_from = $4
		 WHERE p.account_id = $1 AND p.code = 'CEM-001'`,
		accountID, branchID, secondAt, firstAt,
	).Scan(&products, &prices, &name, &currentPrice, &previousValidTo); err != nil {
		t.Fatal(err)
	}
	if products != 1 || prices != 2 {
		t.Errorf("products, prices = %d, %d; want 1, 2", products, prices)
	}
	if name != "Cemento Portland" || currentPrice != "11000.00" || !previousValidTo.Equal(secondAt) {
		t.Errorf("updated row = %q, %q, %v; want current product and closed previous price", name, currentPrice, previousValidTo)
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, secondAt.Add(time.Hour), second)
	}); err != nil {
		t.Fatalf("unchanged ApplyImport() = %v", err)
	}
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT count(*) FROM product_price WHERE account_id = $1 AND branch_id = $2`,
		accountID, branchID).Scan(&prices); err != nil {
		t.Fatal(err)
	}
	if prices != 2 {
		t.Errorf("prices after an unchanged import = %d, want 2", prices)
	}
}
