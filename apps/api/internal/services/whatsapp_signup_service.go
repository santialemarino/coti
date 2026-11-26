package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type whatsAppSignupProvider interface {
	Authorize(context.Context, string, string, string) (domain.WhatsAppSignupGrant, error)
	Subscribe(context.Context, string, string) error
	Register(context.Context, string, string, string) error
}

type whatsAppSignupDB interface {
	tenantTxRunner
	WithAdvisoryQuerier(context.Context, string, func(repository.Querier) error) error
}

type whatsAppSignupStore interface {
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Channel, error)
	GetActiveByTypeAndIdentifiersCrossAccount(context.Context, repository.Querier, domain.ChannelType, []string) (map[string]domain.Channel, error)
	HasWhatsAppReceipt(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

// WhatsAppSignupService connects a verified Meta number to the selected branch.
type WhatsAppSignupService struct {
	db       whatsAppSignupDB
	store    whatsAppSignupStore
	provider whatsAppSignupProvider
	channels *ChannelService
	secret   string
}

// NewWhatsAppSignupService builds the Embedded Signup service.
func NewWhatsAppSignupService(db whatsAppSignupDB, store whatsAppSignupStore, provider whatsAppSignupProvider, channels *ChannelService, secret string) *WhatsAppSignupService {
	return &WhatsAppSignupService{db: db, store: store, provider: provider, channels: channels, secret: secret}
}

// GetReceipt checks whether the currently connected phone has delivered a message with a saved RFQ.
func (s *WhatsAppSignupService) GetReceipt(ctx context.Context, tenant domain.Tenant, channelID uuid.UUID) (bool, error) {
	if !tenant.IsAdmin() {
		return false, domain.ErrForbidden
	}
	if err := requireBranch(tenant, "a WhatsApp channel"); err != nil {
		return false, err
	}
	var received bool
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		channel, err := s.store.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
		if err != nil {
			return err
		}
		if channel.Type != domain.ChannelTypeWhatsApp {
			return domain.ErrNotFound
		}
		received, err = s.store.HasWhatsAppReceipt(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
		return err
	})
	return received, err
}

// Connect authorizes the selected Meta assets before replacing the branch credentials.
func (s *WhatsAppSignupService) Connect(ctx context.Context, tenant domain.Tenant, channelID uuid.UUID, code, accountID, phoneID string) error {
	if !tenant.IsAdmin() {
		return domain.ErrForbidden
	}
	if err := requireBranch(tenant, "a WhatsApp channel"); err != nil {
		return err
	}
	if s.provider == nil || !s.channels.sealer.Enabled() {
		return domain.ErrNotConfigured
	}
	if strings.TrimSpace(code) == "" || len(code) > 4096 || !metaSignupID(accountID) || !metaSignupID(phoneID) {
		return fmt.Errorf("%w: invalid Meta signup response", domain.ErrInvalidInput)
	}
	return s.db.WithAdvisoryQuerier(ctx, "whatsapp-signup:"+phoneID, func(ownerQuery repository.Querier) error {
		var current *domain.Channel
		if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			current, err = s.store.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, channelID)
			if err == nil && current.Type != domain.ChannelTypeWhatsApp {
				return domain.ErrNotFound
			}
			return err
		}); err != nil {
			return err
		}
		owners, err := s.store.GetActiveByTypeAndIdentifiersCrossAccount(ctx, ownerQuery, domain.ChannelTypeWhatsApp, []string{phoneID})
		if err != nil {
			return err
		}
		if owner, ok := owners[phoneID]; ok && owner.ID != channelID {
			return domain.ErrConflict
		}

		grant, err := s.provider.Authorize(ctx, code, accountID, phoneID)
		if err != nil {
			return signupProviderError(err)
		}
		if err := domain.ValidateChannelIdentifier(domain.ChannelTypeWhatsApp, &grant.Identifier, true); err != nil {
			return err
		}
		active := true
		update := domain.WhatsAppChannelUpdate{Identifier: &grant.Identifier, IsActive: &active, PhoneNumberID: &phoneID, BusinessAccountID: &accountID, AccessToken: &grant.AccessToken}
		if err := update.Validate(); err != nil {
			return err
		}
		if err := s.provider.Subscribe(ctx, accountID, grant.AccessToken); err != nil {
			return signupProviderError(err)
		}
		if grant.NeedsRegistration {
			// A provider timeout may occur after registration has already succeeded.
			mac := hmac.New(sha256.New, []byte(s.secret))
			_, _ = mac.Write([]byte("whatsapp-registration:" + phoneID))
			pin := fmt.Sprintf("%06d", binary.BigEndian.Uint32(mac.Sum(nil))%1_000_000)
			if err := s.provider.Register(ctx, phoneID, grant.AccessToken, pin); err != nil {
				return signupProviderError(err)
			}
		}
		_, err = s.channels.updateWhatsAppChannel(ctx, tenant, channelID, update, &current.UpdatedAt)
		return err
	})
}

func metaSignupID(value string) bool {
	return value != "" && len(value) <= 32 && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func signupProviderError(err error) error {
	return fmt.Errorf("%w: WhatsApp signup: %v", domain.ErrDeliveryUnavailable, err)
}
