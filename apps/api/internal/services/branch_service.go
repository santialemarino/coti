package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// branchReader is the branch listing surface. Defined here, in the consumer, so a test can
// fake it without a database.
type branchReader interface {
	ListForUser(ctx context.Context, q repository.Querier, accountID, userID uuid.UUID, isAdmin bool) ([]domain.Branch, error)
	ListAllForAccount(ctx context.Context, q repository.Querier, accountID uuid.UUID) ([]domain.Branch, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (*domain.Branch, error)
	CountActiveExcluding(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (int, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewBranch) (*domain.Branch, error)
	Update(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, in domain.BranchUpdate) (*domain.Branch, error)
	Deactivate(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) error
}

// branchChannelWriter opens the manual-entry channel a new branch needs.
type branchChannelWriter interface {
	CreateManualEntry(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) error
}

// BranchService owns the branches of an account.
type BranchService struct {
	db                tenantTxRunner
	branches          branchReader
	channels          branchChannelWriter
	defaultExpiryDays int
}

// NewBranchService builds a BranchService.
func NewBranchService(
	db tenantTxRunner, branches branchReader, channels branchChannelWriter, defaultExpiryDays int,
) *BranchService {
	return &BranchService{db: db, branches: branches, channels: channels,
		defaultExpiryDays: defaultExpiryDays}
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

// ListAllBranches returns every branch of the account, closed ones included, so an administrator
// can reopen one. A closed branch is deliberately absent from every other read — it is not a
// branch anyone may operate in — which is why administration needs its own.
func (s *BranchService) ListAllBranches(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.Branch, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}

	var branches []domain.Branch
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		branches, err = s.branches.ListAllForAccount(ctx, q, tenant.AccountID)
		return err
	}); err != nil {
		return nil, err
	}
	return branches, nil
}

// GetBranch returns one branch of the caller's account.
func (s *BranchService) GetBranch(
	ctx context.Context, tenant domain.Tenant, branchID uuid.UUID,
) (*domain.Branch, error) {
	var branch *domain.Branch
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		branch, err = s.branches.GetByID(ctx, q, tenant.AccountID, branchID)
		return err
	}); err != nil {
		return nil, err
	}
	return branch, nil
}

// CreateBranch opens a branch and its manual-entry channel in the same transaction, because a
// branch without that channel cannot take a counter or phone order.
func (s *BranchService) CreateBranch(
	ctx context.Context, tenant domain.Tenant, in domain.NewBranch,
) (*domain.Branch, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if in.DefaultExpiryDays <= 0 {
		in.DefaultExpiryDays = s.defaultExpiryDays
	}

	var branch *domain.Branch
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		branch, err = s.branches.Create(ctx, q, tenant.AccountID, in)
		if err != nil {
			return err
		}
		return s.channels.CreateManualEntry(ctx, q, tenant.AccountID, branch.ID)
	}); err != nil {
		return nil, err
	}
	return branch, nil
}

// UpdateBranch replaces a branch's editable fields.
func (s *BranchService) UpdateBranch(
	ctx context.Context, tenant domain.Tenant, branchID uuid.UUID, in domain.BranchUpdate,
) (*domain.Branch, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", domain.ErrInvalidInput)
	}
	if in.DefaultExpiryDays <= 0 {
		return nil, fmt.Errorf("%w: default expiry days must be greater than zero",
			domain.ErrInvalidInput)
	}

	var branch *domain.Branch
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if in.IsActive != nil && !*in.IsActive {
			if err := s.assertNotLastActive(ctx, q, tenant.AccountID, branchID); err != nil {
				return err
			}
		}
		var err error
		branch, err = s.branches.Update(ctx, q, tenant.AccountID, branchID, in)
		return err
	}); err != nil {
		return nil, err
	}
	return branch, nil
}

// DeactivateBranch closes a branch, refusing to close the last active one.
func (s *BranchService) DeactivateBranch(
	ctx context.Context, tenant domain.Tenant, branchID uuid.UUID,
) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if err := s.assertNotLastActive(ctx, q, tenant.AccountID, branchID); err != nil {
			return err
		}
		return s.branches.Deactivate(ctx, q, tenant.AccountID, branchID)
	})
}

// assertNotLastActive refuses to leave an account with nowhere to operate. Counted inside the
// caller's transaction so a concurrent close cannot slip past between check and write.
func (s *BranchService) assertNotLastActive(
	ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
) error {
	branch, err := s.branches.GetByID(ctx, q, accountID, branchID)
	if err != nil {
		return err
	}
	if !branch.IsActive {
		return nil
	}
	remaining, err := s.branches.CountActiveExcluding(ctx, q, accountID, branchID)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return domain.WithCode(domain.CodeLastActiveBranch,
			fmt.Errorf("%w: an account needs at least one active branch", domain.ErrInvalidInput))
	}
	return nil
}
