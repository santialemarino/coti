package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type sellerReportRepository interface {
	Get(
		ctx context.Context,
		q repository.Querier,
		accountID, sellerID uuid.UUID,
		branchIDs []uuid.UUID,
		filter domain.SellerReportFilter,
	) (*domain.SellerReport, error)
}

// SellerReportService builds reports from one seller's assigned RFQs.
type SellerReportService struct {
	db      tenantScoper
	reports sellerReportRepository
}

// NewSellerReportService builds a SellerReportService.
func NewSellerReportService(db tenantScoper, reports sellerReportRepository) *SellerReportService {
	return &SellerReportService{db: db, reports: reports}
}

// Get returns metrics for the authenticated seller within their reachable branches.
func (s *SellerReportService) Get(
	ctx context.Context, tenant domain.Tenant, filter domain.SellerReportFilter,
) (*domain.SellerReport, error) {
	if filter.DateFrom != nil && filter.DateTo != nil && filter.DateTo.Before(*filter.DateFrom) {
		return nil, fmt.Errorf("%w: date_to must be on or after date_from", domain.ErrInvalidInput)
	}

	var report *domain.SellerReport
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		report, err = s.reports.Get(
			ctx, q, tenant.AccountID, tenant.UserID, tenant.BranchFilter(), filter,
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}
