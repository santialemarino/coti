package services

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// quotePortalContactReader reports, by domain.ContactDestinationKey, which destinations a customer
// proved in a portal account.
type quotePortalContactReader interface {
	ListPortalContacts(ctx context.Context, q repository.Querier, accountID uuid.UUID, phones,
		emails []string) (map[string]bool, error)
}

// quotePDFLoader reads the stored PDF of a representation so an email can attach it.
type quotePDFLoader interface {
	LoadPDF(ctx context.Context, representation domain.QuoteRepresentation) ([]byte, error)
}

// quoteSendMessage is what one delivery says and where its link points.
type quoteSendMessage struct {
	// Body keeps the link as the placeholder: the token is a credential and is stored only once.
	Body        string
	Link        string
	Stage       quoteMessageStage
	Email       *quoteEmailContent
	Attachments []domain.EmailAttachment
	// Template replaces the WhatsApp text outside the customer-service window.
	Template *domain.WhatsAppTemplate
	// Refusal fails the send without calling the provider.
	Refusal error
}

// WithPortalContacts decides per destination between the order link and a sign-up invitation.
func (s *QuoteDeliveryService) WithPortalContacts(reader quotePortalContactReader) *QuoteDeliveryService {
	s.portalContacts = reader
	return s
}

// sendMessages renders each pending delivery's message: the order link for a contact proved in a
// portal account before this send, the summary with a sign-up invitation for anyone else.
func (s *QuoteDeliveryService) sendMessages(ctx context.Context, tenant domain.Tenant,
	sends []domain.QuoteSend, bundle *domain.QuoteRepresentationResult, followup *string, base string,
	validUntil time.Time,
) (map[uuid.UUID]quoteSendMessage, error) {
	var phones, emails []string
	var targets []whatsAppTarget
	for _, send := range sends {
		if send.TrackingStatus != domain.SendTrackingStatusPending {
			continue
		}
		switch send.ChannelType {
		case domain.ChannelTypeWhatsApp:
			phones = append(phones, send.Destination)
			targets = append(targets, whatsAppTarget{ChannelID: send.ChannelID, Destination: send.Destination})
		case domain.ChannelTypeEmail:
			emails = append(emails, strings.ToLower(send.Destination))
		}
	}
	portal := map[string]bool{}
	var closed map[whatsAppTarget]bool
	usePortal := followup == nil && s.portalContacts != nil && len(phones)+len(emails) > 0
	if template := s.whatsAppTemplate(followup != nil); usePortal || (template != "" && len(targets) > 0) {
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var err error
			if usePortal {
				if portal, err = s.portalContacts.ListPortalContacts(ctx, q, tenant.AccountID, phones, emails); err != nil {
					return err
				}
			}
			if template != "" {
				closed, err = s.closedWhatsAppWindows(ctx, q, tenant, targets)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	var attachments []domain.EmailAttachment
	if followup == nil && bundle != nil && (len(emails) > 0 || len(closed) > 0) {
		attachments = s.quotePDFAttachment(ctx, *bundle)
	}

	messages := make(map[uuid.UUID]quoteSendMessage, len(sends))
	for _, send := range sends {
		message := quoteSendMessage{Body: sellerSendBody, Link: send.PublicURL, Stage: quoteMessageOrderLink}
		window := closed[whatsAppTarget{ChannelID: send.ChannelID, Destination: send.Destination}]
		switch {
		case followup != nil:
			message.Body = *followup
			if window && bundle != nil {
				message = s.followupTemplateMessage(message, bundle.Representation.Payload.Supplier.Name, *followup)
			}
		case bundle != nil:
			if s.portalContacts != nil && !portal[domain.ContactDestinationKey(send.ChannelType, send.Destination)] {
				message.Stage = quoteMessageInvitation
				message.Link = quoteMessageLink(base, send.PublicToken, message.Stage)
			}
			payload := bundle.Representation.Payload
			message.Body = buildQuoteWhatsAppMessage(payload, &validUntil, message.Stage)
			if send.ChannelType == domain.ChannelTypeEmail {
				email := buildQuoteEmail(payload, &validUntil, message.Stage, len(attachments) > 0)
				message.Email = &email
				message.Attachments = attachments
			}
			if window {
				message = s.quoteTemplateMessage(message, payload, validUntil, attachments)
			}
		}
		messages[send.ID] = message
	}
	return messages, nil
}

// quotePDFAttachment loads the quotation's PDF. A missing file never blocks the sale: the email
// still carries the summary, and the failure is logged for the operator.
func (s *QuoteDeliveryService) quotePDFAttachment(ctx context.Context,
	bundle domain.QuoteRepresentationResult,
) []domain.EmailAttachment {
	loader, ok := s.representations.(quotePDFLoader)
	if !ok {
		return nil
	}
	content, err := loader.LoadPDF(ctx, bundle.Representation)
	if err != nil {
		s.log.WarnContext(ctx, "quote PDF unavailable; emailing without it",
			slog.String("quote_id", bundle.Representation.QuoteID.String()), slog.Any("error", err))
		return nil
	}
	return []domain.EmailAttachment{{
		Filename:    "Cotizacion-" + bundle.Representation.Payload.Reference + ".pdf",
		ContentType: quotePDFContentType, Content: content,
	}}
}
