package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// DeleteRecommendationAlternative removes a recommendation from a mutable quote only.
func (s *QuoteService) DeleteRecommendationAlternative(
	ctx context.Context, tenant domain.Tenant, quoteID, itemID, alternativeID uuid.UUID,
) error {
	if err := requireBranch(tenant, "a quote alternative"); err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.ClaimForEdit(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted {
			return domain.ErrConflict
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, quote.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if version.IsImmutable {
			return domain.ErrConflict
		}
		return s.quotes.DeleteRecommendationAlternative(ctx, q, tenant.AccountID, version.ID, itemID, alternativeID)
	})
}
