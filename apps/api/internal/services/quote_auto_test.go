package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestQuoteService_AutoQuote(t *testing.T) {
	for _, name := range []string{"complete", "ambiguous", "unmatched", "missing-price", "zero-price", "inactive", "foreign-product", "incompatible-unit", "unresolved-quantity", "empty", "later-version", "already-quoted", "sent", "archived"} {
		t.Run(name, func(t *testing.T) {
			id, unit := uuid.New(), "bolsa"
			item := domain.QuoteItem{ID: uuid.New(), ProductID: &id, Quantity: decimal.NewFromInt(2), Unit: &unit, MatchStatus: domain.ItemMatchStatusMatched}
			f := newQuoteFixture([]domain.QuoteItem{item}, map[uuid.UUID]domain.BranchPrice{id: branchPrice(id, "1200.50", nil)})
			price := f.prices.prices[id]
			price.Currency = "ARS"
			f.prices.prices[id] = price
			product := domain.Product{ID: id, AccountID: f.tenant.AccountID, Unit: &unit, IsActive: true}
			switch name {
			case "ambiguous":
				f.quotes.items[0].MatchStatus = domain.ItemMatchStatusAmbiguous
			case "unmatched":
				f.quotes.items[0].ProductID = nil
			case "missing-price":
				delete(f.prices.prices, id)
			case "zero-price":
				price.Price = decimal.Zero
				f.prices.prices[id] = price
			case "inactive":
				product.IsActive = false
			case "foreign-product":
				product.AccountID = uuid.New()
			case "incompatible-unit":
				other := "kg"
				product.Unit = &other
			case "unresolved-quantity":
				f.quotes.items[0].Quantity = decimal.Zero
			case "empty":
				f.quotes.items = nil
			case "later-version":
				f.quotes.version.VersionNumber = 2
			case "already-quoted":
				f.quotes.quote.CurrentStatus = domain.QuoteStatusQuoted
			case "sent":
				f.quotes.quote.CurrentStatus = domain.QuoteStatusSent
			case "archived":
				at := time.Now()
				f.quotes.quote.ArchivedAt = &at
			}
			f.service.WithRecommendationProducts(fakeRecommendationProducts{products: map[uuid.UUID]domain.Product{id: product}})
			f.service.WithAutomaticQuoteInputs(fakeAutomaticQuoteInputs{})
			var priced *domain.PricedQuote
			err := f.db.InTenantTx(context.Background(), f.tenant, func(q repository.Querier) error {
				var err error
				priced, err = f.service.TryAutoQuote(context.Background(), q, f.tenant, f.quoteID)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if name != "complete" && name != "zero-price" {
				if priced != nil || len(f.quotes.statusUpdates) != 0 || len(f.quotes.appliedPricings) != 0 {
					t.Fatal("ineligible quote was changed")
				}
				return
			}
			expected := decimal.RequireFromString("2401.00")
			if name == "zero-price" {
				expected = decimal.Zero
			}
			if priced == nil || !priced.Version.Total.Equal(expected) || priced.Version.IsImmutable || priced.Quote.CurrentStatus != domain.QuoteStatusQuoted {
				t.Fatalf("automatic valuation: %+v", priced)
			}
			if len(f.quotes.statusChanges) != 1 || f.quotes.statusChanges[0].userID != nil {
				t.Fatal("automatic transition must have no seller actor")
			}
		})
	}
}

type fakeAutomaticQuoteInputs struct{}

func (fakeAutomaticQuoteInputs) LockAutomaticQuoteInputs(context.Context, repository.Querier, uuid.UUID, uuid.UUID, []uuid.UUID) error {
	return nil
}
