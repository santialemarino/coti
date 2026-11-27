package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// ListTagRules returns the effective automatic tag settings for an administrator.
func (s *ClientService) ListTagRules(ctx context.Context, tenant domain.Tenant) ([]domain.ClientTagRule, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	var rules []domain.ClientTagRule
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		rules, err = s.tags.ListRules(ctx, q, tenant.AccountID)
		return err
	})
	return rules, err
}

// UpdateTagRule saves a single automatic tag rule for an administrator.
func (s *ClientService) UpdateTagRule(ctx context.Context, tenant domain.Tenant, rule domain.ClientTagRule) error {
	if !tenant.IsAdmin() {
		return domain.ErrForbidden
	}
	if !rule.Kind.Valid() || rule.Minimum < 1 || rule.WindowDays < 0 {
		return domain.ErrInvalidInput
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.tags.UpdateRule(ctx, q, tenant.AccountID, rule)
	})
}

// SetTagExclusion records or clears one client's automatic tag exclusion.
func (s *ClientService) SetTagExclusion(
	ctx context.Context, tenant domain.Tenant, clientID, tagID uuid.UUID, excluded bool,
) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		survivor, err := s.clients.GetSurvivor(ctx, q, tenant.AccountID, clientID)
		if err != nil {
			return err
		}
		return s.tags.SetExclusion(ctx, q, tenant.AccountID, survivor, tagID, excluded)
	})
}
