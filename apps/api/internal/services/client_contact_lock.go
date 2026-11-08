package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func (s *ClientContactService) lockContactValues(ctx context.Context, q repository.Querier, accountID uuid.UUID, contacts []domain.ClientContact) error {
	keys := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		value, err := domain.NormalizeClientContact(contact.Kind, contact.Value)
		if err == nil {
			keys = append(keys, "contact:"+string(contact.Kind)+":"+value)
		}
	}
	return s.contacts.Lock(ctx, q, accountID, keys...)
}

func (s *ClientContactService) lockContactProfiles(ctx context.Context, q repository.Querier, accountID uuid.UUID, taxID string, profiles []domain.Client) error {
	keys := make([]string, 0, len(profiles)+1)
	if taxID != "" {
		keys = append(keys, "tax:"+taxID)
	}
	for _, profile := range profiles {
		if profile.TaxID != nil {
			keys = append(keys, "tax:"+*profile.TaxID)
		} else {
			keys = append(keys, "profile:"+profile.ID.String())
		}
	}
	return s.contacts.Lock(ctx, q, accountID, keys...)
}
