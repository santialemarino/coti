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

type sellerReportTestRenderer struct {
	report      domain.SellerReport
	generatedAt time.Time
	err         error
}

func (f *sellerReportTestRenderer) Render(
	report domain.SellerReport, generatedAt time.Time,
) ([]byte, error) {
	f.report = report
	f.generatedAt = generatedAt
	if f.err != nil {
		return nil, f.err
	}
	return []byte("%PDF-1.4 report"), nil
}

func newSellerReportTestService(
	t *testing.T, tenant domain.Tenant,
) (*SellerReportService, *sellerReportTestDB, *sellerReportTestStore) {
	t.Helper()
	db := &sellerReportTestDB{}
	store := &sellerReportTestStore{}
	service := NewSellerReportService(db, store, &sellerReportTestRenderer{}, SellerReportSettings{
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

	report, err := service.Get(context.Background(), tenant, "", "", domain.SellerReportFilterInput{})
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
			if _, err := service.Get(context.Background(), tenant, tt.from, tt.to, domain.SellerReportFilterInput{}); !errors.Is(
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

	report, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02", domain.SellerReportFilterInput{})
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

func TestSellerReportSpansASellersBranchesWhenNoneIsSelected(t *testing.T) {
	branches := []uuid.UUID{uuid.New(), uuid.New()}
	cases := []struct {
		name    string
		allowed []uuid.UUID
		want    bool
	}{
		{"two branches", branches, true},
		{"one branch", branches[:1], false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(),
				Role: domain.UserRoleSeller, AllowedBranchIDs: tc.allowed}
			service, _, store := newSellerReportTestService(t, tenant)

			report, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
				domain.SellerReportFilterInput{})
			if err != nil {
				t.Fatal(err)
			}
			if store.query.MultiBranch != tc.want || report.Scope.MultiBranch != tc.want {
				t.Fatalf("query multi-branch = %v, scope = %+v, want %v",
					store.query.MultiBranch, report.Scope, tc.want)
			}
			if report.Scope.BranchID != nil {
				t.Fatalf("scope = %+v, want no selected branch", report.Scope)
			}
		})
	}
}

func TestSellerReportResolvesFiltersForAnAdministrator(t *testing.T) {
	sellerID, clientID := uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(),
		Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)

	report, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{
			Statuses: []string{"sent,ACCEPTED", "SENT"},
			Channel:  " email ",
			SellerID: sellerID.String(),
			ClientID: clientID.String(),
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.query.Statuses) != 2 || store.query.Statuses[0] != domain.QuoteStatusSent ||
		store.query.Statuses[1] != domain.QuoteStatusAccepted {
		t.Fatalf("statuses = %v, want SENT and ACCEPTED once", store.query.Statuses)
	}
	if store.query.Channel == nil || *store.query.Channel != domain.ChannelTypeEmail {
		t.Fatalf("channel = %v, want EMAIL", store.query.Channel)
	}
	if store.query.SellerID != nil {
		t.Fatalf("reach seller = %v, want none for an administrator", store.query.SellerID)
	}
	if store.query.SellerFilter == nil || *store.query.SellerFilter != sellerID {
		t.Fatalf("seller filter = %v, want %s", store.query.SellerFilter, sellerID)
	}
	if store.query.ClientFilter == nil || *store.query.ClientFilter != clientID {
		t.Fatalf("client filter = %v, want %s", store.query.ClientFilter, clientID)
	}
	if len(report.Filters.Statuses) != 2 || report.Filters.Channel == nil {
		t.Fatalf("response filters = %+v, want the applied narrowing", report.Filters)
	}
}

func TestSellerReportNarrowsBranchesToTheCallersReach(t *testing.T) {
	reachable, other, unreachable := uuid.New(), uuid.New(), uuid.New()
	cases := []struct {
		name          string
		tenant        domain.Tenant
		requested     string
		wantBranches  []uuid.UUID
		wantMulti     bool
		wantRequested int
	}{
		{
			name:          "an admin picks two branches of the account",
			tenant:        domain.Tenant{Role: domain.UserRoleAdmin},
			requested:     reachable.String() + "," + other.String(),
			wantBranches:  []uuid.UUID{reachable, other},
			wantMulti:     true,
			wantRequested: 2,
		},
		{
			name:          "an admin picks one branch",
			tenant:        domain.Tenant{Role: domain.UserRoleAdmin},
			requested:     reachable.String(),
			wantBranches:  []uuid.UUID{reachable},
			wantRequested: 1,
		},
		{
			name: "a seller cannot reach past their assignments",
			tenant: domain.Tenant{Role: domain.UserRoleSeller,
				AllowedBranchIDs: []uuid.UUID{reachable, other}},
			requested:     reachable.String() + "," + unreachable.String(),
			wantBranches:  []uuid.UUID{reachable},
			wantRequested: 1,
		},
		{
			name: "only unreachable branches read nothing",
			tenant: domain.Tenant{Role: domain.UserRoleSeller,
				AllowedBranchIDs: []uuid.UUID{reachable}},
			requested:    unreachable.String(),
			wantBranches: []uuid.UUID{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenant := tc.tenant
			tenant.AccountID, tenant.UserID = uuid.New(), uuid.New()
			service, _, store := newSellerReportTestService(t, tenant)

			if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
				domain.SellerReportFilterInput{BranchIDs: []string{tc.requested}}); err != nil {
				t.Fatal(err)
			}
			got := store.query.BranchFilter
			if got == nil || len(got) != len(tc.wantBranches) {
				t.Fatalf("branch filter = %v, want %v", got, tc.wantBranches)
			}
			for index := range got {
				if got[index] != tc.wantBranches[index] {
					t.Fatalf("branch filter = %v, want %v", got, tc.wantBranches)
				}
			}
			if store.query.MultiBranch != tc.wantMulti {
				t.Fatalf("multi-branch = %v, want %v", store.query.MultiBranch, tc.wantMulti)
			}
			if len(store.query.RequestedBranches) != tc.wantRequested {
				t.Fatalf("requested branches = %v, want %d", store.query.RequestedBranches,
					tc.wantRequested)
			}
		})
	}
}

func TestSellerReportParsesTagFilters(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), Role: domain.UserRoleSeller}
	first, second := uuid.New(), uuid.New()
	service, _, store := newSellerReportTestService(t, tenant)

	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{TagIDs: []string{
			first.String() + ", " + second.String(), first.String(),
		}}); err != nil {
		t.Fatal(err)
	}
	if len(store.query.TagFilter) != 2 || store.query.TagFilter[0] != first ||
		store.query.TagFilter[1] != second {
		t.Fatalf("tag filter = %v, want both tags once", store.query.TagFilter)
	}

	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{TagIDs: []string{"not-a-tag"}}); !errors.Is(
		err, domain.ErrInvalidInput) {
		t.Fatalf("Get() = %v, want invalid input for a malformed tag", err)
	}
	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{}); err != nil || store.query.TagFilter != nil {
		t.Fatalf("tag filter = %v (%v), want none when nothing is selected", store.query.TagFilter, err)
	}
}

func TestSellerReportIgnoresTheSellerFilterForASeller(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(),
		Role: domain.UserRoleSeller}
	service, _, store := newSellerReportTestService(t, tenant)

	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{SellerID: uuid.New().String()}); err != nil {
		t.Fatal(err)
	}
	if store.query.SellerFilter != nil {
		t.Fatalf("seller filter = %v, want the caller's own reach only", store.query.SellerFilter)
	}
	if store.query.SellerID == nil || *store.query.SellerID != tenant.UserID {
		t.Fatalf("reach seller = %v, want %s", store.query.SellerID, tenant.UserID)
	}
}

func TestSellerReportRejectsInvalidFilters(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)

	tests := []struct {
		name  string
		input domain.SellerReportFilterInput
	}{
		{"unknown status", domain.SellerReportFilterInput{Statuses: []string{"LOST"}}},
		{"unknown channel", domain.SellerReportFilterInput{Channel: "carrier-pigeon"}},
		{"bad seller", domain.SellerReportFilterInput{SellerID: "not-a-uuid"}},
		{"bad client", domain.SellerReportFilterInput{ClientID: "not-a-uuid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02",
				tt.input); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("Get() = %v, want ErrInvalidInput", err)
			}
		})
	}
	if store.query.StartAt != (time.Time{}) {
		t.Fatal("the store ran for an invalid filter")
	}
}

func TestSellerReportRenderPDFReusesTheReportScope(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)
	store.report = &domain.SellerReport{KPIs: domain.SellerReportKPIs{
		AcceptedAmount: decimal.RequireFromString("1250.50"),
	}}
	renderer := service.renderer.(*sellerReportTestRenderer)

	content, period, err := service.RenderPDF(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{})
	if err != nil {
		t.Fatal(err)
	}
	if period.From != "2026-10-01" || period.To != "2026-10-02" {
		t.Fatalf("period = %+v, want the requested dates", period)
	}
	if string(content) != "%PDF-1.4 report" {
		t.Fatalf("content = %q", content)
	}
	if renderer.report.Range.From != "2026-10-01" || renderer.report.Range.To != "2026-10-02" {
		t.Fatalf("rendered range = %+v", renderer.report.Range)
	}
	if renderer.report.KPIs.AcceptedAmount.StringFixed(2) != "1250.50" {
		t.Fatalf("rendered KPIs = %+v", renderer.report.KPIs)
	}
	wantTime := time.Date(2026, time.October, 25, 15, 0, 0, 0, domain.Argentina)
	if !renderer.generatedAt.Equal(wantTime) {
		t.Fatalf("generated at = %s, want %s", renderer.generatedAt, wantTime)
	}
}

func TestSellerReportRenderPDFPropagatesRendererError(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, _ := newSellerReportTestService(t, tenant)
	want := errors.New("render failed")
	service.renderer.(*sellerReportTestRenderer).err = want

	if _, _, err := service.RenderPDF(context.Background(), tenant, "2026-10-01", "2026-10-02",
		domain.SellerReportFilterInput{}); !errors.Is(
		err, want,
	) {
		t.Fatalf("RenderPDF() = %v, want %v", err, want)
	}
}

func TestSellerReportPropagatesStoreError(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), Role: domain.UserRoleAdmin}
	service, _, store := newSellerReportTestService(t, tenant)
	want := errors.New("read failed")
	store.err = want

	if _, err := service.Get(context.Background(), tenant, "2026-10-01", "2026-10-02", domain.SellerReportFilterInput{}); !errors.Is(
		err, want,
	) {
		t.Fatalf("Get() = %v, want %v", err, want)
	}
}
