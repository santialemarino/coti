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

// RegisterProfile creates the canonical profile only within the credential registration transaction.
func (s *ClientContactService) RegisterProfile(ctx context.Context, q repository.Querier, p domain.CustomerPrincipal, in domain.ClientContactUpdate) error {
	in.Name = strings.TrimSpace(in.Name)
	if p.CustomerID == uuid.Nil || !domain.ValidCUIT(p.TaxID) || in.Name == "" || utf8.RuneCountInString(in.Name) > maxClientNameLength || strings.ContainsFunc(in.Name, unicode.IsControl) {
		return domain.ErrInvalidInput
	}
	contacts, err := domain.NormalizeClientContacts(in.Contacts)
	if err != nil || len(contacts) == 0 {
		return domain.ErrInvalidInput
	}
	if err := s.lockContactValues(ctx, q, p.AccountID, contacts); err != nil {
		return err
	}
	if err := s.lockContactProfiles(ctx, q, p.AccountID, p.TaxID, nil); err != nil {
		return err
	}
	profiles, err := s.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
	if err != nil {
		return err
	}
	var profile *domain.Client
	if len(profiles) == 0 {
		origin := domain.ClientOriginWebApp
		profile, err = s.profiles.Create(ctx, q, p.AccountID, domain.NewClient{Name: &in.Name, TaxID: &p.TaxID, OriginChannel: &origin})
		if err != nil {
			return err
		}
	} else if len(profiles) == 1 {
		profile = &profiles[0]
		current, err := s.contacts.List(ctx, q, p.AccountID, []uuid.UUID{profile.ID})
		if err != nil {
			return err
		}
		for _, existing := range current[profile.ID] {
			found := false
			for _, contact := range contacts {
				if contact.Kind == existing.Kind && contact.Value == existing.Value {
					found = true
				}
			}
			if !found {
				for _, contact := range contacts {
					if contact.Kind == existing.Kind && contact.Primary {
						existing.Primary = false
					}
				}
				contacts = append(contacts, existing)
			}
		}
	} else {
		return domain.ErrConflict
	}
	if err := s.contacts.UpdateName(ctx, q, p.AccountID, profile.ID, in.Name); err != nil {
		return err
	}
	return s.contacts.Save(ctx, q, p.AccountID, profile.ID, contacts, "CUSTOMER")
}
