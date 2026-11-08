//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestRecommendationRequest_ConcurrentAddAndTenantIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "On-demand recommendation")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	base := e.seedPricedProduct(t, accountID, branchID, "Base", "100.00", nil)
	alternative := e.seedPricedProduct(t, accountID, branchID, "Alternative", "80.00", nil)
	var familyID, subgroupID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT family_id, id FROM product_subgroup ORDER BY family_id, sort_order LIMIT 1`).Scan(&familyID, &subgroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE product SET family_id=$2, subgroup_id=$3, unit='bolsa'
		WHERE account_id=$1 AND id = ANY($4::uuid[])`, accountID, familyID, subgroupID, []uuid.UUID{base, alternative}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO product_alternative
		(account_id, base_product_id, alternative_product_id, type) VALUES ($1,$2,$3,'ECONOMY')`, accountID, base, alternative); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_alternative WHERE account_id=$1 AND base_product_id=$2`, accountID, base)
	})
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{{description: "Base", quantity: "1", productID: &base}})
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET current_status='QUOTED' WHERE id=$1`, seeded.quoteID); err != nil {
		t.Fatal(err)
	}
	var itemID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `UPDATE quote_item SET unit_price_snapshot=100, subtotal=100
		WHERE version_id=$1 RETURNING id`, seeded.versionID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	service := services.NewRecommendationRequestService(e.db, repository.NewQuoteRepository(),
		repository.NewProductAlternativeRepository(), repository.NewProductPriceRepository())
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID}
	proposals, err := service.Search(ctx, tenant, seeded.quoteID)
	if err != nil || len(proposals) != 1 || proposals[0].ProductID != alternative {
		t.Fatalf("Search() = %+v, %v", proposals, err)
	}
	otherBranch := e.seedBranch(t, accountID, "Other branch")
	_, err = service.Search(ctx, domain.Tenant{AccountID: accountID, BranchID: otherBranch}, seeded.quoteID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("different branch: %v, want not found", err)
	}
	otherAccount, foreignBranch := e.seedAccount(t, "Other account")
	_, err = service.Search(ctx, domain.Tenant{AccountID: otherAccount, BranchID: foreignBranch}, seeded.quoteID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("different account: %v, want not found", err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, addErr := service.AddIfVacant(ctx, tenant, seeded.quoteID, itemID, alternative, domain.ProductAlternativeEconomy)
			results <- addErr
		}()
	}
	wg.Wait()
	close(results)
	var successes, conflicts int
	for addErr := range results {
		switch {
		case addErr == nil:
			successes++
		case errors.Is(addErr, domain.ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent add: %v", addErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want one each", successes, conflicts)
	}
	var count, approved int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE approved_by_seller)
		FROM quote_item_alternative WHERE account_id=$1 AND quote_item_id=$2`, accountID, itemID).Scan(&count, &approved); err != nil {
		t.Fatal(err)
	}
	if count != 1 || approved != 0 {
		t.Fatalf("stored recommendations = %d, approved = %d; want one unapproved", count, approved)
	}

	second := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{{description: "Base", quantity: "1", productID: &base}})
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET current_status='QUOTED' WHERE id=$1`, second.quoteID); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `UPDATE quote_item SET unit_price_snapshot=100, subtotal=100
		WHERE version_id=$1 RETURNING id`, second.versionID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	matcherID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO quote_item_alternative
		(id,account_id,quote_item_id,product_id,type,origin,rank,confidence_score)
		VALUES ($1,$2,$3,$4,'PRODUCT','AI',4,0.8)`, matcherID, accountID, itemID, alternative); err != nil {
		t.Fatal(err)
	}
	added, err := service.AddIfVacant(ctx, tenant, second.quoteID, itemID, alternative, domain.ProductAlternativeEconomy)
	if err != nil || added == nil || added.ID != matcherID || added.Rank != 4 ||
		added.Origin != domain.QuoteItemAlternativeOriginAI || added.ApprovedBySeller {
		t.Fatalf("matcher promotion = %+v, %v", added, err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM quote_item_alternative
		WHERE account_id=$1 AND quote_item_id=$2`, accountID, itemID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("matcher promotion left %d rows, want 1", count)
	}
}
