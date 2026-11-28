package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// Every contact mail but the name prompt is a branded message from the supplier: a heading, its
// copy and, when it carries a link, a button with the URL repeated for a client that drops it.
func TestClientContactDeliveryService_SendContact_BrandsEveryCustomerMail(t *testing.T) {
	t.Parallel()
	const link = "https://portal.test/c/san-martin/verify-contact?challenge=abc"
	cases := []struct {
		name    string
		message domain.ContactMessage
		subject string
		text    []string
	}{
		{
			name:    "verification",
			message: domain.ContactMessage{Purpose: domain.ContactPurposeVerification, Link: link},
			subject: contactVerificationSubject,
			text:    []string{contactVerificationHeading, "Corralón San Martín", contactVerificationAction + ": " + link},
		},
		{
			name:    "recovery",
			message: domain.ContactMessage{Purpose: domain.ContactPurposeRecovery, Code: "123456"},
			subject: contactRecoverySubject,
			text:    []string{contactRecoveryHeading, "es 123456.", contactRecoveryIgnore},
		},
		{
			name:    "registration link",
			message: domain.ContactMessage{Purpose: domain.ContactPurposeRegistrationLink, Link: link},
			subject: contactRegistrationLinkSubject,
			text:    []string{contactRegistrationLinkHeading, contactRegistrationLinkAction + ": " + link},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transport := &mailTestTransport{}
			service := NewClientContactDeliveryService(mailTestDB{}, nil, mailTestAccounts{}, transport,
				nil, nil, config.CustomerAuthConfig{})
			message := tc.message
			message.AccountID, message.Kind, message.To = uuid.New(), domain.ChannelTypeEmail, "cliente@example.test"

			if _, err := service.SendContact(context.Background(), message); err != nil {
				t.Fatalf("SendContact() = %v, want no error", err)
			}
			if len(transport.sent) != 1 {
				t.Fatalf("sent %d mails, want 1", len(transport.sent))
			}
			mail := transport.sent[0]
			if mail.Subject != tc.subject || mail.FromName != "Corralón San Martín" || mail.To != message.To {
				t.Errorf("mail = subject %q from %q to %q", mail.Subject, mail.FromName, mail.To)
			}
			if mail.HTMLBody == "" || !strings.Contains(mail.HTMLBody, tc.text[0]) {
				t.Errorf("HTML body = %q, want the branded layout with %q", mail.HTMLBody, tc.text[0])
			}
			for _, want := range tc.text {
				if !strings.Contains(mail.TextBody, want) {
					t.Errorf("text body = %q, want it to carry %q", mail.TextBody, want)
				}
			}
		})
	}
}

// A link mail with no link, or a recovery mail with no code, would arrive with nothing to act on.
func TestClientContactDeliveryService_SendContact_RefusesAMailWithNothingToActOn(t *testing.T) {
	t.Parallel()
	for _, purpose := range []domain.ContactPurpose{
		domain.ContactPurposeVerification, domain.ContactPurposeRecovery, domain.ContactPurposeRegistrationLink, "",
	} {
		transport := &mailTestTransport{}
		service := NewClientContactDeliveryService(mailTestDB{}, nil, mailTestAccounts{}, transport,
			nil, nil, config.CustomerAuthConfig{})
		_, err := service.SendContact(context.Background(), domain.ContactMessage{
			AccountID: uuid.New(), Purpose: purpose, Kind: domain.ChannelTypeEmail, To: "cliente@example.test",
		})
		if !errors.Is(err, domain.ErrInvalidInput) || len(transport.sent) != 0 {
			t.Errorf("purpose %q: err = %v, sent %d, want ErrInvalidInput and nothing sent", purpose, err, len(transport.sent))
		}
	}
}

type fakeContactChannels struct{}

func (fakeContactChannels) GetContactMailbox(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.ContactMailbox, error) {
	return &domain.ContactMailbox{Address: "ventas@moron.test", SupplierName: "Corralón San Martín", BranchName: "Morón"}, nil
}

func (fakeContactChannels) GetMessagingConfiguration(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, domain.ChannelType) (*domain.ChannelConfiguration, error) {
	return nil, domain.ErrNotConfigured
}

func TestClientContactDelivery_AcknowledgementThreadsAsAnAutomaticReply(t *testing.T) {
	mailer := &mailTestTransport{}
	service := NewClientContactDeliveryService(mailTestDB{}, fakeContactChannels{}, nil, mailer, nil, nil,
		config.CustomerAuthConfig{})

	_, err := service.SendContact(context.Background(), domain.ContactMessage{
		AccountID: testAccountID, BranchID: testBranchID, ChannelID: testChannelID,
		Purpose: domain.ContactPurposeAcknowledgement, Kind: domain.ChannelTypeEmail,
		To: "compras@obra.test", Body: orderAcknowledgement, InReplyTo: "<order@mail.test>",
	})
	if err != nil {
		t.Fatalf("SendContact() = %v", err)
	}
	if len(mailer.sent) != 1 {
		t.Fatalf("sent %d mails, want one", len(mailer.sent))
	}
	mail := mailer.sent[0]
	if !mail.AutoReplied || mail.InReplyTo != "<order@mail.test>" || mail.ReplyTo != "ventas@moron.test" ||
		mail.TextBody != orderAcknowledgement {
		t.Errorf("mail = %+v, want the acknowledgement as an automatic reply in the client's thread", mail)
	}
}
