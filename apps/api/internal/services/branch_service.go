package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// branchReader is the branch listing surface. Defined here, in the consumer, so a test can
// fake it without a database.
type branchReader interface {
	ListForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, isAdmin bool) ([]domain.Branch, error)
}

// BranchService owns the branch list a caller may operate on.
type BranchService struct {
	db       tenantTxRunner
	branches branchReader
}

// NewBranchService builds a BranchService.
func NewBranchService(db tenantTxRunner, branches branchReader) *BranchService {
	return &BranchService{db: db, branches: branches}
}

// ListBranches returns the branches the caller may switch between: every active branch of the
// account for an admin, and the assigned ones for a seller.
func (s *BranchService) ListBranches(ctx context.Context, tenant domain.Tenant) ([]domain.Branch, error) {
	var branches []domain.Branch
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		branches, listErr = s.branches.ListForUser(ctx, q, tenant.AccountID, tenant.UserID,
			tenant.IsAdmin())
		return listErr
	})
	if err != nil {
		return nil, err
	}
	return branches, nil
}
