package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type sellerReportTestDB struct {
	tenant domain.Tenant
	calls  int
}

func (f *sellerReportTestDB) InTenantTx(
	_ context.Context, tenant domain.Tenant, fn func(repository.Querier) error,
) error {
	f.tenant = tenant
	f.calls++
	return fn(nil)
}

type sellerReportTestStore struct {
	query  domain.SellerReportQuery
	report *domain.SellerReport
	err    error
}

func (f *sellerReportTestStore) Build(
	_ context.Context, _ repository.Querier, query domain.SellerReportQuery,
) (*domain.SellerReport, error) {
	f.query = query
	if f.err != nil {
		return nil, f.err
	}
	if f.report == nil {
		return &domain.SellerReport{}, nil
	}
	return f.report, nil
}

func newSellerReportTestService(
	t *testing.T, tenant domain.Tenant,
) (*SellerReportService, *sellerReportTestDB, *sellerReportTestStore) {
	t.Helper()
	db := &sellerReportTestDB{}
	store := &sellerReportTestStore{}
	service := NewSellerReportService(db, store, SellerReportSettings{
		MaxRangeDays:   366,
		HighConfidence: decimal.NewFromFloat(0.8),
	})
	service.now = func() time.Time {
		return time.Date(2026, time.October, 25, 15, 0, 0, 0, domain.Argentina)
	}
	return service, db, store
}

func TestSellerReportDefaultsToLastThirtyArgentineDays(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(),
		Role: domain.UserRoleAdmin}
	service, db, store := newSellerReportTestService(t, tenant)

	report, err := service.Get(context.Background(), tenant, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if db.calls != 1 {
		t.Fatalf("transactions = %d, want 1", db.calls)
	}
	if store.query.FromDate != "2026-09-26" || store.query.ToDate != "2026-10-25" {
		t.Fatalf("range = %s..%s, want 2026-09-26..2026-10-25",
			store.query.FromDate, store.query.ToDate)
	}
	if got := store.query.StartAt.UTC(); !got.Equal(time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %s, want midnight in Argentina", got)
	}
	if got := store.query.EndAt.UTC(); !got.Equal(time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %s, want the day after the last day", got)
	}
	if report.Range.From != "2026-09-26" || report.Range.To != "2026-10-25" {
		t.Fatalf("response range = %+v", report.Range)
	}
	if !report.Scope.MultiBranch || report.Scope.BranchID != nil {
		t.Fatalf("scope = %+v, want every branch", report.Scope)
	}
}

func TestSellerReportRejectsInvalidPeriods(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)

	tests := []struct {
		name string
		from string
		to   string
	}{
		{"missing end", "2026-10-01", ""},
		{"missing start", "", "2026-10-01"},
		{"bad start", "10/01/2026", "2026-10-02"},
		{"bad end", "2026-10-01", "2026-13-02"},
		{"reversed", "2026-10-02", "2026-10-01"},
		{"too wide", "2025-01-01", "2026-10-25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Get(context.Background(), tenant, tt.from, tt.to); !errors.Is(
				err, domain.ErrInvalidInput,
			) {
				t.Fatalf("Get(%q, %q) = %v, want ErrInvalidInput", tt.from, tt.to, err)
			}
		})
	}
	if store.query.StartAt != (time.Time{}) {
		t.Fatal("the store ran for an invalid period")
	}
}

func TestSellerReportScopesToSelectedBranchAndSeller(t *testing.T) {
	branchID := uuid.New()
	userID := uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: userID,
		Role: domain.UserRoleSeller, BranchID: branchID}
	service, db, store := newSellerReportTestService(t, tenant)
	store.report = &domain.SellerReport{KPIs: domain.SellerReportKPIs{
		AcceptedAmount: decimal.RequireFromString("1250.50"),
	}}

	report, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if db.tenant.AccountID != tenant.AccountID || db.tenant.BranchID != tenant.BranchID ||
		db.tenant.UserID != tenant.UserID {
		t.Fatalf("tenant = %+v, want the caller's scope", db.tenant)
	}
	if len(store.query.BranchFilter) != 1 || store.query.BranchFilter[0] != branchID {
		t.Fatalf("branch filter = %v, want [%s]", store.query.BranchFilter, branchID)
	}
	if store.query.SellerID == nil || *store.query.SellerID != userID {
		t.Fatalf("seller = %v, want %s", store.query.SellerID, userID)
	}
	if store.query.FromDate != "2026-10-01" || store.query.ToDate != "2026-10-02" {
		t.Fatalf("range = %s..%s", store.query.FromDate, store.query.ToDate)
	}
	if report.Scope.MultiBranch || report.Scope.BranchID == nil ||
		*report.Scope.BranchID != branchID {
		t.Fatalf("scope = %+v, want the selected branch", report.Scope)
	}
	if report.KPIs.AcceptedAmount.StringFixed(2) != "1250.50" {
		t.Fatalf("stores' KPIs were lost: %+v", report.KPIs)
	}
}

func TestSellerReportPropagatesStoreError(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)
	want := errors.New("read failed")
	store.err = want

	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02"); !errors.Is(
		err, want,
	) {
		t.Fatalf("Get() = %v, want %v", err, want)
	}
}
