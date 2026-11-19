package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// SetFailedArchived changes a failed source's archive flag within the seller's reach.
func (s *RFQService) SetFailedArchived(ctx context.Context, tenant domain.Tenant, id uuid.UUID, archived bool) error {
	if err := requireBranch(tenant, "archiving a failed request"); err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		item, err := s.rfqs.GetByRFQID(ctx, q, tenant, id, s.invoicingEnv)
		if err != nil {
			return err
		}
		if item.BranchID != tenant.BranchID {
			return domain.ErrNotFound
		}
		if item.Status != "FAILED" {
			return domain.ErrConflict
		}
		return s.rfqs.SetFailedArchived(ctx, q, tenant.AccountID, tenant.BranchID, id, archived)
	})
}
