//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func recommendationTaxonomyIDs(t *testing.T, db *DB) (family, subgroup, otherSubgroup, otherFamily, foreignSubgroup uuid.UUID) {
	t.Helper()
	err := db.CrossAccount().QueryRow(context.Background(), `SELECT first.family_id, first.id, second.id,
		foreign_group.family_id, foreign_group.id
		FROM product_subgroup first
		JOIN product_subgroup second ON second.family_id = first.family_id AND second.id <> first.id
		JOIN product_subgroup foreign_group ON foreign_group.family_id <> first.family_id
		ORDER BY first.family_id, first.sort_order, second.sort_order, foreign_group.family_id,
		         foreign_group.sort_order LIMIT 1`).Scan(&family, &subgroup, &otherSubgroup, &otherFamily, &foreignSubgroup)
	if err != nil {
		t.Fatalf("read migrated product taxonomy: %v", err)
	}
	return
}

func seedClassifiedRecommendationProduct(t *testing.T, db *DB, accountID uuid.UUID, family, subgroup *uuid.UUID, unit *string, active bool) uuid.UUID {
	t.Helper()
	id := seedProduct(t, db, accountID, "Recommendation "+uuid.NewString())
	if _, err := db.CrossAccount().Exec(context.Background(), `UPDATE product
		SET family_id = $2, subgroup_id = $3, unit = $4, is_active = $5 WHERE id = $1`,
		id, family, subgroup, unit, active); err != nil {
		t.Fatalf("classify recommendation product: %v", err)
	}
	return id
}

func seedCatalogRecommendation(t *testing.T, db *DB, accountID, baseID, alternativeID uuid.UUID, kind domain.ProductAlternativeType) {
	t.Helper()
	// Legacy relations must remain stored even when their products stop being compatible.
	if _, err := db.CrossAccount().Exec(context.Background(), `INSERT INTO product_alternative
		(account_id, base_product_id, alternative_product_id, type) VALUES ($1, $2, $3, $4)`,
		accountID, baseID, alternativeID, kind); err != nil {
		t.Fatalf("seed catalog recommendation: %v", err)
	}
}

func TestProductAlternativeRepository_ListByBaseProductIDs_FiltersIncompatibleLegacyRecommendations(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Recommendation compatibility")
	family, subgroup, otherSubgroup, otherFamily, foreignSubgroup := recommendationTaxonomyIDs(t, db)
	unit, normalized, blank, wrongUnit := "bolsa", " BOLSA ", "  ", "m3"
	cases := []struct {
		name                        string
		baseFamily, baseSubgroup    *uuid.UUID
		alternativeFamily, subgroup *uuid.UUID
		baseUnit, alternativeUnit   *string
		active, want                bool
	}{
		{"matching products", &family, &subgroup, &family, &subgroup, &unit, &unit, true, true},
		{"normalized alternative unit", &family, &subgroup, &family, &subgroup, &unit, &normalized, true, true},
		{"normalized base unit", &family, &subgroup, &family, &subgroup, &normalized, &unit, true, true},
		{"different family", &family, &subgroup, &otherFamily, &foreignSubgroup, &unit, &unit, true, false},
		{"different subgroup", &family, &subgroup, &family, &otherSubgroup, &unit, &unit, true, false},
		{"missing alternative family", &family, &subgroup, nil, nil, &unit, &unit, true, false},
		{"missing base family", nil, nil, &family, &subgroup, &unit, &unit, true, false},
		{"missing alternative subgroup", &family, &subgroup, &family, nil, &unit, &unit, true, false},
		{"missing base subgroup", &family, nil, &family, &subgroup, &unit, &unit, true, false},
		{"both subgroups missing", &family, nil, &family, nil, &unit, &unit, true, false},
		{"missing alternative unit", &family, &subgroup, &family, &subgroup, &unit, nil, true, false},
		{"missing base unit", &family, &subgroup, &family, &subgroup, nil, &unit, true, false},
		{"both units missing", &family, &subgroup, &family, &subgroup, nil, nil, true, false},
		{"blank alternative unit", &family, &subgroup, &family, &subgroup, &unit, &blank, true, false},
		{"both units blank", &family, &subgroup, &family, &subgroup, &blank, &blank, true, false},
		{"different unit", &family, &subgroup, &family, &subgroup, &unit, &wrongUnit, true, false},
		{"inactive alternative", &family, &subgroup, &family, &subgroup, &unit, &unit, false, false},
	}
	repo := NewProductAlternativeRepository()
	for _, kind := range []domain.ProductAlternativeType{domain.ProductAlternativeEconomy, domain.ProductAlternativePremium} {
		for _, tc := range cases {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				baseID := seedClassifiedRecommendationProduct(t, db, accountID, tc.baseFamily, tc.baseSubgroup, tc.baseUnit, true)
				alternativeID := seedClassifiedRecommendationProduct(t, db, accountID, tc.alternativeFamily, tc.subgroup, tc.alternativeUnit, tc.active)
				seedCatalogRecommendation(t, db, accountID, baseID, alternativeID, kind)
				var found map[uuid.UUID][]domain.ProductAlternativeView
				err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
					var listErr error
					found, listErr = repo.ListByBaseProductIDs(ctx, q, accountID, []uuid.UUID{baseID}, kind)
					return listErr
				})
				if err != nil {
					t.Fatal(err)
				}
				offers := found[baseID]
				if tc.want {
					if len(offers) != 1 || offers[0].Product.ID != alternativeID || offers[0].Link.Type != kind {
						t.Fatalf("recommendations = %+v, want the configured compatible product", offers)
					}
				} else if len(found) != 0 {
					t.Fatalf("recommendations = %+v, want no incompatible legacy relation", found)
				}
				var stored int
				if err := db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM product_alternative
					WHERE account_id = $1 AND base_product_id = $2`, accountID, baseID).Scan(&stored); err != nil {
					t.Fatal(err)
				}
				if stored != 1 {
					t.Fatalf("stored legacy relations = %d, want 1", stored)
				}
			})
		}
	}
}

func TestProductAlternativeRepository_ListByBaseProductIDs_ScopesAccountsWithoutRLS(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Recommendation owner")
	accountB := seedAccount(t, db, "Recommendation foreign account")
	family, subgroup, _, _, _ := recommendationTaxonomyIDs(t, db)
	unit := "bolsa"
	baseA := seedClassifiedRecommendationProduct(t, db, accountA, &family, &subgroup, &unit, true)
	productA := seedClassifiedRecommendationProduct(t, db, accountA, &family, &subgroup, &unit, true)
	baseB := seedClassifiedRecommendationProduct(t, db, accountB, &family, &subgroup, &unit, true)
	productB := seedClassifiedRecommendationProduct(t, db, accountB, &family, &subgroup, &unit, true)
	corruptBase := seedClassifiedRecommendationProduct(t, db, accountA, &family, &subgroup, &unit, true)
	kind := domain.ProductAlternativeEconomy
	seedCatalogRecommendation(t, db, accountA, baseA, productA, kind)
	seedCatalogRecommendation(t, db, accountB, baseB, productB, kind)
	seedCatalogRecommendation(t, db, accountA, corruptBase, productB, kind)
	repo := NewProductAlternativeRepository()

	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	found, err := repo.ListByBaseProductIDs(ctx, tx, accountA, []uuid.UUID{baseA, baseB, corruptBase}, kind)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || len(found[baseA]) != 1 || found[baseA][0].Product.ID != productA {
		t.Fatalf("recommendations without RLS = %+v, want only the owner's valid relation", found)
	}
	products, err := NewProductRepository().GetByIDs(ctx, tx, accountA, []uuid.UUID{baseA, baseB})
	if err != nil {
		t.Fatal(err)
	}
	product, ok := products[baseA]
	if len(products) != 1 || !ok || product.FamilyID == nil || *product.FamilyID != family ||
		product.SubgroupID == nil || *product.SubgroupID != subgroup || product.Unit == nil || *product.Unit != unit {
		t.Fatalf("products without RLS = %+v, want only the owner's classified product", products)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
		var readErr error
		found, readErr = repo.ListByBaseProductIDs(ctx, q, accountA, []uuid.UUID{baseA}, kind)
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("spoofed account read = %+v, want no recommendations", found)
	}
}

func TestQuoteRepository_UnapproveAlternatives_PreservesFrozenAndForeignAlternatives(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Unapproval owner")
	accountB := seedAccount(t, db, "Unapproval foreign account")
	productA := seedProduct(t, db, accountA, "Owner recommendation")
	productB := seedProduct(t, db, accountB, "Foreign recommendation")
	_, versionA, itemA := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, frozenVersion, frozenItem := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, versionB, itemB := seedQuoteChain(t, db, accountB, branchOf(t, db, accountB), productB)
	alternativeA, frozenAlternative, alternativeB := uuid.New(), uuid.New(), uuid.New()
	for _, seed := range []struct{ account, item, product, alternative uuid.UUID }{
		{accountA, itemA, productA, alternativeA},
		{accountA, frozenItem, productA, frozenAlternative},
		{accountB, itemB, productB, alternativeB},
	} {
		if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO quote_item_alternative
			(id, account_id, quote_item_id, product_id, type, origin, rank, approved_by_seller)
			VALUES ($1, $2, $3, $4, 'PRODUCT', 'SELLER', 1, TRUE)`,
			seed.alternative, seed.account, seed.item, seed.product); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE quote_version
		SET is_immutable = TRUE, frozen_at = now() WHERE id = $1`, frozenVersion); err != nil {
		t.Fatal(err)
	}
	repo := NewQuoteRepository()
	cases := []struct {
		name     string
		version  uuid.UUID
		ids      []uuid.UUID
		conflict bool
	}{
		{"mutable owner version", versionA, []uuid.UUID{alternativeA}, false},
		{"frozen owner version", frozenVersion, []uuid.UUID{frozenAlternative}, true},
		{"foreign version and alternative", versionB, []uuid.UUID{alternativeB}, true},
		{"foreign alternative on owner version", versionA, []uuid.UUID{alternativeB}, true},
		{"different owner version", versionA, []uuid.UUID{frozenAlternative}, true},
		{"mixed owner and foreign alternatives", versionA, []uuid.UUID{alternativeA, alternativeB}, true},
	}
	for _, ownerConnection := range []bool{false, true} {
		mode := "tenant"
		if ownerConnection {
			mode = "owner without RLS"
		}
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				allIDs := []uuid.UUID{alternativeA, frozenAlternative, alternativeB}
				if _, err := db.CrossAccount().Exec(ctx, `UPDATE quote_item_alternative
					SET approved_by_seller = TRUE WHERE id = ANY($1)`, allIDs); err != nil {
					t.Fatal(err)
				}
				unapprove := func(q Querier) error {
					return repo.UnapproveAlternatives(ctx, q, accountA, tc.version, tc.ids)
				}
				var err error
				if ownerConnection {
					tx, beginErr := db.AdminTx(ctx)
					if beginErr != nil {
						t.Fatal(beginErr)
					}
					err = unapprove(tx)
					if err == nil {
						err = tx.Commit(ctx)
					} else if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
						t.Fatalf("rollback rejected unapproval: %v", rollbackErr)
					}
				} else {
					err = db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, unapprove)
				}
				if tc.conflict && !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("UnapproveAlternatives() = %v, want ErrConflict", err)
				}
				if !tc.conflict && err != nil {
					t.Fatalf("UnapproveAlternatives() = %v, want no error", err)
				}
				for _, id := range allIDs {
					var approved bool
					if err := db.CrossAccount().QueryRow(ctx, `SELECT approved_by_seller
						FROM quote_item_alternative WHERE id = $1`, id).Scan(&approved); err != nil {
						t.Fatal(err)
					}
					want := tc.conflict || id != alternativeA
					if approved != want {
						t.Errorf("alternative %v approval = %v, want %v", id, approved, want)
					}
				}
			})
		}
	}
}
