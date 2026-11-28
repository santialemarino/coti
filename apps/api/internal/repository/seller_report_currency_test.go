//go:build integration

package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func seedReportAcceptedCurrency(t *testing.T, db *DB, accountID, branchID, productID, clientID uuid.UUID, currency, total string) {
	t.Helper()
	quoteID, versionID, itemID := seedQuoteChain(t, db, accountID, branchID, productID)
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	reportExec(t, db, "dated RFQ", `UPDATE rfq SET client_id = $2, received_at = $3
	   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`, quoteID, clientID, created)
	reportExec(t, db, "accepted quote", `UPDATE quote SET client_id = $2, current_status = 'ACCEPTED',
	   created_at = $3 WHERE id = $1`, quoteID, clientID, created)
	reportExec(t, db, "version currency", `UPDATE quote_version SET currency = $2, total = $3,
	   created_at = $4 WHERE id = $1`, versionID, currency, total, created)
	reportExec(t, db, "priced item", `UPDATE quote_item SET quantity = 1, unit_price_snapshot = $2,
	   subtotal = $2 WHERE id = $1`, itemID, total)
	reportExec(t, db, "acceptance event", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, accountID, quoteID, created.Add(time.Hour))
}

func TestSellerReportRepositorySeparatesAcceptedCurrencies(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Currency report")
	branchID := reportBranch(t, db, accountID)
	secondBranch := uuid.New()
	reportExec(t, db, "second branch", `INSERT INTO branch (id, account_id, name)
	   VALUES ($1, $2, 'USD branch')`, secondBranch, accountID)
	productID := seedProduct(t, db, accountID, "Shared product")
	clientID := seedClient(t, db, accountID)
	seedReportAcceptedCurrency(t, db, accountID, branchID, productID, clientID, "ARS", "100.00")
	seedReportAcceptedCurrency(t, db, accountID, secondBranch, productID, clientID, "USD", "10.00")

	foreignAccount := seedAccount(t, db, "Foreign currency report")
	seedReportAcceptedCurrency(t, db, foreignAccount, reportBranch(t, db, foreignAccount),
		seedProduct(t, db, foreignAccount, "Foreign product"), seedClient(t, db, foreignAccount), "EUR", "999.00")

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if report.KPIs.QuotesAccepted != 2 || report.KPIs.AcceptedAmount != nil || report.KPIs.AcceptedCurrency != nil {
		t.Fatalf("mixed-currency KPIs = %+v, want two acceptances and no scalar money", report.KPIs)
	}
	if groups := report.KPIs.AcceptedAmounts; len(groups) != 2 ||
		groups[0].Currency != "ARS" || groups[0].Amount.StringFixed(2) != "100.00" ||
		groups[1].Currency != "USD" || groups[1].Amount.StringFixed(2) != "10.00" {
		t.Fatalf("accepted amounts = %+v, want ARS 100 and USD 10 separately", groups)
	}
	if len(report.TopClients) != 2 || len(report.TopRevenueProducts) != 2 {
		t.Fatalf("rankings = %+v / %+v, want the same client and product in both currencies", report.TopClients, report.TopRevenueProducts)
	}
	for index, expected := range []struct{ currency, amount string }{{"ARS", "100.00"}, {"USD", "10.00"}} {
		client, product := report.TopClients[index], report.TopRevenueProducts[index]
		if client.ClientID == nil || *client.ClientID != clientID || client.Currency != expected.currency ||
			client.AcceptedAmount.StringFixed(2) != expected.amount || product.ProductID != productID ||
			product.Currency != expected.currency || product.Revenue.StringFixed(2) != expected.amount {
			t.Fatalf("currency ranking = %+v / %+v, want %+v", client, product, expected)
		}
	}

	in := reportQuery(accountID)
	in.BranchFilter = []uuid.UUID{secondBranch}
	single := buildReport(t, db, domain.Tenant{AccountID: accountID, BranchID: secondBranch}, in)
	if single.KPIs.AcceptedAmount == nil || single.KPIs.AcceptedAmount.StringFixed(2) != "10.00" ||
		single.KPIs.AcceptedCurrency == nil || *single.KPIs.AcceptedCurrency != "USD" || single.KPIs.QuotesAccepted != 1 {
		t.Fatalf("single-currency KPIs = %+v, want USD 10 from only the selected branch", single.KPIs)
	}
}

func TestSellerReportRepositoryRanksFiveResultsPerCurrency(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Currency rankings")
	branchID := reportBranch(t, db, accountID)
	for index := 0; index < 6; index++ {
		productID := seedProduct(t, db, accountID, fmt.Sprintf("Product %d", index))
		clientID := seedClient(t, db, accountID)
		seedReportAcceptedCurrency(t, db, accountID, branchID, productID, clientID, "ARS", fmt.Sprint(100+index))
		seedReportAcceptedCurrency(t, db, accountID, branchID, productID, clientID, "USD", fmt.Sprint(1+index))
	}
	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if len(report.TopClients) != 10 || len(report.TopRevenueProducts) != 10 {
		t.Fatalf("ranking sizes = %d clients / %d products, want five in each currency", len(report.TopClients), len(report.TopRevenueProducts))
	}
	for index := 0; index < 10; index++ {
		currency, total := "ARS", 105-index
		if index >= 5 {
			currency, total = "USD", 11-index
		}
		expected := fmt.Sprintf("%d.00", total)
		if client, product := report.TopClients[index], report.TopRevenueProducts[index]; client.Currency != currency || client.AcceptedAmount.StringFixed(2) != expected ||
			product.Currency != currency || product.Revenue.StringFixed(2) != expected {
			t.Fatalf("rank %d = %+v / %+v, want %s %s", index, client, product, currency, expected)
		}
	}
}

func TestSellerReportRepositoryKeepsResolvedZeroInItsCurrency(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Zero currency report")
	seedReportAcceptedCurrency(t, db, accountID, reportBranch(t, db, accountID),
		seedProduct(t, db, accountID, "Zero product"), seedClient(t, db, accountID), "USD", "0.00")
	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if report.KPIs.AcceptedAmount == nil || !report.KPIs.AcceptedAmount.IsZero() ||
		report.KPIs.AcceptedCurrency == nil || *report.KPIs.AcceptedCurrency != "USD" ||
		len(report.KPIs.AcceptedAmounts) != 1 || report.KPIs.QuotesAccepted != 1 {
		t.Fatalf("zero KPIs = %+v, want one known USD zero", report.KPIs)
	}
}
