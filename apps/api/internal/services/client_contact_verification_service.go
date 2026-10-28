package services

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type contactVerificationStore interface {
	GrantProvisionalHistory(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID, domain.ChannelType, string) error
	GetChallenge(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID) (*domain.ContactChallenge, error)
	LockChallenge(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID) (*domain.ContactChallenge, error)
	HasRecentChallenge(context.Context, repository.Querier, domain.CustomerPrincipal, domain.ChannelType, string, time.Time) (bool, error)
	CreateChallenge(context.Context, repository.Querier, domain.ContactChallenge) error
	FinishChallenge(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID, bool) error
	MarkVerified(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID, domain.ChannelType, string) error
	GetContactHostname(context.Context, repository.Querier, uuid.UUID) (string, error)
}

// ClientContactVerificationService proves contact control before granting access to its previous orders.
type ClientContactVerificationService struct {
	contacts  *ClientContactService
	store     contactVerificationStore
	messenger domain.ContactMessenger
	cfg       config.CustomerAuthConfig
	now       func() time.Time
}

// NewClientContactVerificationService wires contact proofs to account-scoped persistence and delivery.
func NewClientContactVerificationService(contacts *ClientContactService, store contactVerificationStore, messenger domain.ContactMessenger, cfg config.CustomerAuthConfig) *ClientContactVerificationService {
	return &ClientContactVerificationService{contacts: contacts, store: store, messenger: messenger, cfg: cfg, now: time.Now}
}

// Request sends a bound, expiring proof only for a destination in the customer's own saved profile.
func (s *ClientContactVerificationService) Request(ctx context.Context, p domain.CustomerPrincipal, kind domain.ChannelType, value string) (*domain.ContactChallenge, error) {
	if p.CustomerID == uuid.Nil || s.messenger == nil {
		return nil, domain.ErrUnauthenticated
	}
	value, err := domain.NormalizeClientContact(kind, value)
	if err != nil {
		return nil, err
	}
	secret, err := newTokenSecret()
	if err != nil {
		return nil, err
	}
	if kind == domain.ChannelTypeWhatsApp {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return nil, err
		}
		secret = fmt.Sprintf("%06d", n.Int64())
	}
	c := &domain.ContactChallenge{ID: uuid.New(), AccountID: p.AccountID, CustomerID: p.CustomerID, Kind: kind, Value: value, ExpiresAt: s.now().Add(s.cfg.ContactVerificationTTL)}
	c.TokenHash = hashToken(c.ID.String() + ":" + secret)
	hostname := ""
	err = s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		if err := s.contacts.lockContactValues(ctx, q, p.AccountID, []domain.ClientContact{{Kind: kind, Value: value}}); err != nil {
			return err
		}
		recent, err := s.store.HasRecentChallenge(ctx, q, p, kind, value, s.now().Add(-s.cfg.ContactVerificationCooldown))
		if err != nil {
			return err
		}
		if recent {
			return domain.ErrLocked
		}
		profiles, err := s.contacts.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
		if err != nil {
			return err
		}
		if len(profiles) != 1 {
			return domain.ErrNotFound
		}
		c.ClientID = profiles[0].ID
		access, err := s.contacts.contacts.GetProfileAccess(ctx, q, p, c.ClientID)
		if err != nil {
			return err
		}
		visible, err := s.contacts.contacts.ListForCustomer(ctx, q, p, c.ClientID, access.Proved)
		if err != nil {
			return err
		}
		found := false
		for _, contact := range visible {
			if contact.Kind == kind && contact.Value == value {
				found = true
			}
		}
		if !found {
			return domain.ErrNotFound
		}
		hostname, err = s.store.GetContactHostname(ctx, q, p.AccountID)
		if err != nil {
			return err
		}
		return s.store.CreateChallenge(ctx, q, *c)
	})
	if err != nil {
		return nil, err
	}
	origin := "https://" + hostname
	if s.cfg.GoogleLocalOrigin != "" {
		origin = strings.TrimRight(s.cfg.GoogleLocalOrigin, "/")
	}
	body := contactVerificationLink(origin + "/verify-contact?challenge=" + c.ID.String() + "&token=" + url.QueryEscape(secret))
	code := ""
	if kind == domain.ChannelTypeWhatsApp {
		code = secret
		body = contactVerificationCode(secret)
	}
	_, err = s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: p.AccountID, Purpose: domain.ContactPurposeVerification, Kind: kind, To: value, Body: body, Code: code})
	if err != nil {
		_ = s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error { return s.store.FinishChallenge(ctx, q, p, c.ID, true) })
		return nil, err
	}
	return c, nil
}

// Confirm validates proof and moves provisional history in the same transaction as verification.
func (s *ClientContactVerificationService) Confirm(ctx context.Context, p domain.CustomerPrincipal, id uuid.UUID, secret string) error {
	if p.CustomerID == uuid.Nil || len(secret) > 128 {
		return domain.ErrUnauthenticated
	}
	var denied error
	err := s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		c, err := s.store.GetChallenge(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := s.contacts.lockContactValues(ctx, q, p.AccountID, []domain.ClientContact{{Kind: c.Kind, Value: c.Value}}); err != nil {
			return err
		}
		c, err = s.store.LockChallenge(ctx, q, p, id)
		if err != nil {
			return err
		}
		hash := hashToken(c.ID.String() + ":" + secret)
		if c.ConsumedAt != nil {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(c.TokenHash)) != 1 {
				return domain.ErrUnauthenticated
			}
			collections, err := s.contacts.contacts.List(ctx, q, p.AccountID, []uuid.UUID{c.ClientID})
			if err != nil {
				return err
			}
			for _, contact := range collections[c.ClientID] {
				if contact.Kind == c.Kind && contact.Value == c.Value && contact.Verified {
					return nil
				}
			}
			return domain.ErrUnauthenticated
		}
		if !c.ExpiresAt.After(s.now()) || c.Attempts >= s.cfg.ContactVerificationAttempts {
			return domain.ErrUnauthenticated
		}
		if subtle.ConstantTimeCompare([]byte(hash), []byte(c.TokenHash)) != 1 {
			denied = domain.ErrUnauthenticated
			return s.store.FinishChallenge(ctx, q, p, id, false)
		}
		candidates, err := s.contacts.contacts.FindProfiles(ctx, q, p.AccountID, "", c.Kind, c.Value)
		if err != nil {
			return err
		}
		if err := s.contacts.lockContactProfiles(ctx, q, p.AccountID, p.TaxID, candidates); err != nil {
			return err
		}
		profile, err := s.contacts.profiles.GetByID(ctx, q, p.AccountID, c.ClientID)
		if err != nil {
			return err
		}
		if profile.TaxID == nil || *profile.TaxID != p.TaxID {
			return domain.ErrForbidden
		}
		for _, candidate := range candidates {
			if candidate.ID != c.ClientID && candidate.TaxID != nil {
				return domain.ErrConflict
			}
		}
		ids := []uuid.UUID{c.ClientID}
		for _, candidate := range candidates {
			if candidate.ID != c.ClientID {
				ids = append(ids, candidate.ID)
			}
		}
		collections, err := s.contacts.contacts.List(ctx, q, p.AccountID, ids)
		if err != nil {
			return err
		}
		contacts := collections[c.ClientID]
		present := false
		for _, contact := range contacts {
			if contact.Kind == c.Kind && contact.Value == c.Value {
				present = true
			}
		}
		if !present {
			return domain.ErrNotFound
		}
		var sourceID *uuid.UUID
		var merged []domain.ClientContact
		for i := range candidates {
			if candidates[i].ID != c.ClientID {
				if sourceID != nil {
					return domain.ErrConflict
				}
				sourceID = &candidates[i].ID
			}
		}
		if sourceID != nil {
			if err := s.store.GrantProvisionalHistory(ctx, q, p, *sourceID, c.Kind, c.Value); err != nil {
				return err
			}
			if err := s.contacts.contacts.Merge(ctx, q, domain.Tenant{AccountID: p.AccountID, Role: domain.UserRoleAdmin}, *sourceID, c.ClientID); err != nil {
				return err
			}
		}
		// Verifying first lets the proof this confirm establishes decide the merged primaries.
		if err := s.store.MarkVerified(ctx, q, p, c.ClientID, c.Kind, c.Value); err != nil {
			return err
		}
		access, err := s.contacts.contacts.GetProfileAccess(ctx, q, p, c.ClientID)
		if err != nil {
			return err
		}
		if sourceID != nil {
			for _, incoming := range collections[*sourceID] {
				found, hasKind := false, false
				for _, existing := range contacts {
					if existing.Kind == incoming.Kind {
						hasKind = true
						if existing.Value == incoming.Value {
							found = true
						}
					}
				}
				if !found {
					incoming.Primary = !hasKind && access.Proved
					contacts = append(contacts, incoming)
					merged = append(merged, incoming)
				}
			}
		}
		if err := s.contacts.contacts.Save(ctx, q, p.AccountID, c.ClientID, contacts, "CUSTOMER"); err != nil {
			return err
		}
		if err := s.contacts.contacts.Declare(ctx, q, p, c.ClientID, merged); err != nil {
			return err
		}
		if access.Proved {
			if err := s.contacts.contacts.ProjectPrimaries(ctx, q, p.AccountID, c.ClientID); err != nil {
				return err
			}
		}
		return s.store.FinishChallenge(ctx, q, p, id, true)
	})
	if err != nil {
		return err
	}
	return denied
}
