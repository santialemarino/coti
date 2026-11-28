package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestReportChange(t *testing.T) {
	for _, tc := range []struct {
		current, previous, delta, percent string
		points                            bool
	}{
		{"120", "100", "20", "20", false}, {"80", "100", "-20", "-20", false},
		{"12", "0", "12", "", false}, {"0", "0", "0", "0", false}, {"0.3", "0.2", "10", "", true},
	} {
		current, previous := decimal.RequireFromString(tc.current), decimal.RequireFromString(tc.previous)
		got := reportChange(&current, &previous, tc.points)
		if got.Delta == nil || got.Delta.String() != tc.delta {
			t.Fatalf("delta=%v want %s", got.Delta, tc.delta)
		}
		if tc.percent == "" {
			if got.Percentage != nil {
				t.Fatal("undefined percentage")
			}
		} else if got.Percentage == nil || got.Percentage.String() != tc.percent {
			t.Fatalf("percent=%v", got.Percentage)
		}
	}
	if got := reportChange(nil, nil, false); got.Delta != nil || got.Previous != nil {
		t.Fatal("missing measurement became zero")
	}
}

func TestPreviousReportCalendarPeriod(t *testing.T) {
	for _, tc := range []struct{ from, to, previousFrom, previousTo string }{
		{"2026-11-10", "2026-11-16", "2026-11-03", "2026-11-09"},
		{"2026-01-01", "2026-01-02", "2025-12-30", "2025-12-31"},
		{"2024-03-01", "2024-03-02", "2024-02-28", "2024-02-29"},
		{"2026-03-01", "2026-03-02", "2026-02-27", "2026-02-28"},
	} {
		from, _ := time.ParseInLocation(reportDateLayout, tc.from, domain.Argentina)
		to, _ := time.ParseInLocation(reportDateLayout, tc.to, domain.Argentina)
		query := domain.SellerReportQuery{AccountID: uuid.New(), StartAt: from.UTC(), EndAt: to.AddDate(0, 0, 1).UTC()}
		previous := previousReportQuery(query)
		if previous.FromDate != tc.previousFrom || previous.ToDate != tc.previousTo || !previous.EndAt.Equal(query.StartAt) || previous.AccountID != query.AccountID {
			t.Fatalf("previous=%+v", previous)
		}
	}
}

func TestReportComparisonPreservesFilters(t *testing.T) {
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New(), Role: domain.UserRoleSeller}
	service, _, store := newSellerReportTestService(t, tenant)
	report, err := service.Get(context.Background(), tenant, "2026-11-10", "2026-11-16", domain.SellerReportFilterInput{Channel: "EMAIL", Statuses: []string{"SENT"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparison.Range.From != "2026-11-03" || store.previousQuery.AccountID != store.query.AccountID || *store.previousQuery.SellerID != tenant.UserID || store.previousQuery.BranchFilter[0] != tenant.BranchID || *store.previousQuery.Channel != domain.ChannelTypeEmail || store.previousQuery.Statuses[0] != domain.QuoteStatusSent {
		t.Fatalf("comparison lost scope: %+v", store.previousQuery)
	}
}
