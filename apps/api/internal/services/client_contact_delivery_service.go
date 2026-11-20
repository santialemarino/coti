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
	accounts mailAccountReader
	mail     domain.Mailer
	whatsapp domain.ContactWhatsAppSender
	opener   channelConfigOpener
	cfg      config.CustomerAuthConfig
}

// NewClientContactDeliveryService wires the configured channel transports for onboarding and proofs.
func NewClientContactDeliveryService(db tenantTxRunner, store contactChannelStore, accounts mailAccountReader, mail domain.Mailer, whatsapp domain.ContactWhatsAppSender, opener channelConfigOpener, cfg config.CustomerAuthConfig) *ClientContactDeliveryService {
	return &ClientContactDeliveryService{db: db, store: store, accounts: accounts, mail: mail, whatsapp: whatsapp, opener: opener, cfg: cfg}
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

// sendEmail answers a name prompt as the supplier's branch mailbox, in plain text inside the
// customer's thread; every other contact mail goes out in the supplier's name and brand.
func (s *ClientContactDeliveryService) sendEmail(ctx context.Context, message domain.ContactMessage) error {
	if s.mail == nil {
		return domain.ErrNotConfigured
	}
	if message.Purpose == domain.ContactPurposeNameRequest {
		return s.sendNamePrompt(ctx, message)
	}
	if message.Purpose != domain.ContactPurposeVerification && message.Purpose != domain.ContactPurposeRecovery &&
		message.Purpose != domain.ContactPurposeRegistrationLink {
		return domain.ErrInvalidInput
	}
	var account *domain.Account
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: message.AccountID}, func(q repository.Querier) error {
		var err error
		account, err = s.accounts.GetByID(ctx, q, message.AccountID)
		return err
	})
	if err != nil {
		return err
	}
	out, err := contactMail(account.Name, message)
	if err != nil {
		return err
	}
	email, err := renderEmail(*account, out)
	if err != nil {
		return err
	}
	return s.mail.Send(ctx, email)
}

func (s *ClientContactDeliveryService) sendNamePrompt(ctx context.Context, message domain.ContactMessage) error {
	var mailbox *domain.ContactMailbox
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: message.AccountID}, func(q repository.Querier) error {
		var err error
		mailbox, err = s.store.GetContactMailbox(ctx, q, message.AccountID, message.BranchID, message.ChannelID)
		return err
	})
	if err != nil {
		return err
	}
	return s.mail.Send(ctx, domain.EmailMessage{
		To:          message.To,
		TextBody:    message.Body,
		Subject:     contactNamePromptSubject(mailbox.SupplierName),
		FromName:    mailbox.SupplierName,
		ReplyTo:     mailbox.Address,
		ReplyToName: mailbox.SupplierName + " — " + mailbox.BranchName,
		InReplyTo:   message.InReplyTo,
	})
}

// contactMail words a verification, recovery or registration mail for the supplier's customer.
func contactMail(supplier string, message domain.ContactMessage) (OutboundMail, error) {
	out := OutboundMail{AccountID: message.AccountID, To: message.To, FromName: supplier}
	switch message.Purpose {
	case domain.ContactPurposeVerification:
		out.Subject, out.Heading = contactVerificationSubject, contactVerificationHeading
		out.Paragraphs = []string{contactVerificationIntro(supplier), contactVerificationIgnore}
		out.ActionLabel, out.ActionURL = contactVerificationAction, message.Link
	case domain.ContactPurposeRecovery:
		if message.Code == "" {
			return OutboundMail{}, domain.ErrInvalidInput
		}
		out.Subject, out.Heading = contactRecoverySubject, contactRecoveryHeading
		out.Paragraphs = []string{contactRecoveryCodeIntro(supplier, message.Code), contactRecoveryIgnore}
		return out, nil
	case domain.ContactPurposeRegistrationLink:
		out.Subject, out.Heading = contactRegistrationLinkSubject, contactRegistrationLinkHeading
		out.Paragraphs = []string{contactRegistrationIntro(supplier), contactRegistrationSameAccount}
		out.ActionLabel, out.ActionURL = contactRegistrationLinkAction, message.Link
	default:
		return OutboundMail{}, domain.ErrInvalidInput
	}
	if out.ActionURL == "" {
		return OutboundMail{}, domain.ErrInvalidInput
	}
	return out, nil
}
