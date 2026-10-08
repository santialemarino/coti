package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type sellerReportTestDB struct {
	calls int
}

func (db *sellerReportTestDB) InTenantTx(
	_ context.Context, _ domain.Tenant, fn func(repository.Querier) error,
) error {
	db.calls++
	return fn(nil)
}

func (db *sellerReportTestDB) CrossAccount() repository.Querier {
	return nil
}

type sellerReportTestRepository struct {
	accountID uuid.UUID
	sellerID  uuid.UUID
	branchIDs []uuid.UUID
	filter    domain.SellerReportFilter
	report    *domain.SellerReport
}

func (repo *sellerReportTestRepository) Get(
	_ context.Context, _ repository.Querier, accountID, sellerID uuid.UUID,
	branchIDs []uuid.UUID, filter domain.SellerReportFilter,
) (*domain.SellerReport, error) {
	repo.accountID = accountID
	repo.sellerID = sellerID
	repo.branchIDs = branchIDs
	repo.filter = filter
	return repo.report, nil
}

func TestSellerReportService_GetScopesTheReportToTheAuthenticatedSeller(t *testing.T) {
	t.Parallel()
	db := &sellerReportTestDB{}
	repo := &sellerReportTestRepository{report: &domain.SellerReport{}}
	service := NewSellerReportService(db, repo)
	tenant := domain.Tenant{
		AccountID: uuid.New(),
		UserID:    uuid.New(),
		Role:      domain.UserRoleSeller,
		AllowedBranchIDs: []uuid.UUID{
			uuid.New(),
			uuid.New(),
		},
	}
	from := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)

	_, err := service.Get(context.Background(), tenant, domain.SellerReportFilter{
		DateFrom: &from,
		DateTo:   &to,
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if db.calls != 1 {
		t.Fatalf("tenant transactions = %d, want 1", db.calls)
	}
	if repo.accountID != tenant.AccountID || repo.sellerID != tenant.UserID {
		t.Fatalf("report scope = (%s, %s), want authenticated account and seller (%s, %s)",
			repo.accountID, repo.sellerID, tenant.AccountID, tenant.UserID)
	}
	if len(repo.branchIDs) != len(tenant.AllowedBranchIDs) {
		t.Fatalf("branch scope = %v, want %v", repo.branchIDs, tenant.AllowedBranchIDs)
	}
	if repo.filter.DateFrom != &from || repo.filter.DateTo != &to {
		t.Fatal("date filter was not forwarded to the repository")
	}
}

func TestSellerReportService_GetRejectsInvertedDateRange(t *testing.T) {
	t.Parallel()
	db := &sellerReportTestDB{}
	service := NewSellerReportService(db, &sellerReportTestRepository{})
	from := time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)

	_, err := service.Get(context.Background(), domain.Tenant{}, domain.SellerReportFilter{
		DateFrom: &from,
		DateTo:   &to,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Get() error = %v, want ErrInvalidInput", err)
	}
	if db.calls != 0 {
		t.Fatalf("tenant transactions = %d, want no transaction", db.calls)
	}
}
