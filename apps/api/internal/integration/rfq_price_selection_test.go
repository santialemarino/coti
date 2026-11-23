//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func (e *env) classifyPriceSelectionProduct(t *testing.T, productID uuid.UUID) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `
		UPDATE product SET unit = 'bolsa', family_id = s.family_id, subgroup_id = s.id
		FROM product_subgroup s JOIN product_family f ON f.id = s.family_id
		WHERE product.id = $1 AND f.name = 'MATERIALES DE CONSTRUCCION' AND s.name = 'BOLSAS'`, productID); err != nil {
		t.Fatal(err)
	}
}

func TestRFQPriceSelection_ExpandsCatalogAndReevaluatesCurrentBranchPrices(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "RFQ price selection")
	base := e.seedPricedProduct(t, account, branch, "Cemento Loma Negra 50 kg", "9500", nil)
	cheap := e.seedPricedProduct(t, account, branch, "Cemento Avellaneda 50 kg", "8900", nil)
	for _, id := range []uuid.UUID{base, cheap} {
		e.classifyPriceSelectionProduct(t, id)
	}
	otherAccount, otherBranch := e.seedAccount(t, "Other supplier")
	foreign := e.seedPricedProduct(t, otherAccount, otherBranch, "Cemento otra cuenta 50 kg", "1", nil)
	e.classifyPriceSelectionProduct(t, foreign)
	ctx := context.Background()
	products := repository.NewProductRepository()
	selector := services.NewRFQPriceSelectionService(e.db, products, repository.NewProductPriceRepository(), matchConfig())
	tenant := domain.Tenant{AccountID: account, BranchID: branch}
	line := domain.ExtractedRFQLine{RequestedDescription: "cemento más barato", SearchDescription: "cemento",
		SelectionPreference: domain.RFQSelectionLowestPrice}
	selectProduct := func() domain.LineMatch {
		t.Helper()
		matches := []domain.LineMatch{{ProductID: &base, MatchStatus: domain.ItemMatchStatusAmbiguous,
			Confidence: decimal.RequireFromString("0.8")}}
		if err := selector.Select(ctx, tenant, []domain.ExtractedRFQLine{line}, matches); err != nil {
			t.Fatal(err)
		}
		return matches[0]
	}
	if match := selectProduct(); match.ProductID == nil || *match.ProductID != cheap || match.MatchStatus != domain.ItemMatchStatusMatched {
		t.Fatalf("first match = %+v, want the cheaper product outside the initial shortlist", match)
	}
	line.SelectionPreference = domain.RFQSelectionHighestPrice
	if match := selectProduct(); match.ProductID == nil || *match.ProductID != base || match.MatchStatus != domain.ItemMatchStatusMatched {
		t.Fatalf("highest-price match = %+v, want the more expensive current product", match)
	}
	line.SelectionPreference = domain.RFQSelectionLowestPrice
	e.openPricePeriod(t, account, branch, cheap, "10000", nil)
	if match := selectProduct(); match.ProductID == nil || *match.ProductID != base || match.MatchStatus != domain.ItemMatchStatusMatched {
		t.Fatalf("match after price update = %+v, want the newly cheaper original product", match)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `
		UPDATE product_price SET valid_from = now() + interval '1 day'
		WHERE account_id = $1 AND branch_id = $2 AND product_id = $3`, account, branch, cheap); err != nil {
		t.Fatal(err)
	}
	if match := selectProduct(); match.MatchStatus != domain.ItemMatchStatusAmbiguous {
		t.Fatalf("match with a product only priced in the future = %+v, want review", match)
	}
	t.Run("explicit account filter with RLS bypassed", func(t *testing.T) {
		groups, err := products.ListPriceSelectionCandidates(ctx, e.db.CrossAccount(), account, branch, []uuid.UUID{foreign})
		if err != nil || len(groups) != 0 {
			t.Fatalf("foreign anchor = %v, %v; want no groups", groups, err)
		}
		groups, err = products.ListPriceSelectionCandidates(ctx, e.db.CrossAccount(), account, otherBranch, []uuid.UUID{base})
		if err != nil || len(groups) != 0 {
			t.Fatalf("foreign branch = %v, %v; want no groups", groups, err)
		}
	})
	t.Run("RLS refuses a claimed foreign account", func(t *testing.T) {
		if err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			groups, err := products.ListPriceSelectionCandidates(ctx, q, otherAccount, otherBranch, []uuid.UUID{foreign})
			if err != nil {
				return err
			}
			if len(groups) != 0 {
				t.Fatalf("RLS exposed foreign groups: %v", groups)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("inactive branch products are excluded", func(t *testing.T) {
		if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE branch_product SET is_active = FALSE
			WHERE account_id = $1 AND branch_id = $2 AND product_id = $3`, account, branch, cheap); err != nil {
			t.Fatal(err)
		}
		if match := selectProduct(); match.ProductID == nil || *match.ProductID != base || match.MatchStatus != domain.ItemMatchStatusMatched {
			t.Fatalf("match = %+v, want the only active product", match)
		}
	})
}
