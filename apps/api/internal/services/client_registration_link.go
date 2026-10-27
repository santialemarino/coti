package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// SendRegistrationLink sends a seller-requested portal link without changing customer credentials.
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
	store, ok := s.contacts.(contactVerificationStore)
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
	body := "Para completar tus datos de facturación, ingresá a tu cuenta o creala en " + origin + "/login?next=%2Fprofile. Si ya tenés una cuenta con tu CUIT/CUIL, usá esa misma cuenta y agregá el contacto con el que hiciste el pedido."
	_, err = s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: tenant.AccountID, BranchID: tenant.BranchID, Kind: kind, To: destination, Body: body})
	return err
}
