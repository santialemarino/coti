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
	if err := s.checkContactLimit(contacts, 0); err != nil {
		return err
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
	switch len(profiles) {
	case 0:
		origin := domain.ClientOriginWebApp
		profile, err := s.profiles.Create(ctx, q, p.AccountID, domain.NewClient{Name: &in.Name, TaxID: &p.TaxID, OriginChannel: &origin})
		if err != nil {
			return err
		}
		if err := s.contacts.Save(ctx, q, p.AccountID, profile.ID, contacts, "CUSTOMER"); err != nil {
			return err
		}
		if err := s.contacts.ProjectPrimaries(ctx, q, p.AccountID, profile.ID); err != nil {
			return err
		}
		return s.contacts.Declare(ctx, q, p, profile.ID, contacts)
	case 1:
		// An earlier profile with this CUIT keeps its name, fiscal data and contacts until the customer
		// verifies one of the contacts it already had.
		return s.contacts.SaveDeclared(ctx, q, p, profiles[0].ID, contacts)
	default:
		return domain.ErrConflict
	}
}
