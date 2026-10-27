package services

import (
	"context"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// GetRecoveryEmail reads only the authenticated customer's recovery destination.
func (s *CustomerAuthService) GetRecoveryEmail(ctx context.Context, principal domain.CustomerPrincipal) (*domain.CustomerRecoveryEmail, error) {
	var result *domain.CustomerRecoveryEmail
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: principal.AccountID}, func(q repository.Querier) error {
		identity, err := s.store.GetIdentityForUpdate(ctx, q, principal.AccountID, principal.CustomerID)
		if err != nil {
			return err
		}
		if identity.DisabledAt != nil {
			return domain.ErrUnauthenticated
		}
		result = &domain.CustomerRecoveryEmail{Email: identity.RecoveryEmail, Verified: identity.RecoveryEmailVerifiedAt != nil}
		return nil
	})
	return result, err
}

// UpdateRecoveryEmail requires a new proof before a changed address can recover credentials.
func (s *CustomerAuthService) UpdateRecoveryEmail(ctx context.Context, hostname string, principal domain.CustomerPrincipal, raw string) error {
	if s.credentials == nil || s.mail == nil {
		return domain.ErrNotConfigured
	}
	email := domain.NormalizeEmail(raw)
	if email != "" && !validCustomerEmail(email) {
		return domain.ErrInvalidInput
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return err
	}
	if site.AccountID != principal.AccountID {
		return domain.ErrForbidden
	}
	var saved *domain.CustomerIdentity
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: principal.AccountID}, func(q repository.Querier) error {
		identity, err := s.store.GetIdentityForUpdate(ctx, q, principal.AccountID, principal.CustomerID)
		if err != nil {
			return err
		}
		if identity.DisabledAt != nil {
			return domain.ErrUnauthenticated
		}
		if identity.RecoveryEmail != nil && *identity.RecoveryEmail == email {
			if identity.RecoveryEmailVerifiedAt == nil {
				saved = identity
			}
			return nil
		}
		var destination *string
		if email != "" {
			destination = &email
		}
		if err := s.credentials.SetRecoveryEmail(ctx, q, principal.AccountID, principal.CustomerID, destination); err != nil {
			return err
		}
		identity.RecoveryEmail = destination
		identity.RecoveryEmailVerifiedAt = nil
		if destination != nil {
			saved = identity
		}
		return nil
	})
	if err != nil || saved == nil {
		return err
	}
	return s.sendCustomerChallenge(ctx, *site, *saved, "VERIFY_EMAIL", email, true)
}
