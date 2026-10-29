package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type registrationLinkStore interface {
	GetContactHostname(context.Context, repository.Querier, uuid.UUID) (string, error)
	HasRecentWhatsAppMessage(context.Context, repository.Querier, uuid.UUID, uuid.UUID, []string, time.Duration) (bool, error)
}

// SendRegistrationLink sends a seller-requested portal link without changing customer credentials.
// WhatsApp refuses free text outside its customer-service window, so the link only goes there while
// the customer wrote to the branch within it; otherwise the seller copies the link themselves.
func (s *ClientContactService) SendRegistrationLink(ctx context.Context, tenant domain.Tenant, clientID uuid.UUID, kind domain.ChannelType) error {
	if err := requireBranch(tenant, "a customer registration link"); err != nil {
		return err
	}
	if s.messenger == nil {
		return domain.ErrNotConfigured
	}
	if kind != domain.ChannelTypeEmail && kind != domain.ChannelTypeWhatsApp {
		return domain.ErrInvalidInput
	}
	store, ok := s.contacts.(registrationLinkStore)
	if !ok {
		return domain.ErrNotConfigured
	}
	hostname, destination := "", ""
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.profiles.GetByID(ctx, q, tenant.AccountID, clientID); err != nil {
			return err
		}
		collections, err := s.contacts.List(ctx, q, tenant.AccountID, []uuid.UUID{clientID})
		if err != nil {
			return err
		}
		for _, contact := range collections[clientID] {
			if contact.Kind == kind && contact.Primary && contact.Verified {
				destination = contact.Value
			}
		}
		if destination == "" {
			return domain.ErrInvalidInput
		}
		if kind == domain.ChannelTypeWhatsApp {
			forms, err := domain.InboundSenderDestinations(kind, destination)
			if err != nil {
				return err
			}
			senders := make([]string, 0, 2*len(forms))
			for _, form := range forms {
				senders = append(senders, form, strings.TrimPrefix(form, "+"))
			}
			open, err := store.HasRecentWhatsAppMessage(ctx, q, tenant.AccountID, tenant.BranchID, senders, nameReplyWindow)
			if err != nil {
				return err
			}
			if !open {
				return domain.WithCode(domain.CodeWhatsAppWindowClosed, fmt.Errorf("%w: no customer message within the WhatsApp window", domain.ErrInvalidInput))
			}
		}
		hostname, err = store.GetContactHostname(ctx, q, tenant.AccountID)
		return err
	})
	if err != nil {
		return err
	}
	origin := "https://" + hostname
	if s.localOrigin != "" {
		origin = strings.TrimRight(s.localOrigin, "/")
	}
	body := contactRegistrationLink(origin + "/login?next=%2Fprofile")
	_, err = s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: tenant.AccountID, BranchID: tenant.BranchID, Purpose: domain.ContactPurposeRegistrationLink, Kind: kind, To: destination, Body: body})
	return err
}
