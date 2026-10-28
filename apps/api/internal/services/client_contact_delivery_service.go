package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type contactChannelStore interface {
	GetContactMailbox(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.ContactMailbox, error)
	GetMessagingConfiguration(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, domain.ChannelType) (*domain.ChannelConfiguration, error)
}

// ClientContactDeliveryService delivers deterministic identity messages outside ingestion transactions.
type ClientContactDeliveryService struct {
	db       tenantTxRunner
	store    contactChannelStore
	mail     domain.Mailer
	whatsapp domain.ContactWhatsAppSender
	opener   channelConfigOpener
	cfg      config.CustomerAuthConfig
}

// NewClientContactDeliveryService wires the configured channel transports for onboarding and proofs.
func NewClientContactDeliveryService(db tenantTxRunner, store contactChannelStore, mail domain.Mailer, whatsapp domain.ContactWhatsAppSender, opener channelConfigOpener, cfg config.CustomerAuthConfig) *ClientContactDeliveryService {
	return &ClientContactDeliveryService{db: db, store: store, mail: mail, whatsapp: whatsapp, opener: opener, cfg: cfg}
}

// SendContact sends a fixed backend message through this supplier's contact channel.
func (s *ClientContactDeliveryService) SendContact(ctx context.Context, message domain.ContactMessage) (string, error) {
	if message.AccountID == uuid.Nil {
		return "", domain.ErrForbidden
	}
	if message.Kind == domain.ChannelTypeEmail {
		return "", s.sendEmail(ctx, message)
	}
	if message.Kind != domain.ChannelTypeWhatsApp || s.whatsapp == nil || s.opener == nil {
		return "", domain.ErrNotConfigured
	}
	var stored *domain.ChannelConfiguration
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: message.AccountID}, func(q repository.Querier) error {
		var err error
		stored, err = s.store.GetMessagingConfiguration(ctx, q, message.AccountID, message.BranchID, message.ChannelID, domain.ChannelTypeWhatsApp)
		return err
	})
	if err != nil {
		return "", err
	}
	parsed, err := domain.ParseChannelConfig(domain.ChannelTypeWhatsApp, stored.Config)
	if err != nil {
		return "", err
	}
	if err := parsed.MapSecrets(s.opener.Open); err != nil {
		return "", domain.ErrNotConfigured
	}
	credentials, ok := parsed.(*domain.WhatsAppChannelConfig)
	if !ok {
		return "", domain.ErrNotConfigured
	}
	receipt, err := s.whatsapp.SendContact(ctx, domain.ContactWhatsAppMessage{To: message.To, Body: message.Body, Code: message.Code, Template: s.cfg.ContactCodeTemplate, Language: s.cfg.ContactCodeLanguage, Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: credentials.PhoneNumberID, AccessToken: credentials.AccessToken}})
	if err != nil {
		return "", err
	}
	if receipt == nil {
		return "", domain.ErrNotConfigured
	}
	return receipt.ProviderReference, nil
}

// sendEmail answers a name prompt as the supplier's branch mailbox; customer proofs keep the
// platform sender because they do not answer anything the customer wrote to a branch.
func (s *ClientContactDeliveryService) sendEmail(ctx context.Context, message domain.ContactMessage) error {
	if s.mail == nil {
		return domain.ErrNotConfigured
	}
	email := domain.EmailMessage{To: message.To, TextBody: message.Body}
	switch message.Purpose {
	case domain.ContactPurposeVerification:
		email.Subject = contactVerificationSubject
	case domain.ContactPurposeRegistrationLink:
		email.Subject = contactRegistrationLinkSubject
	case domain.ContactPurposeNameRequest:
		var mailbox *domain.ContactMailbox
		err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: message.AccountID}, func(q repository.Querier) error {
			var err error
			mailbox, err = s.store.GetContactMailbox(ctx, q, message.AccountID, message.BranchID, message.ChannelID)
			return err
		})
		if err != nil {
			return err
		}
		email.Subject = contactNamePromptSubject(mailbox.SupplierName)
		email.FromName = mailbox.SupplierName
		email.ReplyTo = mailbox.Address
		email.ReplyToName = mailbox.SupplierName + " — " + mailbox.BranchName
		email.InReplyTo = message.InReplyTo
	default:
		return domain.ErrInvalidInput
	}
	return s.mail.Send(ctx, email)
}
