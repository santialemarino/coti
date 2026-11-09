//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestQuoteRepository_PromoteMatcherRecommendation_ScopesAndPreservesDecision(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Promotion owner")
	accountB := seedAccount(t, db, "Promotion other account")
	productA := seedProduct(t, db, accountA, "Owner candidate")
	productB := seedProduct(t, db, accountB, "Foreign candidate")
	_, versionA, itemA := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, frozenVersion, frozenItem := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, occupiedVersion, occupiedItem := seedQuoteChain(t, db, accountA, branchOf(t, db, accountA), productA)
	_, versionB, itemB := seedQuoteChain(t, db, accountB, branchOf(t, db, accountB), productB)
	valid, frozen, foreign, approved, chosen, classified, seller, occupied, slot :=
		uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	type seedRow struct {
		id, account, item, product uuid.UUID
		origin                     string
		kind                       *string
		approved, chosen           bool
	}
	economy, premium := "ECONOMY", "PREMIUM"
	for _, row := range []seedRow{
		{valid, accountA, itemA, productA, "AI", nil, false, false},
		{frozen, accountA, frozenItem, productA, "AI", nil, false, false},
		{foreign, accountB, itemB, productB, "AI", nil, false, false},
		{approved, accountA, itemA, productA, "AI", nil, true, false},
		{chosen, accountA, itemA, productA, "AI", nil, false, true},
		{classified, accountA, itemA, productA, "AI", &premium, false, false},
		{seller, accountA, itemA, productA, "SELLER", nil, false, false},
		{occupied, accountA, occupiedItem, productA, "AI", nil, false, false},
		{slot, accountA, occupiedItem, productA, "SELLER", &economy, false, false},
	} {
		if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO quote_item_alternative
			(id, account_id, quote_item_id, product_id, type, origin, rank, confidence_score,
			 recommendation_type, approved_by_seller, chosen_by_client)
			VALUES ($1, $2, $3, $4, 'PRODUCT', $5, 7, 0.2500, $6, $7, $8)`,
			row.id, row.account, row.item, row.product, row.origin, row.kind, row.approved, row.chosen); err != nil {
			t.Fatalf("seed candidate: %v", err)
		}
	}
	if _, err := db.CrossAccount().Exec(ctx, `UPDATE quote_version
		SET is_immutable = TRUE, frozen_at = now() WHERE id = $1`, frozenVersion); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name                                string
		version, item, alternative, product uuid.UUID
		wantSuccess                         bool
	}{
		{"owner candidate", versionA, itemA, valid, productA, true},
		{"frozen version", frozenVersion, frozenItem, frozen, productA, false},
		{"foreign account", versionB, itemB, foreign, productB, false},
		{"foreign candidate", versionA, itemA, foreign, productB, false},
		{"different item", versionA, frozenItem, valid, productA, false},
		{"different version", frozenVersion, itemA, valid, productA, false},
		{"different product", versionA, itemA, valid, productB, false},
		{"seller approved", versionA, itemA, approved, productA, false},
		{"client selected", versionA, itemA, chosen, productA, false},
		{"already classified", versionA, itemA, classified, productA, false},
		{"seller candidate", versionA, itemA, seller, productA, false},
		{"occupied slot", occupiedVersion, occupiedItem, occupied, productA, false},
	}
	repo := NewQuoteRepository()
	price := decimal.NewNullDecimal(decimal.RequireFromString("80.00"))
	floor := decimal.NewNullDecimal(decimal.RequireFromString("50.00"))
	rollback := errors.New("roll back test promotion")
	for _, owner := range []bool{false, true} {
		mode := "tenant"
		if owner {
			mode = "owner without RLS"
		}
		for _, tc := range cases {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				check := func(q Querier) error {
					got, err := repo.PromoteMatcherRecommendation(ctx, q, accountA, tc.version, tc.item,
						tc.alternative, tc.product, domain.ProductAlternativeEconomy, price, floor)
					if !tc.wantSuccess {
						if !errors.Is(err, domain.ErrNotFound) {
							t.Errorf("PromoteMatcherRecommendation() = %v, want ErrNotFound", err)
						}
						return rollback
					}
					if err != nil {
						t.Errorf("PromoteMatcherRecommendation() = %v, want success", err)
						return rollback
					}
					if got.ID != valid || got.ProductID == nil || *got.ProductID != productA ||
						got.RecommendationType == nil || *got.RecommendationType != domain.ProductAlternativeEconomy ||
						got.Origin != domain.QuoteItemAlternativeOriginAI || got.Rank != 7 ||
						!got.ConfidenceScore.Valid || !got.ConfidenceScore.Decimal.Equal(decimal.RequireFromString("0.25")) ||
						!got.PriceSnapshot.Valid || !got.PriceSnapshot.Decimal.Equal(price.Decimal) ||
						!got.MinPriceSnapshot.Valid || !got.MinPriceSnapshot.Decimal.Equal(floor.Decimal) ||
						got.ApprovedBySeller || got.ChosenByClient {
						t.Errorf("promoted candidate = %+v, want same identity and decisions with priced recommendation", got)
					}
					_, err = repo.PromoteMatcherRecommendation(ctx, q, accountA, tc.version, tc.item,
						tc.alternative, tc.product, domain.ProductAlternativeEconomy, price, floor)
					if !errors.Is(err, domain.ErrNotFound) {
						t.Errorf("repeated promotion = %v, want ErrNotFound", err)
					}
					var count int
					if err := q.QueryRow(ctx, `SELECT count(*) FROM quote_item_alternative WHERE id = $1`, valid).Scan(&count); err != nil || count != 1 {
						t.Errorf("promoted candidate count = %d, error %v; want one", count, err)
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
			})
		}
	}
	t.Run("RLS rejects a spoofed owner account", func(t *testing.T) {
		err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
			_, err := repo.PromoteMatcherRecommendation(ctx, q, accountA, versionA, itemA,
				valid, productA, domain.ProductAlternativeEconomy, price, floor)
			return err
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("spoofed account promotion = %v, want ErrNotFound", err)
		}
	})
}
