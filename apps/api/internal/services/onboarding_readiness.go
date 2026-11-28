package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type onboardingReadinessReader interface {
	GetReadiness(context.Context, repository.Querier, uuid.UUID, []uuid.UUID) (*domain.OnboardingReadiness, error)
}

// WithReadinessReader wires live operational setup diagnostics.
func (s *OnboardingService) WithReadinessReader(reader onboardingReadinessReader) *OnboardingService {
	s.readiness = reader
	return s
}

// GetReadiness derives the authorized account or selected branch's current setup.
func (s *OnboardingService) GetReadiness(ctx context.Context, tenant domain.Tenant) (*domain.OnboardingReadiness, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if s.readiness == nil {
		return nil, domain.ErrNotConfigured
	}
	var result *domain.OnboardingReadiness
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		result, err = s.readiness.GetReadiness(ctx, q, tenant.AccountID, tenant.BranchFilter())
		return err
	})
	return result, err
}
