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

// ListMergeCandidates returns the profiles without a customer account a seller could merge with
// clientID.
func (s *ClientContactService) ListMergeCandidates(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID) ([]domain.Client, error) {
	var result []domain.Client
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.profiles.GetByID(ctx, q, tenant.AccountID, clientID); err != nil {
			return err
		}
		registered, err := s.profiles.IsRegistered(ctx, q, tenant.AccountID, clientID)
		if err != nil {
			return err
		}
		if registered {
			return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		}
		result, err = s.contacts.ListMergeCandidates(ctx, q, tenant, clientID)
		return err
	})
	return result, err
}

// RenameClient lets a seller correct the name of a profile without a customer account, such as one
// that kept a WhatsApp display name. A registered profile's name is its customer's.
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
		if profile.Registered {
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

// MergeClients merges source into target on a seller's word: both without a customer account, live,
// and with every order in the caller's reach. Source's orders move and its contacts keep routing to
// target. A CUIT never ends up on a merged profile: when only source carries one, source survives.
func (s *ClientContactService) MergeClients(ctx context.Context, tenant domain.Tenant, target, source uuid.UUID) error {
	return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.mergeProvisional(ctx, q, tenant, source, target)
	})
}

// ResolveSuggestion settles one pending observation: MERGE joins its two profiles into the candidate,
// a provisional profile into the customer account on the other side, or into keep when the seller
// picked the source; SEPARATE dismisses it without merging.
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
				if err := s.mergeResolved(ctx, q, tenant, source, target, keep != nil, *o); err != nil {
					return err
				}
			}
		}
		return s.contacts.Decide(ctx, q, tenant, id, string(action))
	})
}

// mergeResolved merges the two survivors of a case: two provisional profiles as a seller merge, or a
// provisional one into the customer's account, which always survives, when kept names no other side.
func (s *ClientContactService) mergeResolved(ctx context.Context, q repository.Querier, tenant domain.Tenant, source, target uuid.UUID, kept bool, o domain.ContactObservation) error {
	sourceRegistered, err := s.profiles.IsRegistered(ctx, q, tenant.AccountID, source)
	if err != nil {
		return err
	}
	targetRegistered, err := s.profiles.IsRegistered(ctx, q, tenant.AccountID, target)
	if err != nil {
		return err
	}
	switch {
	case !sourceRegistered && !targetRegistered:
		return s.mergeProvisional(ctx, q, tenant, source, target)
	case sourceRegistered && targetRegistered, sourceRegistered && kept:
		return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
	case sourceRegistered:
		return s.joinAccount(ctx, q, tenant, target, source, o.Kind, o.Value)
	default:
		return s.joinAccount(ctx, q, tenant, source, target, o.Kind, o.Value)
	}
}

// joinAccount merges a provisional profile into a registered one on the seller's confirmation, once
// the account's customer has proved the shared contact: the customer's proof and the seller's word
// are the two signals a typo alone never gives. The customer keeps the provisional orders.
func (s *ClientContactService) joinAccount(ctx context.Context, q repository.Querier, tenant domain.Tenant, provisional, account uuid.UUID, kind domain.ChannelType, value string) error {
	if err := s.lockContactProfiles(ctx, q, tenant.AccountID, "", []domain.Client{{ID: provisional}, {ID: account}}); err != nil {
		return err
	}
	profiles, err := s.contacts.LockMergeProfiles(ctx, q, tenant, []uuid.UUID{provisional, account})
	if err != nil {
		return err
	}
	source, sourceFound := profiles[provisional]
	target, targetFound := profiles[account]
	switch {
	case !sourceFound || !targetFound:
		return domain.ErrNotFound
	case source.Registered || !target.Registered:
		return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
	case source.MergedInto != nil || target.MergedInto != nil:
		return domain.WithCode(domain.CodeClientMerged, domain.ErrConflict)
	case !source.InReach:
		return domain.WithCode(domain.CodeClientOutOfReach, domain.ErrForbidden)
	case source.TaxID != nil:
		return domain.WithCode(domain.CodeClientTaxIDConflict, domain.ErrConflict)
	}
	owner, err := s.contacts.GetContactOwner(ctx, q, tenant.AccountID, account, kind, value)
	if err != nil {
		return err
	}
	if owner == nil {
		return domain.WithCode(domain.CodeClientContactUnproved, domain.ErrConflict)
	}
	if err := s.contacts.GrantProvisionalHistory(ctx, q, domain.CustomerPrincipal{AccountID: tenant.AccountID, CustomerID: *owner}, provisional, kind, value); err != nil {
		return err
	}
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, provisional, account); err != nil {
		return err
	}
	if err := s.contacts.Merge(ctx, q, tenant, provisional, account); err != nil {
		return err
	}
	return s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, account)
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
		case profile.Registered:
			return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		case profile.MergedInto != nil:
			return domain.WithCode(domain.CodeClientMerged, domain.ErrConflict)
		case !profile.InReach:
			return domain.WithCode(domain.CodeClientOutOfReach, domain.ErrForbidden)
		}
	}
	if profiles[source].TaxID != nil {
		if profiles[target].TaxID != nil {
			return domain.WithCode(domain.CodeClientTaxIDConflict, domain.ErrConflict)
		}
		source, target = target, source
	}
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, source, target); err != nil {
		return err
	}
	if err := s.contacts.Merge(ctx, q, tenant, source, target); err != nil {
		return err
	}
	return s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, target)
}
