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

// ListMergeCandidates returns the profiles a seller could merge with clientID, customer accounts
// included beside a profile without a CUIT.
func (s *ClientContactService) ListMergeCandidates(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID) ([]domain.Client, error) {
	var result []domain.Client
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.profiles.GetByID(ctx, q, tenant.AccountID, clientID); err != nil {
			return err
		}
		var err error
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

// MergeClients merges source into target on a seller's word and returns the survivor. Two profiles
// without a customer account merge as asked, live and with every order in the caller's reach, except
// that a CUIT never ends up on a merged profile. A profile without an account merges into a
// customer's account, which always survives, whichever side the seller named (mergeIntoAccount).
func (s *ClientContactService) MergeClients(ctx context.Context, tenant domain.Tenant, target, source uuid.UUID) (uuid.UUID, error) {
	var survivor uuid.UUID
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if source == target {
			return domain.ErrInvalidInput
		}
		// Same order as inbound routing: the contact locks first, then the profile rows.
		if err := s.lockContactProfiles(ctx, q, tenant.AccountID, "", []domain.Client{{ID: source}, {ID: target}}); err != nil {
			return err
		}
		profiles, err := s.contacts.LockMergeProfiles(ctx, q, tenant, []uuid.UUID{source, target})
		if err != nil {
			return err
		}
		from, fromFound := profiles[source]
		into, intoFound := profiles[target]
		if fromFound && intoFound && from.Registered != into.Registered {
			provisional, account := source, target
			if from.Registered {
				provisional, account = target, source
			}
			survivor = account
			return s.mergeIntoAccount(ctx, q, tenant, provisional, account)
		}
		survivor, err = s.mergeProvisional(ctx, q, tenant, source, target)
		return err
	})
	return survivor, err
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
		if o.Status != domain.ContactReviewPending && (o.Status != domain.ContactReviewMerged || action != domain.ContactReviewMerged) {
			return domain.ErrConflict
		}
		if action == domain.ContactReviewMerged {
			if o.SourceClientID == nil || o.CandidateClientID == nil {
				return domain.ErrInvalidInput
			}
			source, target := *o.SourceClientID, *o.CandidateClientID
			// Either side may have been merged since the case was raised; it is settled on survivors.
			if source, err = s.profiles.GetSurvivor(ctx, q, tenant.AccountID, source); err != nil {
				return err
			}
			if target, err = s.profiles.GetSurvivor(ctx, q, tenant.AccountID, target); err != nil {
				return err
			}
			if keep != nil && (*keep == source || *keep == *o.SourceClientID) {
				source, target = target, source
			} else if keep != nil && *keep != target && *keep != *o.CandidateClientID {
				return domain.ErrInvalidInput
			}
			if o.Status == domain.ContactReviewMerged {
				if source != target {
					return domain.ErrConflict
				}
				return nil
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
		_, err := s.mergeProvisional(ctx, q, tenant, source, target)
		return err
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
	// Only the proved case contact is the customer's; the others arrive unverified, as on any join.
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, provisional, account, false); err != nil {
		return err
	}
	if err := s.contacts.Merge(ctx, q, tenant, provisional, account, false); err != nil {
		return err
	}
	return s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, account)
}

// mergeProvisional runs one seller merge inside the caller's transaction, after locking both profiles
// against concurrent inbound routing, and returns the survivor. A retry of a merge that already
// happened succeeds without moving anything again.
func (s *ClientContactService) mergeProvisional(ctx context.Context, q repository.Querier, tenant domain.Tenant, source, target uuid.UUID) (uuid.UUID, error) {
	if source == target {
		return uuid.Nil, domain.ErrInvalidInput
	}
	if err := s.lockContactProfiles(ctx, q, tenant.AccountID, "", []domain.Client{{ID: source}, {ID: target}}); err != nil {
		return uuid.Nil, err
	}
	profiles, err := s.contacts.LockMergeProfiles(ctx, q, tenant, []uuid.UUID{source, target})
	if err != nil {
		return uuid.Nil, err
	}
	for _, pair := range [][2]uuid.UUID{{source, target}, {target, source}} {
		if p, ok := profiles[pair[0]]; ok && p.MergedInto != nil && *p.MergedInto == pair[1] {
			return pair[1], nil
		}
	}
	for _, id := range []uuid.UUID{source, target} {
		profile, ok := profiles[id]
		switch {
		case !ok:
			return uuid.Nil, domain.ErrNotFound
		case profile.Registered:
			return uuid.Nil, domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
		case profile.MergedInto != nil:
			return uuid.Nil, domain.WithCode(domain.CodeClientMerged, domain.ErrConflict)
		case !profile.InReach:
			return uuid.Nil, domain.WithCode(domain.CodeClientOutOfReach, domain.ErrForbidden)
		}
	}
	if profiles[source].TaxID != nil {
		if profiles[target].TaxID != nil {
			return uuid.Nil, domain.WithCode(domain.CodeClientTaxIDConflict, domain.ErrConflict)
		}
		source, target = target, source
	}
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, source, target, true); err != nil {
		return uuid.Nil, err
	}
	if err := s.contacts.Merge(ctx, q, tenant, source, target, false); err != nil {
		return uuid.Nil, err
	}
	return target, s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, target)
}

// mergeIntoAccount merges a provisional profile into a customer's account on the seller's word. The
// account survives and the seller sees the history together at once; the provisional contacts arrive
// unverified, and its orders reach the customer's portal only after the customer proves one of them,
// so a wrong merge never shows one person's orders to another.
func (s *ClientContactService) mergeIntoAccount(ctx context.Context, q repository.Querier, tenant domain.Tenant, provisional, account uuid.UUID) error {
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
	case source.MergedInto != nil && *source.MergedInto == account:
		return nil
	case source.Registered || !target.Registered:
		return domain.WithCode(domain.CodeClientRegistered, domain.ErrForbidden)
	case source.MergedInto != nil || target.MergedInto != nil:
		return domain.WithCode(domain.CodeClientMerged, domain.ErrConflict)
	case !source.InReach:
		return domain.WithCode(domain.CodeClientOutOfReach, domain.ErrForbidden)
	case source.TaxID != nil:
		return domain.WithCode(domain.CodeClientTaxIDConflict, domain.ErrConflict)
	}
	if err := s.contacts.MoveContacts(ctx, q, tenant.AccountID, provisional, account, false); err != nil {
		return err
	}
	if err := s.contacts.Merge(ctx, q, tenant, provisional, account, true); err != nil {
		return err
	}
	return s.contacts.ProjectPrimaries(ctx, q, tenant.AccountID, account)
}
