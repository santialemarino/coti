package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestQuoteService_AutoQuoteChecksEveryLineAndCurrency(t *testing.T) {
	for _, scenario := range []string{"complete", "mixed-currencies", "second-unpriced"} {
		t.Run(scenario, func(t *testing.T) {
			first, second, unit := uuid.New(), uuid.New(), "bolsa"
			items := []domain.QuoteItem{{ID: uuid.New(), ProductID: &first, Quantity: decimal.NewFromInt(2), Unit: &unit, MatchStatus: domain.ItemMatchStatusMatched}, {ID: uuid.New(), ProductID: &second, Quantity: decimal.NewFromInt(3), Unit: &unit, MatchStatus: domain.ItemMatchStatusMatched}}
			prices := map[uuid.UUID]domain.BranchPrice{first: {ProductID: first, Price: decimal.RequireFromString("1200.50"), Currency: "ARS"}, second: {ProductID: second, Price: decimal.NewFromInt(10), Currency: "ARS"}}
			if scenario == "mixed-currencies" {
				p := prices[second]
				p.Currency = "USD"
				prices[second] = p
			}
			if scenario == "second-unpriced" {
				delete(prices, second)
			}
			f := newQuoteFixture(items, prices)
			products := map[uuid.UUID]domain.Product{}
			for _, id := range []uuid.UUID{first, second} {
				products[id] = domain.Product{ID: id, AccountID: f.tenant.AccountID, Unit: &unit, IsActive: true}
			}
			f.service.WithAutomaticQuoteInputs(fakeAutomaticQuoteInputs{}).WithRecommendationProducts(fakeRecommendationProducts{products: products})
			var result *domain.PricedQuote
			err := f.db.InTenantTx(context.Background(), f.tenant, func(q repository.Querier) error {
				var err error
				result, err = f.service.TryAutoQuote(context.Background(), q, f.tenant, f.quoteID)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "complete" {
				if result != nil || len(f.quotes.appliedPricings) > 0 {
					t.Fatal("a later ineligible line produced a partial valuation")
				}
				return
			}
			if result == nil || !result.Version.Total.Equal(decimal.RequireFromString("2431.00")) {
				t.Fatalf("two-line valuation=%+v", result)
			}
		})
	}
}
