//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestQuoteRepository_DeleteRecommendationAlternative_ScopesAccountAndMutableVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Recommendation deletion owner")
	accountB := seedAccount(t, db, "Recommendation deletion other account")
	productA := seedProduct(t, db, accountA, "Owner product")
	productB := seedProduct(t, db, accountB, "Other product")
	_, versionA, itemA := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, frozenVersion, frozenItem := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, versionB, itemB := seedQuoteChain(t, db, accountB, branchOf(t, db, accountB), productB)
	mutableID, frozenID, foreignID, matcherID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	economy, premium := "ECONOMY", "PREMIUM"
	for _, seed := range []struct {
		id, account, item, product uuid.UUID
		kind                       *string
	}{
		{mutableID, accountA, itemA, productA, &economy},
		{frozenID, accountA, frozenItem, productA, &premium},
		{foreignID, accountB, itemB, productB, &economy},
		{matcherID, accountA, itemA, productA, nil},
	} {
		if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO quote_item_alternative
			(id, account_id, quote_item_id, product_id, type, origin, rank, recommendation_type)
			VALUES ($1, $2, $3, $4, 'PRODUCT', 'SELLER', 1, $5)`,
			seed.id, seed.account, seed.item, seed.product, seed.kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE quote_version
		SET is_immutable = TRUE, frozen_at = now() WHERE id = $1`, frozenVersion); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                       string
		version, item, alternative uuid.UUID
		wantDeleted                bool
	}{
		{"mutable recommendation", versionA, itemA, mutableID, true},
		{"immutable version", frozenVersion, frozenItem, frozenID, false},
		{"foreign account", versionB, itemB, foreignID, false},
		{"foreign alternative", versionA, itemA, foreignID, false},
		{"different item", versionA, frozenItem, mutableID, false},
		{"different version", frozenVersion, itemA, mutableID, false},
		{"matching candidate is not a recommendation", versionA, itemA, matcherID, false},
	}
	repo := NewQuoteRepository()
	rollback := errors.New("roll back fixture")
	for _, owner := range []bool{false, true} {
		mode := "tenant"
		if owner {
			mode = "owner without RLS"
		}
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				check := func(q Querier) error {
					err := repo.DeleteRecommendationAlternative(ctx, q, accountA, tc.version, tc.item, tc.alternative)
					if tc.wantDeleted && err != nil {
						t.Errorf("DeleteRecommendationAlternative() = %v, want success", err)
					} else if !tc.wantDeleted && !errors.Is(err, domain.ErrNotFound) {
						t.Errorf("DeleteRecommendationAlternative() = %v, want ErrNotFound", err)
					}
					if tc.wantDeleted {
						var count int
						if err := q.QueryRow(ctx, `SELECT count(*) FROM quote_item_alternative WHERE id = $1`, tc.alternative).Scan(&count); err != nil || count != 0 {
							t.Errorf("remaining recommendation = %d, error %v; want zero", count, err)
						}
					}
					return rollback
				}
				if owner {
					tx, err := db.AdminTx(ctx)
					if err != nil {
						t.Fatal(err)
					}
					check(tx)
					if err := tx.Rollback(ctx); err != nil {
						t.Fatalf("rollback: %v", err)
					}
				} else if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, check); !errors.Is(err, rollback) {
					t.Fatalf("tenant transaction = %v, want rollback sentinel", err)
				}
				var count int
				if err := db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM quote_item_alternative WHERE id = $1`, tc.alternative).Scan(&count); err != nil || count != 1 {
					t.Errorf("persisted recommendation = %d, error %v; want one", count, err)
				}
			})
		}
	}
}
