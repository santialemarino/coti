package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	reportDefaultDays = 30
	reportDateLayout  = "2006-01-02"
)

type sellerReportStore interface {
	Build(context.Context, repository.Querier, domain.SellerReportQuery) (*domain.SellerReport, error)
}

// SellerReportSettings carries the bounds and thresholds a report query reads.
type SellerReportSettings struct {
	MaxRangeDays   int
	HighConfidence decimal.Decimal
}

// SellerReportService turns a requested period into a scoped seller dashboard read.
type SellerReportService struct {
	db      tenantTxRunner
	reports sellerReportStore
	now     func() time.Time
	cfg     SellerReportSettings
}

// NewSellerReportService builds a SellerReportService.
func NewSellerReportService(db tenantTxRunner, reports sellerReportStore,
	cfg SellerReportSettings,
) *SellerReportService {
	return &SellerReportService{db: db, reports: reports, now: time.Now, cfg: cfg}
}

// Get returns the report for the requested inclusive dates, defaulting to the last 30 days.
func (s *SellerReportService) Get(ctx context.Context, tenant domain.Tenant, fromRaw,
	toRaw string,
) (*domain.SellerReport, error) {
	from, to, err := s.resolveRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	start := from.UTC()
	end := to.AddDate(0, 0, 1).UTC()
	sellerID := sellerParam(tenant)
	query := domain.SellerReportQuery{
		AccountID:      tenant.AccountID,
		BranchFilter:   tenant.BranchFilter(),
		SellerID:       sellerID,
		StartAt:        start,
		EndAt:          end,
		FromDate:       from.Format(reportDateLayout),
		ToDate:         to.Format(reportDateLayout),
		TimeZone:       domain.Argentina.String(),
		HighConfidence: s.cfg.HighConfidence,
	}

	var report *domain.SellerReport
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var readErr error
		report, readErr = s.reports.Build(ctx, q, query)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	report.Range = domain.SellerReportRange{
		From: query.FromDate,
		To:   query.ToDate,
	}
	report.Scope = domain.SellerReportScope{
		MultiBranch: tenant.IsAdmin() && !tenant.HasBranch(),
	}
	if tenant.HasBranch() {
		branchID := tenant.BranchID
		report.Scope.BranchID = &branchID
	}
	return report, nil
}

func (s *SellerReportService) resolveRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	if (fromRaw == "") != (toRaw == "") {
		return time.Time{}, time.Time{},
			fmt.Errorf("%w: from and to must be provided together", domain.ErrInvalidInput)
	}
	if fromRaw == "" {
		today := s.now().In(domain.Argentina)
		to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, domain.Argentina)
		return to.AddDate(0, 0, -(reportDefaultDays - 1)), to, nil
	}
	from, err := time.ParseInLocation(reportDateLayout, fromRaw, domain.Argentina)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from must be YYYY-MM-DD",
			domain.ErrInvalidInput)
	}
	to, err := time.ParseInLocation(reportDateLayout, toRaw, domain.Argentina)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: to must be YYYY-MM-DD",
			domain.ErrInvalidInput)
	}
	if from.After(to) {
		return time.Time{}, time.Time{},
			fmt.Errorf("%w: from must not be after to", domain.ErrInvalidInput)
	}
	if int(to.Sub(from).Hours()/24)+1 > s.cfg.MaxRangeDays {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"%w: the period must not exceed %d days", domain.ErrInvalidInput, s.cfg.MaxRangeDays)
	}
	return from, to, nil
}

func sellerParam(tenant domain.Tenant) *uuid.UUID {
	if tenant.IsAdmin() {
		return nil
	}
	return &tenant.UserID
}
