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
		MultiBranch:    true,
		StartAt:        time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC),
		FromDate:       "2026-10-01",
		ToDate:         "2026-10-02",
		TimeZone:       domain.Argentina.String(),
		Granularity:    domain.ReportGranularityDay,
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
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM handler_decision WHERE quote_version_id = $1`,
			versionID)
	})
	inPeriod := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
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
		// Flag-only, apart: the quote trigger clears the flag on any other write.
		{"follow-up", `UPDATE quote SET needs_followup = TRUE, followup_flagged_at = $2 WHERE id = $1`,
			[]any{quoteID, inPeriod}},
		{"unmatched line", `UPDATE quote_item SET match_status = 'NO_MATCH', product_id = NULL
		   WHERE version_id = $1`, []any{versionID}},
		{"decision", `INSERT INTO handler_decision
		   (account_id, quote_version_id, seller_decision, created_at)
		   VALUES ($1, $2, 'APPROVED_AS_IS', $3)`, []any{accountID, versionID, inPeriod}},
		{"sent", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
			[]any{accountID, quoteID, inPeriod.Add(-time.Hour)}},
		{"accepted", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, []any{accountID, quoteID, inPeriod}},
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
	if report.Assistant.Proposals != 0 || report.KPIs.FollowUpPending != 0 ||
		report.KPIs.TimeToAcceptance.Sample != 0 || report.AIMatch.VersionLines != 0 {
		t.Fatalf("a seller saw another seller's metrics: assistant %+v, follow-up %d, wait %+v, lines %d",
			report.Assistant, report.KPIs.FollowUpPending, report.KPIs.TimeToAcceptance,
			report.AIMatch.VersionLines)
	}

	admin := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if admin.Assistant.Proposals != 1 || admin.KPIs.FollowUpPending != 1 ||
		admin.KPIs.TimeToAcceptance.Sample != 1 || admin.AIMatch.VersionUnmatched != 1 {
		t.Fatalf("the administrator's report = assistant %+v, follow-up %d, wait %+v, unmatched %d; "+
			"want the hidden quote's figures counted", admin.Assistant, admin.KPIs.FollowUpPending,
			admin.KPIs.TimeToAcceptance, admin.AIMatch.VersionUnmatched)
	}
}

func TestSellerReportRepositoryAppliesStatusChannelSellerAndClientFilters(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Reportes filtrados")
	var branchID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	productID := uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, 'Cal')`,
		productID, accountID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product WHERE id = $1`, productID)
	})
	sellerA := seedUser(t, db, accountID, "SELLER")
	sellerB := seedUser(t, db, accountID, "SELLER")
	clientA, clientB := uuid.New(), uuid.New()
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO client (id, account_id, name) VALUES ($1, $2, 'Obra Norte'), ($3, $2, 'Obra Sur')`,
		clientA, accountID, clientB); err != nil {
		t.Fatalf("seed clients: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM client WHERE id = ANY($1)`,
			[]uuid.UUID{clientA, clientB})
	})

	quoteA, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	quoteB, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	for _, seed := range []struct {
		what string
		sql  string
		args []any
	}{
		{"quote A", `UPDATE quote SET seller_id = $2, current_status = 'SENT', created_at = $3
		             WHERE id = $1`,
			[]any{quoteA, sellerA, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"quote B", `UPDATE quote SET seller_id = $2, current_status = 'ACCEPTED', created_at = $3
		             WHERE id = $1`,
			[]any{quoteB, sellerB, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"sent A", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
			[]any{accountID, quoteA, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC)}},
		{"sent B", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
			[]any{accountID, quoteB, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC)}},
		{"rfq A", `UPDATE rfq SET client_id = $2, received_at = $3
		           WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			[]any{quoteA, clientA, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"rfq B", `UPDATE rfq SET client_id = $2, received_at = $3
		           WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			[]any{quoteB, clientB, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}},
		{"channel B", `UPDATE channel SET type = 'EMAIL', identifier = $2
		               WHERE id = (SELECT rfq.channel_id FROM quote JOIN rfq
		                           ON rfq.account_id = quote.account_id AND rfq.id = quote.rfq_id
		                           WHERE quote.id = $1)`,
			[]any{quoteB, "reportes-" + uuid.NewString() + "@example.com"}},
	} {
		if _, err := db.CrossAccount().Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed %s: %v", seed.what, err)
		}
	}

	tests := []struct {
		name  string
		apply func(*domain.SellerReportQuery)
		want  int
	}{
		{"status", func(in *domain.SellerReportQuery) {
			in.Statuses = []domain.QuoteStatus{domain.QuoteStatusSent}
		}, 1},
		{"channel", func(in *domain.SellerReportQuery) {
			channel := domain.ChannelTypeEmail
			in.Channel = &channel
		}, 1},
		{"seller", func(in *domain.SellerReportQuery) { in.SellerFilter = &sellerA }, 1},
		{"client", func(in *domain.SellerReportQuery) { in.ClientFilter = &clientA }, 1},
		{"combined match", func(in *domain.SellerReportQuery) {
			channel := domain.ChannelTypeWhatsApp
			in.Statuses = []domain.QuoteStatus{domain.QuoteStatusSent}
			in.Channel = &channel
			in.SellerFilter = &sellerA
			in.ClientFilter = &clientA
		}, 1},
		{"combined mismatch", func(in *domain.SellerReportQuery) {
			channel := domain.ChannelTypeEmail
			in.Statuses = []domain.QuoteStatus{domain.QuoteStatusSent}
			in.Channel = &channel
			in.SellerFilter = &sellerA
			in.ClientFilter = &clientA
		}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := reportQuery(accountID)
			tt.apply(&in)
			report := buildReport(t, db, domain.Tenant{AccountID: accountID}, in)
			if report.KPIs.RFQsReceived != tt.want {
				t.Fatalf("RFQsReceived = %d, want %d", report.KPIs.RFQsReceived, tt.want)
			}
			if report.KPIs.QuotesSent != tt.want {
				t.Fatalf("QuotesSent = %d, want %d", report.KPIs.QuotesSent, tt.want)
			}
			byStatus := 0
			for _, status := range report.QuotesByStatus {
				byStatus += status.Count
			}
			if byStatus != tt.want {
				t.Fatalf("quotes by status = %+v, want %d in total", report.QuotesByStatus, tt.want)
			}
			if report.Timeline[0].Sent != tt.want {
				t.Fatalf("timeline = %+v, want %d sent on the first day", report.Timeline, tt.want)
			}
			if len(report.TopProducts) != min(tt.want, 1) ||
				(tt.want > 0 && report.TopProducts[0].Requests != tt.want) {
				t.Fatalf("products = %+v, want %d requests", report.TopProducts, tt.want)
			}
			if len(report.Branches) != 1 || report.Branches[0].RFQs != tt.want {
				t.Fatalf("branches = %+v, want %d orders", report.Branches, tt.want)
			}
		})
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
			name:           "the last send wins over an unsent newer version",
			laterVersionAt: timePtr(time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)),
			sendAccepted:   true,
			wantAmount:     "1000.00",
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

func TestSellerReportRepositoryMeasuresConversionAsACohort(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes cohorte")
	otherAccountID := seedAccount(t, db, "Reportes cohorte ajena")
	branchID := reportBranch(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Ladrillo")
	earlier, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	earlierToo, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	converted, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	open, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	day := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.UTC) }
	change := func(quoteID uuid.UUID, status string, at time.Time) {
		t.Helper()
		reportExec(t, db, "status change", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', $3, $4)`, accountID, quoteID, status, at)
	}
	change(earlier, "SENT", time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	change(earlier, "ACCEPTED", day(1, 12))
	change(earlierToo, "SENT", time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	change(earlierToo, "ACCEPTED", day(2, 12))
	change(converted, "SENT", day(1, 13))
	change(converted, "ACCEPTED", day(10, 12))
	change(open, "SENT", day(2, 13))

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	kpis := report.KPIs
	if kpis.QuotesSent != 2 || kpis.SentAccepted != 1 || kpis.QuotesAccepted != 2 {
		t.Fatalf("KPIs = %+v, want two sent, one of them accepted since, two acceptances in the period",
			kpis)
	}
	if kpis.ConversionRate == nil || kpis.ConversionRate.StringFixed(2) != "0.50" {
		t.Fatalf("conversion = %v, want the cohort share 0.50", kpis.ConversionRate)
	}

	other := buildReport(t, db, domain.Tenant{AccountID: otherAccountID}, reportQuery(otherAccountID))
	if other.KPIs.QuotesSent != 0 || other.KPIs.QuotesAccepted != 0 {
		t.Fatalf("another account's report = %+v, want none of this account's activity", other.KPIs)
	}
}

func TestSellerReportRepositoryRanksANamelessClientOnce(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes cliente sin nombre")
	branchID := reportBranch(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Arena")
	nameless, named := uuid.New(), uuid.New()
	reportExec(t, db, "clients", `INSERT INTO client (id, account_id, name, email)
	   VALUES ($1, $2, NULL, 'juan@example.com'), ($3, $2, 'Obra Grande', NULL)`,
		nameless, accountID, named)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `UPDATE rfq SET client_id = NULL WHERE client_id = ANY($1)`,
			[]uuid.UUID{nameless, named})
		mustCleanup(t, db.CrossAccount(), `UPDATE quote SET client_id = NULL WHERE client_id = ANY($1)`,
			[]uuid.UUID{nameless, named})
		mustCleanup(t, db.CrossAccount(), `DELETE FROM client WHERE id = ANY($1)`,
			[]uuid.UUID{nameless, named})
	})
	for _, seed := range []struct {
		client uuid.UUID
		label  string
		total  string
	}{
		{nameless, "Juan", "600"},
		{nameless, "Juan Perez", "600"},
		{named, "", "1000"},
	} {
		quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
		reportExec(t, db, "rfq", `UPDATE rfq SET client_id = $2, client_label = NULLIF($3, ''),
		   received_at = $4 WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			quoteID, seed.client, seed.label, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
		reportExec(t, db, "quote", `UPDATE quote SET client_id = $2, current_status = 'ACCEPTED'
		   WHERE id = $1`, quoteID, seed.client)
		reportExec(t, db, "total", `UPDATE quote_version SET total = $2, created_at = $3 WHERE id = $1`,
			versionID, seed.total, time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC))
		reportExec(t, db, "accepted", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`,
			accountID, quoteID, time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC))
	}

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if len(report.TopClients) != 2 {
		t.Fatalf("top clients = %+v, want one row per client", report.TopClients)
	}
	first := report.TopClients[0]
	if first.ClientID == nil || *first.ClientID != nameless || first.Name != "juan@example.com" ||
		first.AcceptedAmount.StringFixed(2) != "1200.00" {
		t.Fatalf("first client = %+v, want the nameless client's 1200 under its email", first)
	}
}

func TestSellerReportRepositoryBucketsDaysInArgentina(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes huso horario")
	branchID := reportBranch(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cal")
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	// 02:00 UTC on the 2nd is 23:00 on the 1st in Buenos Aires.
	reportExec(t, db, "sent", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
		accountID, quoteID, time.Date(2026, 10, 2, 2, 0, 0, 0, time.UTC))

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if report.Timeline[0].Sent != 1 || report.Timeline[1].Sent != 0 {
		t.Fatalf("timeline = %+v, want the send on the Argentine day it happened", report.Timeline)
	}
}

func TestSellerReportRepositoryBreaksDownASellersBranches(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes vendedor multisucursal")
	centro := reportBranch(t, db, accountID)
	norte, ajena := uuid.New(), uuid.New()
	reportExec(t, db, "branches", `INSERT INTO branch (id, account_id, name)
	   VALUES ($1, $3, 'Norte'), ($2, $3, 'Ajena')`, norte, ajena, accountID)
	sellerID := seedUser(t, db, accountID, "SELLER")
	productID := seedProduct(t, db, accountID, "Hierro")
	for _, branchID := range []uuid.UUID{centro, norte, ajena} {
		quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
		reportExec(t, db, "rfq", `UPDATE rfq SET received_at = $2
		   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	}

	in := reportQuery(accountID)
	in.SellerID = &sellerID
	in.BranchFilter = []uuid.UUID{centro, norte}
	report := buildReport(t, db, domain.Tenant{AccountID: accountID, UserID: sellerID,
		Role: domain.UserRoleSeller, AllowedBranchIDs: in.BranchFilter}, in)
	if report.KPIs.RFQsReceived != 2 {
		t.Fatalf("orders = %d, want the two reachable branches", report.KPIs.RFQsReceived)
	}
	if len(report.Branches) != 2 {
		t.Fatalf("branches = %+v, want only the seller's two branches", report.Branches)
	}
	for _, branch := range report.Branches {
		if branch.BranchID == ajena || branch.RFQs != 1 {
			t.Fatalf("branches = %+v, want one order in each reachable branch", report.Branches)
		}
	}
}

func TestSellerReportRepositoryFiltersByTagAndBranch(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes por etiqueta")
	centro := reportBranch(t, db, accountID)
	norte := uuid.New()
	reportExec(t, db, "branch", `INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Norte')`,
		norte, accountID)
	productID := seedProduct(t, db, accountID, "Cemento")
	tagged, otherTagged, tagID, otherTagID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	reportExec(t, db, "clients", `INSERT INTO client (id, account_id, name)
	   VALUES ($1, $3, 'Constructora'), ($2, $3, 'Particular')`, tagged, otherTagged, accountID)
	reportExec(t, db, "tags", `INSERT INTO tag (id, account_id, name)
	   VALUES ($1, $3, 'Mayorista'), ($2, $3, 'Minorista')`, tagID, otherTagID, accountID)
	// The other client carries a different tag, so "has any tag" cannot pass for "has this tag".
	reportExec(t, db, "client tags", `INSERT INTO client_tag (account_id, client_id, tag_id)
	   VALUES ($1, $2, $3), ($1, $4, $5)`, accountID, tagged, tagID, otherTagged, otherTagID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM client_tag WHERE tag_id = ANY($1)`,
			[]uuid.UUID{tagID, otherTagID})
		mustCleanup(t, db.CrossAccount(), `DELETE FROM tag WHERE id = ANY($1)`,
			[]uuid.UUID{tagID, otherTagID})
		mustCleanup(t, db.CrossAccount(), `UPDATE rfq SET client_id = NULL WHERE client_id = ANY($1)`,
			[]uuid.UUID{tagged, otherTagged})
		mustCleanup(t, db.CrossAccount(), `DELETE FROM client WHERE id = ANY($1)`,
			[]uuid.UUID{tagged, otherTagged})
	})
	for _, seed := range []struct {
		branch, client uuid.UUID
	}{{centro, tagged}, {norte, tagged}, {centro, otherTagged}} {
		quoteID, _, _ := seedQuoteChain(t, db, accountID, seed.branch, productID)
		reportExec(t, db, "rfq", `UPDATE rfq SET client_id = $2, received_at = $3
		   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			quoteID, seed.client, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
		reportExec(t, db, "sent", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`,
			accountID, quoteID, time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC))
	}

	in := reportQuery(accountID)
	in.TagFilter = []uuid.UUID{tagID}
	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, in)
	if report.KPIs.RFQsReceived != 2 || report.KPIs.QuotesSent != 2 {
		t.Fatalf("KPIs = %+v, want only the tagged client's two orders", report.KPIs)
	}
	branchOrders := 0
	for _, branch := range report.Branches {
		branchOrders += branch.RFQs
	}
	if branchOrders != 2 {
		t.Fatalf("branches = %+v, want the breakdown narrowed by the tag too", report.Branches)
	}
	if len(report.Filters.Tags) != 1 || report.Filters.Tags[0].Name != "Mayorista" {
		t.Fatalf("tag echo = %+v, want the tag by name", report.Filters.Tags)
	}

	in = reportQuery(accountID)
	in.BranchFilter = []uuid.UUID{norte}
	in.RequestedBranches = in.BranchFilter
	in.MultiBranch = false
	report = buildReport(t, db, domain.Tenant{AccountID: accountID}, in)
	if report.KPIs.RFQsReceived != 1 {
		t.Fatalf("orders = %d, want the one in the chosen branch", report.KPIs.RFQsReceived)
	}
	if len(report.Filters.Branches) != 1 || report.Filters.Branches[0].Name != "Norte" {
		t.Fatalf("branch echo = %+v, want the chosen branch by name", report.Filters.Branches)
	}
}

func TestSellerReportRepositoryGroupsUnmatchedTextAndHidesIdleClosedBranches(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes texto libre")
	branchID := reportBranch(t, db, accountID)
	closed := uuid.New()
	reportExec(t, db, "closed branch", `INSERT INTO branch (id, account_id, name, is_active)
	   VALUES ($1, $2, 'Cerrada', FALSE)`, closed, accountID)
	productID := seedProduct(t, db, accountID, "Cemento")
	for _, description := range []string{"Malla sima", " malla SIMA "} {
		quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
		reportExec(t, db, "rfq", `UPDATE rfq SET received_at = $2
		   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
			quoteID, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
		reportExec(t, db, "unmatched line", `UPDATE quote_item
		   SET product_id = NULL, match_status = 'NO_MATCH', requested_description = $2
		   WHERE version_id = $1`, versionID, description)
	}

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	if len(report.TopProducts) != 1 || report.TopProducts[0].Requests != 2 {
		t.Fatalf("products = %+v, want both spellings counted as one request text", report.TopProducts)
	}
	for _, branch := range report.Branches {
		if branch.BranchID == closed {
			t.Fatalf("branches = %+v, want the closed branch with no orders left out", report.Branches)
		}
	}
	if len(report.Branches) != 1 {
		t.Fatalf("branches = %+v, want only the active branch", report.Branches)
	}
}

func TestSellerReportRepositoryBucketsTheTrendByWeekAndMonth(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes tendencia")
	branchID := reportBranch(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Arena")
	// Buenos Aires midday, so no event sits near a day boundary.
	at := func(month time.Month, day int) time.Time {
		return time.Date(2026, month, day, 15, 0, 0, 0, time.UTC)
	}
	converted, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	open, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	lateNight, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	// 01:00 UTC on 1 October is still 30 September in Buenos Aires.
	reportExec(t, db, "late-night order", `UPDATE rfq SET received_at = $2
	   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`,
		lateNight, time.Date(2026, time.October, 1, 1, 0, 0, 0, time.UTC))
	// An acceptance before the first send is not a conversion of that send.
	reportExec(t, db, "early acceptance", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'QUOTED', 'ACCEPTED', $3)`, accountID, open, at(time.October, 12))
	for _, seed := range []struct {
		quote    uuid.UUID
		received time.Time
		sent     time.Time
		accepted *time.Time
	}{
		{converted, at(time.October, 2), at(time.October, 6), timePtr(at(time.October, 7))},
		{open, at(time.September, 20), at(time.October, 13), nil},
	} {
		reportExec(t, db, "rfq", `UPDATE rfq SET received_at = $2
		   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`, seed.quote, seed.received)
		reportExec(t, db, "sent", `INSERT INTO quote_status_change
		   (account_id, quote_id, previous_status, new_status, changed_at)
		   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`, accountID, seed.quote, seed.sent)
		if seed.accepted != nil {
			reportExec(t, db, "accepted", `INSERT INTO quote_status_change
			   (account_id, quote_id, previous_status, new_status, changed_at)
			   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, accountID, seed.quote, *seed.accepted)
		}
	}
	period := func(from, to string, granularity domain.ReportGranularity) domain.SellerReportQuery {
		in := reportQuery(accountID)
		start, _ := time.ParseInLocation("2006-01-02", from, domain.Argentina)
		end, _ := time.ParseInLocation("2006-01-02", to, domain.Argentina)
		in.StartAt, in.EndAt = start.UTC(), end.AddDate(0, 0, 1).UTC()
		in.FromDate, in.ToDate, in.Granularity = from, to, granularity
		return in
	}

	weekly := buildReport(t, db, domain.Tenant{AccountID: accountID},
		period("2026-10-01", "2026-10-20", domain.ReportGranularityWeek)).Timeline
	wantDates := []string{"2026-10-01", "2026-10-05", "2026-10-12", "2026-10-19"}
	if len(weekly) != len(wantDates) {
		t.Fatalf("weekly trend = %+v, want %d Monday buckets", weekly, len(wantDates))
	}
	for index, want := range wantDates {
		if weekly[index].Date != want {
			t.Fatalf("weekly trend = %+v, want buckets dated %v", weekly, wantDates)
		}
	}
	if weekly[0].RFQs != 1 || weekly[1].Sent != 1 || weekly[1].Accepted != 1 ||
		weekly[1].SentAccepted != 1 || weekly[2].Sent != 1 || weekly[2].SentAccepted != 0 {
		t.Fatalf("weekly trend = %+v, want the order, sends and acceptance in their weeks", weekly)
	}
	if weekly[1].ConversionRate == nil || weekly[1].ConversionRate.StringFixed(2) != "1.00" ||
		weekly[2].ConversionRate == nil || weekly[2].ConversionRate.StringFixed(2) != "0.00" ||
		weekly[3].ConversionRate != nil {
		t.Fatalf("weekly conversion = %+v, want 1, 0 and none for a week with no sends", weekly)
	}

	monthly := buildReport(t, db, domain.Tenant{AccountID: accountID},
		period("2026-09-15", "2026-10-20", domain.ReportGranularityMonth)).Timeline
	if len(monthly) != 2 || monthly[0].Date != "2026-09-15" || monthly[1].Date != "2026-10-01" {
		t.Fatalf("monthly trend = %+v, want September from the 15th and October", monthly)
	}
	if monthly[0].RFQs != 2 || monthly[1].RFQs != 1 || monthly[1].Sent != 2 ||
		monthly[1].SentAccepted != 1 {
		t.Fatalf("monthly trend = %+v, want each order in its month and both sends in October", monthly)
	}
}

func TestSellerReportRepositoryMeasuresTheAssistantAndFollowUp(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Reportes asistente")
	branchID := reportBranch(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cal")
	quoteID, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	inPeriod := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM handler_decision WHERE quote_version_id = $1`,
			versionID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_item WHERE version_id = $1 AND
		   requested_description = 'clavos'`, versionID)
	})
	reportExec(t, db, "rfq", `UPDATE rfq SET received_at = $2
	   WHERE id = (SELECT rfq_id FROM quote WHERE id = $1)`, quoteID, inPeriod)
	reportExec(t, db, "follow-up", `UPDATE quote SET needs_followup = TRUE, followup_flagged_at = $2
	   WHERE id = $1`, quoteID, inPeriod)
	reportExec(t, db, "unmatched line", `INSERT INTO quote_item
	   (account_id, version_id, requested_description, quantity, match_status)
	   VALUES ($1, $2, 'clavos', 3, 'NO_MATCH')`, accountID, versionID)
	for _, seed := range []struct {
		decision *string
		at       time.Time
	}{
		{strPtr("APPROVED_AS_IS"), inPeriod},
		{strPtr("APPROVED_AS_IS"), inPeriod},
		{strPtr("REJECTED"), inPeriod},
		{nil, inPeriod},
		{strPtr("APPROVED_AS_IS"), time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)},
	} {
		reportExec(t, db, "decision", `INSERT INTO handler_decision
		   (account_id, quote_version_id, seller_decision, created_at)
		   VALUES ($1, $2, $3::handler_seller_decision, $4)`, accountID, versionID, seed.decision, seed.at)
	}
	reportExec(t, db, "sent", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'QUOTED', 'SENT', $3)`, accountID, quoteID, inPeriod.Add(-2*time.Hour))
	reportExec(t, db, "accepted", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, accountID, quoteID, inPeriod)
	// A send after the acceptance is not where the wait started.
	reportExec(t, db, "later send", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'CHANGE_REQUESTED', 'SENT', $3)`, accountID, quoteID, inPeriod.Add(time.Hour))
	// An acceptance never sent before it measures no wait at all.
	unsent, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	reportExec(t, db, "unsent acceptance", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'QUOTED', 'ACCEPTED', $3)`, accountID, unsent, inPeriod)
	reportExec(t, db, "send after it", `INSERT INTO quote_status_change
	   (account_id, quote_id, previous_status, new_status, changed_at)
	   VALUES ($1, $2, 'ACCEPTED', 'SENT', $3)`, accountID, unsent, inPeriod.Add(time.Hour))

	report := buildReport(t, db, domain.Tenant{AccountID: accountID}, reportQuery(accountID))
	assistant := report.Assistant
	if assistant.Proposals != 4 || assistant.ApprovedAsIs != 2 || assistant.Rejected != 1 ||
		assistant.Pending != 1 {
		t.Fatalf("assistant = %+v, want four proposals in the period, one still undecided", assistant)
	}
	if assistant.ApprovalRate == nil || assistant.ApprovalRate.StringFixed(2) != "0.50" {
		t.Fatalf("approval rate = %v, want 0.50 with the undecided one in the denominator",
			assistant.ApprovalRate)
	}
	if report.KPIs.FollowUpPending != 1 {
		t.Fatalf("follow-up pending = %d, want the flagged quote", report.KPIs.FollowUpPending)
	}
	wait := report.KPIs.TimeToAcceptance
	if wait.Sample != 1 || wait.AverageSeconds == nil || *wait.AverageSeconds != 7200 {
		t.Fatalf("time to acceptance = %+v, want two hours from the first send", wait)
	}
	if report.AIMatch.VersionLines != 2 || report.AIMatch.VersionUnmatched != 1 {
		t.Fatalf("version match = %d of %d unmatched, want 1 of 2",
			report.AIMatch.VersionUnmatched, report.AIMatch.VersionLines)
	}
}

func reportBranch(t *testing.T, db *DB, accountID uuid.UUID) uuid.UUID {
	t.Helper()
	var branchID uuid.UUID
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM branch WHERE account_id = $1`, accountID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	return branchID
}

func reportExec(t *testing.T, db *DB, what, query string, args ...any) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("seed %s: %v", what, err)
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
