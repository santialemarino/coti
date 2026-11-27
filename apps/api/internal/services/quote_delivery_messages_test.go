package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakePortalContacts struct {
	proved map[string]bool
	phones []string
	emails []string
}

func (f *fakePortalContacts) ListPortalContacts(_ context.Context, _ repository.Querier, _ uuid.UUID,
	phones, emails []string) (map[string]bool, error) {
	f.phones, f.emails = phones, emails
	return f.proved, nil
}

type fakePDFRepresentations struct {
	fakeQuoteDeliverRepresentations
	pdf []byte
	err error
}

func (f *fakePDFRepresentations) LoadPDF(context.Context, domain.QuoteRepresentation) ([]byte, error) {
	return f.pdf, f.err
}

func deliveryMessagesFixture(t *testing.T, proved map[string]bool, pdfErr error) (*QuoteDeliveryService,
	*fakePortalContacts, []domain.QuoteSend, *domain.QuoteRepresentationResult) {
	t.Helper()
	portal := &fakePortalContacts{proved: proved}
	service := &QuoteDeliveryService{db: &fakePublicQuoteDB{}, portalContacts: portal,
		representations: &fakePDFRepresentations{pdf: []byte("%PDF"), err: pdfErr}, log: slog.Default()}
	pending := domain.SendTrackingStatusPending
	sends := []domain.QuoteSend{
		{ID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp, Destination: "+5491111111111",
			PublicToken: "tok-proved", PublicURL: "https://c.test/quotes/tok-proved", TrackingStatus: pending},
		{ID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp, Destination: "+5491122222222",
			PublicToken: "tok-new", PublicURL: "https://c.test/quotes/tok-new", TrackingStatus: pending},
		{ID: uuid.New(), ChannelType: domain.ChannelTypeEmail, Destination: "Obra@Norte.test",
			PublicToken: "tok-mail", PublicURL: "https://c.test/quotes/tok-mail", TrackingStatus: pending},
	}
	bundle := &domain.QuoteRepresentationResult{Representation: domain.QuoteRepresentation{
		QuoteID: uuid.New(), Payload: quoteMessagePayload(2)}}
	return service, portal, sends, bundle
}

// Decision 92: each destination is judged on its own, by whether a customer proved it in a portal
// account before this send.
func TestQuoteDeliveryService_SendMessages_DecidesTheLinkPerDestination(t *testing.T) {
	t.Parallel()
	proved := map[string]bool{domain.ContactDestinationKey(domain.ChannelTypeWhatsApp, "+5491111111111"): true}
	service, portal, sends, bundle := deliveryMessagesFixture(t, proved, nil)
	validUntil := time.Date(2026, 12, 10, 15, 0, 0, 0, time.UTC)

	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, nil, "https://c.test", validUntil)
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	if len(portal.emails) != 1 || portal.emails[0] != "obra@norte.test" {
		t.Errorf("emails looked up = %q, want the lower-cased address", portal.emails)
	}

	provedMessage := messages[sends[0].ID]
	if provedMessage.Stage != quoteMessageOrderLink || provedMessage.Link != sends[0].PublicURL {
		t.Errorf("proved contact got stage %v and link %q, want the order link", provedMessage.Stage, provedMessage.Link)
	}
	newMessage := messages[sends[1].ID]
	if newMessage.Stage != quoteMessageInvitation || newMessage.Link != "https://c.test/signup?next=%2Fquotes%2Ftok-new" {
		t.Errorf("new contact got stage %v and link %q, want the sign-up invitation", newMessage.Stage, newMessage.Link)
	}
	for _, message := range []quoteSendMessage{provedMessage, newMessage} {
		if !strings.Contains(message.Body, "Válida hasta el 10/12/2026.") ||
			strings.Count(message.Body, domain.QuotePublicURLPlaceholder) != 1 {
			t.Errorf("body does not state the validity with one placeholder:\n%s", message.Body)
		}
		if message.Email != nil || len(message.Attachments) != 0 {
			t.Errorf("a WhatsApp message carries email content")
		}
	}
	mail := messages[sends[2].ID]
	if mail.Email == nil || mail.Email.ActionLabel != "Crear mi cuenta" {
		t.Fatalf("email content = %+v, want an invitation", mail.Email)
	}
	if len(mail.Attachments) != 1 || mail.Attachments[0].Filename != "Cotizacion-COT-000042.pdf" ||
		mail.Attachments[0].ContentType != "application/pdf" {
		t.Errorf("email attachments = %+v, want the quote PDF", mail.Attachments)
	}
}

// A PDF that cannot be read never blocks the sale: the email goes out with the summary only.
func TestQuoteDeliveryService_SendMessages_EmailsWithoutAnUnreadablePDF(t *testing.T) {
	t.Parallel()
	service, _, sends, bundle := deliveryMessagesFixture(t, nil, errors.New("storage down"))
	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, nil, "https://c.test", time.Now())
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	mail := messages[sends[2].ID]
	if len(mail.Attachments) != 0 || strings.Contains(strings.Join(mail.Email.Closing, " "), "adjuntamos") {
		t.Errorf("email promises or carries a PDF it could not read: %+v", mail)
	}
}

// A follow-up keeps the seller's own text and the order link; it is not a first contact.
func TestQuoteDeliveryService_SendMessages_KeepsAFollowupAsWritten(t *testing.T) {
	t.Parallel()
	service, portal, sends, bundle := deliveryMessagesFixture(t, nil, nil)
	followup := "¿Pudiste ver la cotización? " + domain.QuotePublicURLPlaceholder
	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, &followup, "https://c.test", time.Now())
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	if portal.phones != nil {
		t.Errorf("a follow-up looked up portal contacts")
	}
	for _, send := range sends {
		message := messages[send.ID]
		if message.Body != followup || message.Link != send.PublicURL || len(message.Attachments) != 0 {
			t.Errorf("follow-up message = %+v", message)
		}
	}
}
