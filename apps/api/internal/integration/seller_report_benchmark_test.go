//go:build integration

package integration

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestSellerReportSyntheticBenchmark(t *testing.T) {
	if os.Getenv("COTI_REPORT_BENCHMARK") != "1" {
		t.Skip("opt-in synthetic report benchmark")
	}
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Report benchmark")
	channel := e.seedIntakeChannel(t, account, branch)
	product := e.seedPricedProduct(t, account, branch, "Benchmark cement", "1000", nil)
	ctx := context.Background()
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM quote_status_change WHERE account_id=$1`,
			`DELETE FROM quote_item WHERE account_id=$1`,
			`UPDATE quote SET current_version_id=NULL WHERE account_id=$1`,
			`DELETE FROM quote_version WHERE account_id=$1`,
			`DELETE FROM quote WHERE account_id=$1`,
			`DELETE FROM rfq WHERE account_id=$1`,
		} {
			e.mustCleanup(t, query, account)
		}
	})
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO rfq (account_id,branch_id,channel_id,status,received_at)
		 SELECT $1,$2,$3,'GENERATED',
		 CASE WHEN n<=5000 THEN '2026-11-01 03:00:00+00'::timestamptz ELSE '2026-10-02 03:00:00+00'::timestamptz END
		 + ((n-1)%30)*interval '1 day' FROM generate_series(1,10000) n`, []any{account, branch, channel}},
		{`ANALYZE rfq`, nil},
		{`WITH source AS MATERIALIZED (
		 SELECT account_id,branch_id,id AS rfq_id,received_at,
		 gen_random_uuid() AS quote_id,gen_random_uuid() AS version_id,row_number() OVER (ORDER BY id) AS number
		 FROM rfq WHERE account_id=$1
		 ), inserted AS (
		 INSERT INTO quote (id,account_id,branch_id,rfq_id,number,current_status,current_version_id,created_at)
		 SELECT quote_id,account_id,branch_id,rfq_id,number,'ACCEPTED',version_id,received_at FROM source
		 RETURNING id
		 )
		 INSERT INTO quote_version (id,account_id,quote_id,version_number,total,currency,is_immutable,frozen_at,created_at)
		 SELECT source.version_id,source.account_id,source.quote_id,1,1000,'ARS',TRUE,
		 source.received_at+interval '1 hour',source.received_at
		 FROM source JOIN inserted ON inserted.id=source.quote_id`, []any{account}},
		{`ANALYZE quote, quote_version`, nil},
		{`INSERT INTO quote_item (account_id,version_id,product_id,requested_description,quantity,unit_price_snapshot,subtotal,match_status)
		 SELECT account_id,id,$2,'Benchmark cement',1,1000,1000,'MATCHED' FROM quote_version WHERE account_id=$1`, []any{account, product}},
		{`INSERT INTO quote_status_change (account_id,quote_id,previous_status,new_status,changed_at)
		 SELECT account_id,id,'QUOTED','SENT',created_at+interval '1 hour' FROM quote WHERE account_id=$1`, []any{account}},
		{`INSERT INTO quote_status_change (account_id,quote_id,previous_status,new_status,changed_at)
		 SELECT account_id,id,'SENT','ACCEPTED',created_at+interval '2 hours' FROM quote WHERE account_id=$1`, []any{account}},
		{`ANALYZE rfq, quote, quote_version, quote_item, quote_status_change`, nil},
	} {
		start := time.Now()
		if _, err := e.db.CrossAccount().Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
		t.Logf("fixture step completed in %s", time.Since(start))
	}
	store := &reportBenchmarkStore{repository: repository.NewSellerReportRepository()}
	service := services.NewSellerReportService(e.db, store, nil,
		services.SellerReportSettings{MaxRangeDays: 366, HighConfidence: decimal.RequireFromString("0.8")})
	tenant := domain.Tenant{AccountID: account, BranchID: branch, Role: domain.UserRoleAdmin}
	var elapsed []time.Duration
	for range 5 {
		start := time.Now()
		report, err := service.Get(ctx, tenant, "2026-11-01", "2026-11-30", domain.SellerReportFilterInput{})
		if err != nil {
			t.Fatal(err)
		}
		elapsed = append(elapsed, time.Since(start))
		previous := report.Comparison.Metrics["rfqs_received"].Previous
		if report.KPIs.RFQsReceived != 5000 || previous == nil || !previous.Equal(decimal.NewFromInt(5000)) {
			t.Fatalf("current=%d previous=%v, want 5000 in each period", report.KPIs.RFQsReceived, previous)
		}
	}
	sorted := slices.Clone(elapsed)
	slices.Sort(sorted)
	t.Logf("10000 RFQs, quotes, versions and items; 20000 status events; full current report plus previous KPIs: runs=%v p50=%s max=%s", elapsed, sorted[2], sorted[4])
	t.Logf("current report runs=%v; previous KPI runs=%v", store.current, store.previous)
}

type reportBenchmarkStore struct {
	repository        *repository.SellerReportRepository
	current, previous []time.Duration
}

func (s *reportBenchmarkStore) Build(ctx context.Context, q repository.Querier, in domain.SellerReportQuery) (*domain.SellerReport, error) {
	start := time.Now()
	report, err := s.repository.Build(ctx, q, in)
	s.current = append(s.current, time.Since(start))
	return report, err
}

func (s *reportBenchmarkStore) ReadKPIs(ctx context.Context, q repository.Querier, in domain.SellerReportQuery) (*domain.SellerReportKPIs, error) {
	start := time.Now()
	kpis, err := s.repository.ReadKPIs(ctx, q, in)
	s.previous = append(s.previous, time.Since(start))
	return kpis, err
}
