package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// recoveryCodeSpace is how many distinct six-digit codes there are.
const recoveryCodeSpace = 1000000

// ListRecoveryContacts offers a customer's verified contacts masked, the main one first, and
// preselects the first one a code can reach.
func (s *ClientContactVerificationService) ListRecoveryContacts(ctx context.Context, q repository.Querier, p domain.CustomerPrincipal) ([]domain.RecoveryContact, error) {
	clientID, err := s.recoveryProfile(ctx, q, p)
	if errors.Is(err, domain.ErrNotFound) {
		return []domain.RecoveryContact{}, nil
	}
	if err != nil {
		return nil, err
	}
	contacts, err := s.store.ListRecoveryContacts(ctx, q, p.AccountID, clientID, p.CustomerID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.RecoveryContact, 0, len(contacts))
	preselected := false
	for _, c := range contacts {
		option := domain.RecoveryContact{ID: c.ID, Kind: c.Kind, Masked: domain.MaskContact(c.Kind, c.Value), Main: c.Main, Available: s.codeDeliverable(c.Kind)}
		option.Preselected = option.Available && !preselected
		preselected = preselected || option.Preselected
		result = append(result, option)
	}
	return result, nil
}

// RequestRecovery sends a one-use recovery code to one of the customer's verified contacts.
func (s *ClientContactVerificationService) RequestRecovery(ctx context.Context, p domain.CustomerPrincipal, contactID uuid.UUID) (*domain.ContactChallenge, error) {
	if s.messenger == nil {
		return nil, domain.ErrNotConfigured
	}
	n, err := rand.Int(rand.Reader, big.NewInt(recoveryCodeSpace))
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	c := &domain.ContactChallenge{ID: uuid.New(), AccountID: p.AccountID, CustomerID: p.CustomerID, Purpose: domain.ContactChallengeRecover, ExpiresAt: s.now().Add(s.cfg.RecoveryTTL)}
	c.TokenHash = hashToken(c.ID.String() + ":" + code)
	err = s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		clientID, err := s.recoveryProfile(ctx, q, p)
		if err != nil {
			return err
		}
		c.ClientID = clientID
		contact, err := s.ownedContact(ctx, q, p, clientID, func(candidate domain.ClientContact) bool { return candidate.ID == contactID })
		if err != nil {
			return err
		}
		if !s.codeDeliverable(contact.Kind) {
			return domain.ErrNotConfigured
		}
		c.Kind, c.Value = contact.Kind, contact.Value
		if err := s.contacts.lockContactValues(ctx, q, p.AccountID, []domain.ClientContact{*contact}); err != nil {
			return err
		}
		recent, err := s.store.HasRecentChallenge(ctx, q, p, c.Kind, c.Value, s.now().Add(-s.cfg.ContactVerificationCooldown))
		if err != nil {
			return err
		}
		if recent {
			return domain.ErrLocked
		}
		// TODO(#205): apply the per-supplier destination cap and the cross-supplier spam cap here.
		return s.store.CreateChallenge(ctx, q, *c)
	})
	if err != nil {
		return nil, err
	}
	_, err = s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: p.AccountID, Purpose: domain.ContactPurposeRecovery, Kind: c.Kind, To: c.Value, Body: contactRecoveryCode(code), Code: code})
	if err != nil {
		_ = s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error { return s.store.FinishChallenge(ctx, q, p, c.ID, true) })
		return nil, err
	}
	return c, nil
}

// RedeemRecovery consumes a recovery code inside the caller's transaction and returns the customer it
// recovers. A wrong code spends an attempt and returns false; the caller commits so the attempt counts.
func (s *ClientContactVerificationService) RedeemRecovery(ctx context.Context, q repository.Querier, accountID, challengeID uuid.UUID, code string) (uuid.UUID, bool, error) {
	c, err := s.store.LockRecoveryChallenge(ctx, q, accountID, challengeID)
	if errors.Is(err, domain.ErrNotFound) {
		return uuid.Nil, false, domain.ErrUnauthenticated
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	if c.ConsumedAt != nil || !c.ExpiresAt.After(s.now()) || c.Attempts >= s.cfg.ContactVerificationAttempts {
		return uuid.Nil, false, domain.ErrUnauthenticated
	}
	p := domain.CustomerPrincipal{AccountID: accountID, CustomerID: c.CustomerID}
	if subtle.ConstantTimeCompare([]byte(hashToken(c.ID.String()+":"+code)), []byte(c.TokenHash)) != 1 {
		// TODO(#205): record the wrong code against the destination so its lockout counts it.
		return uuid.Nil, false, s.store.FinishChallenge(ctx, q, p, c.ID, false)
	}
	// A contact removed or handed over since the code was sent recovers nothing.
	if _, err := s.ownedContact(ctx, q, p, c.ClientID, func(candidate domain.ClientContact) bool {
		return candidate.Kind == c.Kind && candidate.Value == c.Value
	}); err != nil {
		return uuid.Nil, false, domain.ErrUnauthenticated
	}
	if err := s.store.FinishChallenge(ctx, q, p, c.ID, true); err != nil {
		return uuid.Nil, false, err
	}
	return c.CustomerID, true, nil
}

// codeDeliverable reports whether a one-use code can reach a contact of this kind. A WhatsApp code
// outside the 24-hour window needs Meta's authentication template, so WhatsApp waits for it.
func (s *ClientContactVerificationService) codeDeliverable(kind domain.ChannelType) bool {
	return kind == domain.ChannelTypeEmail || s.cfg.ContactCodeTemplate != ""
}

// recoveryProfile is the one live profile carrying the customer's CUIT/CUIL.
func (s *ClientContactVerificationService) recoveryProfile(ctx context.Context, q repository.Querier, p domain.CustomerPrincipal) (uuid.UUID, error) {
	profiles, err := s.contacts.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(profiles) != 1 {
		return uuid.Nil, domain.ErrNotFound
	}
	return profiles[0].ID, nil
}

func (s *ClientContactVerificationService) ownedContact(ctx context.Context, q repository.Querier, p domain.CustomerPrincipal, clientID uuid.UUID, match func(domain.ClientContact) bool) (*domain.ClientContact, error) {
	contacts, err := s.store.ListRecoveryContacts(ctx, q, p.AccountID, clientID, p.CustomerID)
	if err != nil {
		return nil, err
	}
	for i := range contacts {
		if match(contacts[i]) {
			return &contacts[i], nil
		}
	}
	return nil, domain.ErrNotFound
}
