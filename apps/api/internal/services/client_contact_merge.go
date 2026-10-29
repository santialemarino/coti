package services

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// ListMergeCandidates returns the profiles without a CUIT a seller could merge with clientID.
func (s *ClientContactService) ListMergeCandidates(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID) ([]domain.Client, error) {
	var result []domain.Client
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		client, err := s.profiles.GetByID(ctx, q, tenant.AccountID, clientID)
		if err != nil {
			return err
		}
		if client.TaxID != nil {
			return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		}
		result, err = s.contacts.ListMergeCandidates(ctx, q, tenant, clientID)
		return err
	})
	return result, err
}

// RenameClient lets a seller correct the name of a profile without a CUIT, such as one that kept a
// WhatsApp display name. A registered profile's name is its customer's.
func (s *ClientContactService) RenameClient(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID, name string) (*domain.Client, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxClientNameLength || strings.ContainsFunc(name, unicode.IsControl) {
		return nil, domain.ErrInvalidInput
	}
	var client *domain.Client
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		profiles, err := s.contacts.LockMergeProfiles(ctx, q, tenant, []uuid.UUID{clientID})
		if err != nil {
			return err
		}
		profile, ok := profiles[clientID]
		if !ok || profile.MergedInto != nil {
			return domain.ErrNotFound
		}
		if profile.TaxID != nil {
			return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		}
		if err := s.contacts.UpdateName(ctx, q, tenant.AccountID, clientID, name); err != nil {
			return err
		}
		if err := s.contacts.CompleteNameRequests(ctx, q, tenant.AccountID, clientID); err != nil {
			return err
		}
		client, err = s.profiles.GetByID(ctx, q, tenant.AccountID, clientID)
		return err
	})
	return client, err
}

// MergeClients merges source into target on a seller's word: both without a CUIT, live, and with
// every order in the caller's reach. Source's orders move and its contacts keep routing to target.
func (s *ClientContactService) MergeClients(ctx context.Context, tenant domain.Tenant, target, source uuid.UUID) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.mergeProvisional(ctx, q, tenant, source, target)
	})
}

// ResolveSuggestion settles one pending observation: MERGE joins its two profiles into the candidate,
// or into keep when the seller picked the source, and SEPARATE dismisses it without merging.
func (s *ClientContactService) ResolveSuggestion(ctx context.Context, tenant domain.Tenant, id uuid.UUID, action domain.ContactReviewStatus, keep *uuid.UUID) error {
	if action != domain.ContactReviewMerged && action != domain.ContactReviewSeparate {
		return domain.ErrInvalidInput
	}
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		o, err := s.contacts.GetObservation(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		if o.Status != domain.ContactReviewPending {
			return domain.ErrConflict
		}
		if action == domain.ContactReviewMerged {
			if o.SourceClientID == nil || o.CandidateClientID == nil {
				return domain.ErrInvalidInput
			}
			source, target := *o.SourceClientID, *o.CandidateClientID
			if keep != nil && *keep == source {
				source, target = target, source
			} else if keep != nil && *keep != target {
				return domain.ErrInvalidInput
			}
			// Either side may have been merged since the case was raised; it is settled on survivors.
			if source, err = s.profiles.GetSurvivor(ctx, q, tenant.AccountID, source); err != nil {
				return err
			}
			if target, err = s.profiles.GetSurvivor(ctx, q, tenant.AccountID, target); err != nil {
				return err
			}
			if source != target {
				if err := s.mergeProvisional(ctx, q, tenant, source, target); err != nil {
					return err
				}
			}
		}
		return s.contacts.Decide(ctx, q, tenant, id, string(action))
	})
}

// mergeProvisional runs one seller merge inside the caller's transaction, after locking both profiles
// against concurrent inbound routing.
func (s *ClientContactService) mergeProvisional(ctx context.Context, q repository.Querier, tenant domain.Tenant, source, target uuid.UUID) error {
	if source == target {
		return domain.ErrInvalidInput
	}
	if err := s.lockContactProfiles(ctx, q, tenant.AccountID, "", []domain.Client{{ID: source}, {ID: target}}); err != nil {
		return err
	}
	profiles, err := s.contacts.LockMergeProfiles(ctx, q, tenant, []uuid.UUID{source, target})
	if err != nil {
		return err
	}
	for _, id := range []uuid.UUID{source, target} {
		profile, ok := profiles[id]
		switch {
		case !ok:
			return domain.ErrNotFound
		case profile.TaxID != nil:
			return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		case profile.MergedInto != nil:
			return domain.WithCode(domain.CodeClientMerged, domain.ErrConflict)
		case !profile.InReach:
			return domain.WithCode(domain.CodeClientOutOfReach, domain.ErrForbidden)
		}
	}
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, source, target); err != nil {
		return err
	}
	if err := s.contacts.Merge(ctx, q, tenant, source, target); err != nil {
		return err
	}
	return s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, target)
}
