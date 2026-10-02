//go:build integration

package repository

import (
	"context"
	"slices"
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

// A price scheduled ahead stays the next period: the imported price runs until it starts rather
// than opening a second period with no end, which the open-period index refuses.
func TestCatalogImportRepository_ApplyImport_EndsTheNewPriceWhereAScheduledOneStarts(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Catalog scheduled price")
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
	row := domain.CatalogImportRow{Code: "CEM-001", Name: "Cemento", Unit: "bolsa",
		FamilyID: familyID, Price: "10000.00", IsActive: true}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, firstAt, []domain.CatalogImportRow{row})
	}); err != nil {
		t.Fatalf("first ApplyImport() = %v", err)
	}

	// What POST /v1/products/{id}/prices writes for a price that starts next week.
	scheduledAt := firstAt.Add(7 * 24 * time.Hour)
	if _, err := db.CrossAccount().Exec(ctx,
		`WITH closed AS (
		   UPDATE product_price SET valid_to = $3
		   WHERE account_id = $1 AND branch_id = $2 AND valid_to IS NULL
		   RETURNING product_id, user_id
		 )
		 INSERT INTO product_price
		   (account_id, branch_id, product_id, user_id, price, currency, valid_from)
		 SELECT $1, $2, product_id, user_id, 12000, 'ARS', $3 FROM closed`,
		accountID, branchID, scheduledAt); err != nil {
		t.Fatalf("schedule a price: %v", err)
	}

	importedAt := firstAt.Add(24 * time.Hour)
	row.Price = "11000.00"
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, importedAt, []domain.CatalogImportRow{row})
	}); err != nil {
		t.Fatalf("ApplyImport() with a scheduled price = %v, want the import applied", err)
	}

	rows, err := db.CrossAccount().Query(ctx,
		`SELECT price::text, valid_from, valid_to FROM product_price
		 WHERE account_id = $1 AND branch_id = $2 ORDER BY valid_from`, accountID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type period struct {
		price string
		from  time.Time
		to    *time.Time
	}
	var periods []period
	for rows.Next() {
		var p period
		if err := rows.Scan(&p.price, &p.from, &p.to); err != nil {
			t.Fatal(err)
		}
		periods = append(periods, p)
	}
	if len(periods) != 3 {
		t.Fatalf("periods = %+v, want the original, the imported and the scheduled one", periods)
	}
	if periods[0].to == nil || !periods[0].to.Equal(importedAt) {
		t.Errorf("original period ends at %v, want the import instant %v", periods[0].to, importedAt)
	}
	if periods[1].price != "11000.00" || periods[1].to == nil || !periods[1].to.Equal(scheduledAt) {
		t.Errorf("imported period = %+v, want 11000.00 until the scheduled price starts", periods[1])
	}
	if periods[2].price != "12000.00" || periods[2].to != nil {
		t.Errorf("scheduled period = %+v, want 12000.00 still open", periods[2])
	}
}

// Two imports of one branch racing: the product upsert makes the later one wait for the first to
// commit, so the history comes out as if they had run one after the other.
func TestCatalogImportRepository_ApplyImport_SerializesARacingImport(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Catalog racing import")
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
	startAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	row := func(price string) []domain.CatalogImportRow {
		return []domain.CatalogImportRow{{Code: "CEM-001", Name: "Cemento", Unit: "bolsa",
			FamilyID: familyID, Price: price, IsActive: true}}
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.ApplyImport(ctx, q, tenant, startAt, row("10000.00"))
	}); err != nil {
		t.Fatalf("first ApplyImport() = %v", err)
	}

	applied, release := make(chan struct{}), make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.InTenantTx(ctx, tenant, func(q Querier) error {
			if err := repo.ApplyImport(ctx, q, tenant, startAt.Add(time.Hour), row("11000.00")); err != nil {
				return err
			}
			close(applied)
			<-release
			return nil
		})
	}()
	select {
	case <-applied:
	case err := <-firstDone:
		t.Fatalf("racing ApplyImport() = %v before it could hold its transaction", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.ApplyImport(ctx, q, tenant, startAt.Add(2*time.Hour), row("11500.00"))
		})
	}()
	// Long enough for the second import to reach the rows the first still holds, and it must still
	// be waiting on them: an import that finished here raced the first rather than following it.
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-secondDone:
		t.Fatalf("second ApplyImport() = %v while the first held its transaction, want it waiting", err)
	default:
	}
	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first racing ApplyImport() = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second racing ApplyImport() = %v, want it applied after the first", err)
	}

	rows, err := db.CrossAccount().Query(ctx,
		`SELECT price::text, valid_to IS NULL FROM product_price
		 WHERE account_id = $1 AND branch_id = $2 ORDER BY valid_from`, accountID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var history []string
	for rows.Next() {
		var price string
		var open bool
		if err := rows.Scan(&price, &open); err != nil {
			t.Fatal(err)
		}
		if open {
			price += " open"
		}
		history = append(history, price)
	}
	if want := []string{"10000.00", "11000.00", "11500.00 open"}; !slices.Equal(history, want) {
		t.Errorf("price history = %v, want %v", history, want)
	}
}
