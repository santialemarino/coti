//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestRFQAutomaticQuote_UsesManualValuationAndKeepsMissingPricesDraft(t *testing.T) {
	for _, scenario := range []string{"priced", "missing-price", "multi-line-promotion"} {
		t.Run(scenario, func(t *testing.T) {
			priced := scenario != "missing-price"
			e := newEnv(t)
			account, branch := e.seedAccount(t, "Automatic quote")
			user := e.seedUser(t, account, domain.UserRoleAdmin)
			product := e.seedProduct(t, account, "Cemento Portland 50kg", "bolsa de cemento")
			e.stock(t, account, branch, product)
			e.embedOn(t, product, 0, 1)
			if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE product SET unit='bolsa' WHERE account_id=$1 AND id=$2`, account, product); err != nil {
				t.Fatal(err)
			}
			if priced {
				e.openPricePeriod(t, account, branch, product, "1200.50", nil)
			}
			channel := e.seedIntakeChannel(t, account, branch)
			unit := "bolsa"
			extractor := stagedExtractor{lines: []domain.ExtractedRFQLine{{RequestedDescription: "cemento portland 50kg", Quantity: decimal.NewFromInt(2), Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags explicitly requested"}}}
			if scenario == "multi-line-promotion" {
				extractor.lines = append(extractor.lines, domain.ExtractedRFQLine{RequestedDescription: "cemento portland 50kg", Quantity: decimal.NewFromInt(3), Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Three more bags explicitly requested"})
				if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO promotion (account_id,name,condition_type,action_type,action_value,min_total) VALUES ($1,'Automatic quote parity','ON_TOTAL','PERCENTAGE',10,100)`, account); err != nil {
					t.Fatal(err)
				}
			}
			quotes := repository.NewQuoteRepository()
			quoter := services.NewQuoteService(e.db, quotes, repository.NewProductPriceRepository(), domain.ARCAEnvironmentHomologation, nil).
				WithAutomaticQuoteInputs(repository.NewProductPriceRepository()).
				WithRecommendationProducts(repository.NewProductRepository()).
				WithDiscountEngine(repository.NewQuoteDiscountRepository(), repository.NewPromotionRepository(), quotes)
			pipeline := e.pipeline(t, extractor, map[string]int{"cemento portland 50kg": 0}).WithAutomaticQuoting(quoter)
			tenant := domain.Tenant{AccountID: account, BranchID: branch, UserID: user.ID, Role: domain.UserRoleAdmin}
			draft, err := pipeline.CreateTextDraft(context.Background(), tenant, domain.TextRFQDraftInput{ChannelID: channel, RawText: "2 bolsas cemento portland 50kg"})
			if err != nil {
				t.Fatal(err)
			}
			e.dropDraft(t, draft.RFQ.ID)
			expected := domain.QuoteStatusDraft
			if priced {
				expected = domain.QuoteStatusQuoted
			}
			if draft.Quote.CurrentStatus != expected {
				t.Fatalf("status=%s, want %s; items=%+v", draft.Quote.CurrentStatus, expected, draft.Items)
			}
			if !priced {
				return
			}
			manual, err := e.pipeline(t, extractor, map[string]int{"cemento portland 50kg": 0}).CreateTextDraft(context.Background(), tenant, domain.TextRFQDraftInput{ChannelID: channel, RawText: "2 bolsas cemento portland 50kg"})
			if err != nil {
				t.Fatal(err)
			}
			e.dropDraft(t, manual.RFQ.ID)
			manualPriced, err := quoter.AcceptMaterials(context.Background(), tenant, manual.Quote.ID)
			if err != nil {
				t.Fatal(err)
			}
			expectedTotal := decimal.RequireFromString("2401.00")
			if scenario == "multi-line-promotion" {
				expectedTotal = decimal.RequireFromString("5402.25")
			}
			if !draft.Version.Total.Equal(expectedTotal) || !draft.Version.Total.Equal(manualPriced.Version.Total) {
				t.Fatalf("auto=%s manual=%s", draft.Version.Total, manualPriced.Version.Total)
			}
			var sends int
			if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM quote_send WHERE account_id=$1 AND version_id=$2`, account, draft.Version.ID).Scan(&sends); err != nil {
				t.Fatal(err)
			}
			if sends != 0 || draft.Version.IsImmutable {
				t.Fatal("automatic valuation published the offer")
			}
		})
	}
}
