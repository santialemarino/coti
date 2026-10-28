package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// maxRecoveryCodeLength bounds the code a caller may submit before it is hashed.
const maxRecoveryCodeLength = 16

type customerContactRecovery interface {
	ListRecoveryContacts(context.Context, repository.Querier, domain.CustomerPrincipal) ([]domain.RecoveryContact, error)
	RequestRecovery(context.Context, domain.CustomerPrincipal, uuid.UUID) (*domain.ContactChallenge, error)
	RedeemRecovery(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) (uuid.UUID, bool, error)
}

// WithContactRecovery enables account recovery through a code sent to a verified contact.
func (s *CustomerAuthService) WithContactRecovery(recovery customerContactRecovery) *CustomerAuthService {
	s.recovery = recovery
	return s
}

// RecoveryOptions lists the masked verified contacts of the account a CUIT/CUIL names. An unknown or
// disabled CUIT/CUIL answers exactly like an account with no verified contact: an empty list.
func (s *CustomerAuthService) RecoveryOptions(ctx context.Context, hostname, taxID string) ([]domain.RecoveryContact, error) {
	if s.recovery == nil {
		return nil, domain.ErrNotConfigured
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	taxID = domain.NormalizeTaxID(taxID)
	if !domain.ValidCUIT(taxID) {
		return nil, domain.ErrInvalidInput
	}
	contacts := []domain.RecoveryContact{}
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		identity, err := s.store.GetIdentityByTaxID(ctx, q, site.AccountID, taxID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		contacts, err = s.recovery.ListRecoveryContacts(ctx, q, domain.CustomerPrincipal{CustomerID: identity.ID, AccountID: site.AccountID, TaxID: identity.TaxID})
		return err
	})
	return contacts, err
}

// RequestRecoveryCode sends a one-use code to one of the account's verified contacts. The CUIT/CUIL
// only selects the account; nothing is granted until the code comes back.
func (s *CustomerAuthService) RequestRecoveryCode(ctx context.Context, hostname, taxID string, contactID uuid.UUID) (*domain.ContactChallenge, error) {
	if s.recovery == nil {
		return nil, domain.ErrNotConfigured
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	taxID = domain.NormalizeTaxID(taxID)
	if !domain.ValidCUIT(taxID) {
		return nil, domain.ErrInvalidInput
	}
	var identity *domain.CustomerIdentity
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		var err error
		identity, err = s.store.GetIdentityByTaxID(ctx, q, site.AccountID, taxID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.recovery.RequestRecovery(ctx, domain.CustomerPrincipal{CustomerID: identity.ID, AccountID: site.AccountID, TaxID: identity.TaxID}, contactID)
}

// ResetWithRecoveryCode redeems a recovery code, replaces the password, revokes every earlier session
// and signs the customer in. A Google-only account gains a password the same way.
func (s *CustomerAuthService) ResetWithRecoveryCode(ctx context.Context, hostname string, challengeID uuid.UUID, code, password string) (*domain.CustomerLogin, error) {
	if s.recovery == nil || s.credentials == nil {
		return nil, domain.ErrNotConfigured
	}
	if code == "" || len(code) > maxRecoveryCodeLength {
		return nil, domain.ErrUnauthenticated
	}
	// The policy is checked first, so a weak password never spends one of the code's attempts.
	if err := (domain.PasswordPolicy{MinLength: s.cfg.PasswordMinLength}).Validate(password); err != nil {
		return nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	secret, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	var login *domain.CustomerLogin
	denied := false
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		customerID, ok, err := s.recovery.RedeemRecovery(ctx, q, site.AccountID, challengeID, code)
		if err != nil {
			return err
		}
		if !ok {
			// Committed, so the spent attempt counts.
			denied = true
			return nil
		}
		identity, err := s.store.GetIdentityForUpdate(ctx, q, site.AccountID, customerID)
		if err != nil {
			return err
		}
		if identity.DisabledAt != nil {
			return domain.ErrUnauthenticated
		}
		if err := s.credentials.ResetCustomerPassword(ctx, q, site.AccountID, customerID, hash, s.now()); err != nil {
			return err
		}
		login, err = s.createSession(ctx, q, *identity, secret)
		return err
	})
	if err != nil {
		return nil, err
	}
	if denied {
		return nil, domain.ErrUnauthenticated
	}
	return login, nil
}
