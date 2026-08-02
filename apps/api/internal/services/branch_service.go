package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type branchListRepository interface {
	ListForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, isAdmin bool) ([]domain.Branch, error)
}

// BranchService lists the operating locations available to the caller.
type BranchService struct {
	db       tenantScoper
	branches branchListRepository
}

// NewBranchService builds a BranchService.
func NewBranchService(db tenantScoper, branches branchListRepository) *BranchService {
	return &BranchService{db: db, branches: branches}
}

// List returns every active branch the caller may select.
func (s *BranchService) List(ctx context.Context, tenant domain.Tenant) ([]domain.Branch, error) {
	var branches []domain.Branch
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		branches, listErr = s.branches.ListForUser(ctx, q, tenant.AccountID, tenant.UserID, tenant.IsAdmin())
		return listErr
	})
	return branches, err
}
