//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func reportQuery(accountID uuid.UUID) domain.SellerReportQuery {
	return domain.SellerReportQuery{
		AccountID:      accountID,
		StartAt:        time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC),
		FromDate:       "2026-10-01",
		ToDate:         "2026-10-02",
		TimeZone:       domain.Argentina.String(),
		HighConfidence: decimal.RequireFromString("0.8000"),
	}
}

func buildReport(t *testing.T, db *DB, tenant domain.Tenant, in domain.SellerReportQuery) *domain.SellerReport {
	t.Helper()
	var report *domain.SellerReport
	if err := db.InTenantTx(context.Background(), tenant, func(q Querier) error {
		var err error
		report, err = NewSellerReportRepository().Build(context.Background(), q, in)
		return err
	}); err != nil {
		t.Fatalf("Build() = %v, want no error", err)
	}
	return report
}

func TestSellerReportRepositoryReadsAnEmptyPeriod(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes")

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if report.KPIs.RFQsReceived != 0 || report.KPIs.QuotesSent != 0 ||
		report.KPIs.QuotesAccepted != 0 {
		t.Fatalf("KPIs = %+v, want an empty period", report.KPIs)
	}
	if len(report.Timeline) != 2 ||
		report.Timeline[0].Date != "2026-10-01" || report.Timeline[1].Date != "2026-10-02" {
		t.Fatalf("timeline = %+v, want both days", report.Timeline)
	}
	if len(report.Branches) != 1 || report.Branches[0].BranchName != "Reportes Centro" {
		t.Fatalf("branches = %+v, want the account's branch", report.Branches)
	}
}

func TestSellerReportRepositoryCountsAuditedActivity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Reportes con actividad")
	var branchID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	productID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, 'Cemento')`,
		productID, accountID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product WHERE id = $1`, productID)
	})
	quoteID, versionID, itemID := seedQuoteChain(t, db, accountID, branchID, productID)

	generationID := uuid.New()
	for _, seed := range []struct {
		what string
		sql  string
		args []any
	}{
		{"rfq date", `UPDATE rfq SET received_at = $2 WHERE id =
		   (SELECT rfq_id FROM quote WHERE id = $1)`,
			[]any{quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"quote state", `UPDATE quote SET created_at = $2, current_status = 'ACCEPTED' WHERE id = $1`,
			[]any{quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"version total", `UPDATE quote_version SET total = 1000 WHERE id = $1`, []any{versionID}},
		{"sent", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
			[]any{accountID, quoteID, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC)}},
		{"accepted", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`,
			[]any{accountID, quoteID, time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)}},
		{"send", `INSERT INTO quote_send
		   (account_id, version_id, channel_id, format, sent_at, tracking_status)
		   SELECT $1, $2, rfq.channel_id, 'PDF', $3, 'SENT'
		   FROM quote JOIN rfq ON rfq.account_id = quote.account_id AND rfq.id = quote.rfq_id
		   WHERE quote.id = $4`,
			[]any{accountID, versionID,
				time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC), quoteID}},
		{"generation", `INSERT INTO quote_ai_generation
		   (id, account_id, quote_id, quote_version_id, provider, model, prompt_version,
		    schema_version, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
		   VALUES ($1, $2, $3, $4, 'test', 'test', '1', '1', 0, 0, 0, 0)`,
			[]any{generationID, accountID, quoteID, versionID}},
		{"generation matched", `INSERT INTO quote_ai_generation_item
		   (account_id, generation_id, position, source_quote_item_id, product_id,
		    requested_description, quantity, quantity_source, quantity_rationale,
		    match_status, confidence_score)
		   VALUES ($1, $2, 0, $3, $4, 'cemento', 10, 'EXPLICIT', 'explicit', 'MATCHED', 0.9)`,
			[]any{accountID, generationID, itemID, productID}},
		{"generation unmatched", `INSERT INTO quote_ai_generation_item
		   (account_id, generation_id, position, source_quote_item_id, product_id,
		    requested_description, quantity, quantity_source, quantity_rationale,
		    match_status, confidence_score)
		   VALUES ($1, $2, 1, $3, NULL, 'cal', 2, 'EXPLICIT', 'explicit', 'NO_MATCH', 0.2)`,
			[]any{accountID, generationID, itemID}},
	} {
		if _, err := db.CrossAccount().Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed %s: %v", seed.what, err)
		}
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_ai_generation_item WHERE generation_id = $1`, generationID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_ai_generation WHERE id = $1`, generationID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_send WHERE version_id = $1`, versionID)
	})

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if report.KPIs.RFQsReceived != 1 || report.KPIs.QuotesSent != 1 ||
		report.KPIs.QuotesAccepted != 1 {
		t.Fatalf("KPIs = %+v, want one order sent and accepted", report.KPIs)
	}
	if report.KPIs.AcceptedAmount.StringFixed(2) != "1000.00" {
		t.Fatalf("accepted amount = %s, want 1000.00", report.KPIs.AcceptedAmount)
	}
	if report.KPIs.ConversionRate == nil || report.KPIs.ConversionRate.StringFixed(2) != "1.00" {
		t.Fatalf("conversion = %v, want 1.00", report.KPIs.ConversionRate)
	}
	if report.KPIs.ResponseTime.AverageSeconds == nil ||
		*report.KPIs.ResponseTime.AverageSeconds != 3600 {
		t.Fatalf("response time = %+v, want one hour", report.KPIs.ResponseTime)
	}
	if len(report.QuotesByStatus) != 1 || report.QuotesByStatus[0].Status !=
		domain.QuoteStatusAccepted || report.QuotesByStatus[0].Count != 1 {
		t.Fatalf("statuses = %+v, want one accepted quote", report.QuotesByStatus)
	}
	if len(report.TopProducts) != 1 || report.TopProducts[0].Name != "Cemento" ||
		report.TopProducts[0].Requests != 1 {
		t.Fatalf("products = %+v, want the quoted cement", report.TopProducts)
	}
	if len(report.TopClients) != 0 {
		t.Fatalf("clients = %+v, want none: the order named nobody", report.TopClients)
	}
	if report.AIMatch.Total != 2 || report.AIMatch.Matched != 1 ||
		report.AIMatch.Confidence.Low != 1 || report.AIMatch.Confidence.High != 1 {
		t.Fatalf("AI match = %+v, want one low and one high of two", report.AIMatch)
	}
	confidence := report.AIMatch.Confidence
	if confidence.Low+confidence.Medium+confidence.High != report.AIMatch.Total {
		t.Fatalf("confidence buckets = %+v, want every scored item bucketed", confidence)
	}
	if report.Timeline[0].Sent != 1 || report.Timeline[0].Accepted != 1 {
		t.Fatalf("timeline = %+v, want the first day's send and acceptance", report.Timeline)
	}
}

func TestSellerReportRepositoryHidesAnotherSellersOrders(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Reportes por vendedor")
	var branchID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	productID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, 'Arena')`,
		productID, accountID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product WHERE id = $1`, productID)
	})
	otherSeller := seedUser(t, db, accountID, "SELLER")
	sellerID := seedUser(t, db, accountID, "SELLER")
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	for _, seed := range []struct {
		what string
		sql  string
		args []any
	}{
		{"rfq date", `UPDATE rfq SET received_at = $2 WHERE id =
		   (SELECT rfq_id FROM quote WHERE id = $1)`,
			[]any{quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"other seller", `UPDATE quote SET seller_id = $2 WHERE id = $1`,
			[]any{quoteID, otherSeller}},
	} {
		if _, err := db.CrossAccount().Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed %s: %v", seed.what, err)
		}
	}

	in := reportQuery(accountID)
	in.SellerID = &sellerID
	report := buildReport(t, db, domain.Tenant{
		AccountID: accountID, UserID: sellerID, Role: domain.UserRoleSeller, BranchID: branchID,
	}, in)
	if report.KPIs.RFQsReceived != 0 {
		t.Fatalf("a seller saw another seller's order: %+v", report.KPIs)
	}
}

func TestSellerReportRepositorySeparatesLineCountsFromAcceptedRevenue(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Reportes de productos")
	var branchID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	productID := seedProduct(t, db, accountID, "Cemento")
	acceptedQuoteID, acceptedVersionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	otherQuoteID, otherVersionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_send WHERE version_id IN ($1, $2)`,
			acceptedVersionID, otherVersionID)
	})

	exec := func(what, query string, args ...any) {
		t.Helper()
		if _, err := db.CrossAccount().Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	exec("accepted rfq date", `UPDATE rfq SET received_at = $2 WHERE id =
	   (SELECT rfq_id FROM quote WHERE id = $1)`,
		acceptedQuoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	exec("other rfq date", `UPDATE rfq SET received_at = $2 WHERE id =
	   (SELECT rfq_id FROM quote WHERE id = $1)`,
		otherQuoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	exec("accepted quote", `UPDATE quote SET created_at = $2, current_status = 'ACCEPTED'
	   WHERE id = $1`, acceptedQuoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	exec("accepted version", `UPDATE quote_version SET total = 1000 WHERE id = $1`,
		acceptedVersionID)
	exec("accepted line", `UPDATE quote_item
	   SET quantity = 2, unit_price_snapshot = 500, subtotal = 1000
	   WHERE version_id = $1`, acceptedVersionID)
	exec("other version", `UPDATE quote_version SET total = 5000 WHERE id = $1`, otherVersionID)
	exec("other line", `UPDATE quote_item
	   SET quantity = 10, unit_price_snapshot = 500, subtotal = 5000
	   WHERE version_id = $1`, otherVersionID)
	exec("send", `INSERT INTO quote_send
	   (account_id, version_id, channel_id, format, sent_at, tracking_status)
	   SELECT $1, $2, rfq.channel_id, 'PDF', $3, 'SENT'
	   FROM quote JOIN rfq ON rfq.account_id = quote.account_id AND rfq.id = quote.rfq_id
	   WHERE quote.id = $4`,
		accountID, acceptedVersionID, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC),
		acceptedQuoteID)
	exec("accepted", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`,
		accountID, acceptedQuoteID, time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC))

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if len(report.TopProducts) != 1 || report.TopProducts[0].Requests != 2 {
		t.Fatalf("requested products = %+v, want two line occurrences", report.TopProducts)
	}
	if len(report.TopRevenueProducts) != 1 ||
		report.TopRevenueProducts[0].Revenue.StringFixed(2) != "1000.00" {
		t.Fatalf("revenue products = %+v, want only the accepted line value",
			report.TopRevenueProducts)
	}
}

func TestSellerReportRepositoryResolvesTheAcceptedVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cases := []struct {
		name           string
		laterVersionAt *time.Time
		sendAccepted   bool
		actionLater    bool
		wantAmount     string
	}{
		{
			name:           "customer action wins over the last send",
			laterVersionAt: timePtr(time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)),
			sendAccepted:   true,
			actionLater:    true,
			wantAmount:     "2000.00",
		},
		{
			name:           "a later version blocks the current fallback",
			laterVersionAt: timePtr(time.Date(2026, 10, 1, 16, 0, 0, 0, time.UTC)),
			wantAmount:     "0.00",
		},
		{
			name:       "current version when nothing was sent or changed later",
			wantAmount: "1000.00",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accountID := seedAccount(t, db, "Resolucion "+uuid.NewString())
			var branchID uuid.UUID
			if err := db.CrossAccount().QueryRow(ctx,
				`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
				t.Fatal(err)
			}
			productID := seedProduct(t, db, accountID, "Cal")
			quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)

			laterVersionID := uuid.Nil
			if tc.laterVersionAt != nil {
				laterVersionID = uuid.New()
			}
			t.Cleanup(func() {
				mustCleanup(t, db.CrossAccount(),
					`DELETE FROM client_action WHERE version_id = $1 OR version_id = $2`,
					versionID, laterVersionID)
				mustCleanup(t, db.CrossAccount(),
					`DELETE FROM quote_send WHERE version_id = $1 OR version_id = $2`,
					versionID, laterVersionID)
				if laterVersionID != uuid.Nil {
					mustCleanup(t, db.CrossAccount(),
						`DELETE FROM quote_item WHERE version_id = $1`, laterVersionID)
					mustCleanup(t, db.CrossAccount(),
						`UPDATE quote SET current_version_id = NULL WHERE id = $1`, quoteID)
					mustCleanup(t, db.CrossAccount(),
						`DELETE FROM quote_version WHERE id = $1`, laterVersionID)
				}
			})

			exec := func(what, query string, args ...any) {
				t.Helper()
				if _, err := db.CrossAccount().Exec(ctx, query, args...); err != nil {
					t.Fatalf("seed %s: %v", what, err)
				}
			}
			exec("version one", `UPDATE quote_version SET total = 1000, created_at = $2,
			   is_immutable = TRUE, frozen_at = $2 WHERE id = $1`,
				versionID, time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC))
			exec("version one line", `UPDATE quote_item
			   SET quantity = 10, unit_price_snapshot = 100, subtotal = 1000
			   WHERE version_id = $1`, versionID)
			if laterVersionID != uuid.Nil {
				exec("later version", `INSERT INTO quote_version
				   (id, account_id, quote_id, version_number, total, created_at)
				   VALUES ($1, $2, $3, 2, 2000, $4)`,
					laterVersionID, accountID, quoteID, *tc.laterVersionAt)
				exec("later version line", `INSERT INTO quote_item
				   (account_id, version_id, product_id, requested_description, quantity,
				    unit_price_snapshot, subtotal, match_status)
				   VALUES ($1, $2, $3, 'cal', 10, 200, 2000, 'MATCHED')`,
					accountID, laterVersionID, productID)
			}

			acceptedAt := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
			currentVersionID := versionID
			if laterVersionID != uuid.Nil {
				currentVersionID = laterVersionID
			}
			exec("rfq", `UPDATE rfq SET received_at = $2, client_label = 'Obra'
			   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
				quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
			exec("quote", `UPDATE quote SET created_at = $2, current_status = 'ACCEPTED',
			   current_version_id = $3 WHERE id = $1`,
				quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), currentVersionID)
			exec("accepted", `INSERT INTO quote_status_change
			   (account_id, quote_id, previous_status, new_status, changed_at)
			   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, accountID, quoteID, acceptedAt)
			if tc.sendAccepted {
				exec("send", `INSERT INTO quote_send
				   (account_id, version_id, channel_id, format, sent_at, tracking_status)
				   SELECT $1, $2, rfq.channel_id, 'PDF', $3, 'SENT'
				   FROM quote JOIN rfq ON rfq.account_id = quote.account_id AND rfq.id = quote.rfq_id
				   WHERE quote.id = $4`,
					accountID, versionID, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC), quoteID)
			}
			if tc.actionLater {
				exec("client action", `INSERT INTO client_action
				   (account_id, version_id, type, created_at)
				   VALUES ($1, $2, 'ACCEPT', $3)`,
					accountID, laterVersionID, acceptedAt.Add(time.Minute))
			}

			report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
			if report.KPIs.QuotesAccepted != 1 {
				t.Fatalf("accepted quotes = %d, want 1", report.KPIs.QuotesAccepted)
			}
			if got := report.KPIs.AcceptedAmount.StringFixed(2); got != tc.wantAmount {
				t.Fatalf("accepted amount = %s, want %s", got, tc.wantAmount)
			}
			if tc.wantAmount == "0.00" {
				if len(report.TopClients) != 0 {
					t.Fatalf("top clients = %+v, want none for an unresolved acceptance", report.TopClients)
				}
				if len(report.TopRevenueProducts) != 0 {
					t.Fatalf("revenue products = %+v, want none for an unresolved acceptance",
						report.TopRevenueProducts)
				}
				return
			}
			if len(report.TopClients) != 1 ||
				report.TopClients[0].AcceptedAmount.StringFixed(2) != tc.wantAmount {
				t.Fatalf("top clients = %+v, want the resolved version", report.TopClients)
			}
			if len(report.TopRevenueProducts) != 1 ||
				report.TopRevenueProducts[0].Revenue.StringFixed(2) != tc.wantAmount {
				t.Fatalf("revenue products = %+v, want the resolved version", report.TopRevenueProducts)
			}
		})
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
