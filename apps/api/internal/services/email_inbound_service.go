package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type emailInboundDB interface {
	tenantTxRunner
	WithAdvisoryLock(context.Context, string, func() error) error
}

type emailInboundMessages interface {
	FindByExternalID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID,
		string) (*domain.InboundChannelMessage, error)
	Reserve(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID,
		domain.NewInboundChannelMessage) (*domain.InboundChannelMessage, error)
}

type emailInboundRFQs interface {
	Create(context.Context, repository.Querier, uuid.UUID, domain.NewRFQ) (*domain.RFQ, error)
}

type emailInboundAttachments interface {
	CreateInboundBatch(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID,
		[]domain.NewRetainedAttachment) error
}

type emailInboundChannels interface {
	GetActiveByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID,
		uuid.UUID) (*domain.Channel, error)
	GetActiveByIDForShare(context.Context, repository.Querier, uuid.UUID, uuid.UUID,
		uuid.UUID) (*domain.Channel, error)
}

// EmailInboundService durably records an email and its original files for the RFQ pipeline.
type EmailInboundService struct {
	contacts    inboundContactRecorder
	db          emailInboundDB
	messages    emailInboundMessages
	rfqs        emailInboundRFQs
	attachments emailInboundAttachments
	channels    emailInboundChannels
	storage     domain.ObjectStorage
	maxFileSize int64
	now         func() time.Time
	onRecorded  func()
}

// WithRecordedCallback wakes a worker after an incoming email is persisted.
func (s *EmailInboundService) WithRecordedCallback(callback func()) *EmailInboundService {
	s.onRecorded = callback
	return s
}

// NewEmailInboundService builds an inbound email recorder.
func NewEmailInboundService(db emailInboundDB, messages emailInboundMessages,
	rfqs emailInboundRFQs, attachments emailInboundAttachments, channels emailInboundChannels,
	storage domain.ObjectStorage, maxFileSize int64,
) *EmailInboundService {
	return &EmailInboundService{db: db, messages: messages, rfqs: rfqs, attachments: attachments,
		channels: channels, storage: storage, maxFileSize: maxFileSize, now: time.Now}
}

// Ingest records a provider message once and returns its RFQ for processing.
func (s *EmailInboundService) Ingest(ctx context.Context, route domain.InboundChannelRoute,
	message domain.InboundEmailMessage,
) (uuid.UUID, error) {
	if err := route.Validate(); err != nil {
		return uuid.Nil, err
	}
	if route.ChannelType != domain.ChannelTypeEmail || s.db == nil || s.messages == nil ||
		s.rfqs == nil || s.attachments == nil || s.channels == nil || s.storage == nil || s.maxFileSize <= 0 {
		return uuid.Nil, fmt.Errorf("%w: email intake is not fully wired", domain.ErrNotConfigured)
	}
	message.ExternalMessageID = strings.TrimSpace(message.ExternalMessageID)
	destination, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeEmail,
		message.DestinationIdentifier)
	if err != nil {
		return uuid.Nil, err
	}
	if message.ExternalMessageID == "" || len(message.ExternalMessageID) > maxInboundProviderIdentifierLength ||
		len(message.Original) == 0 {
		return uuid.Nil, fmt.Errorf("%w: email ID and original message are required", domain.ErrInvalidInput)
	}
	var rfqID uuid.UUID
	// opened is the order this mail started, which is owed the receipt acknowledgement.
	var opened *uuid.UUID
	lock := "email-inbound:" + route.ChannelID.String() + ":" + message.ExternalMessageID
	err = s.db.WithAdvisoryLock(ctx, lock, func() error {
		var findErr error
		if err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			channel, channelErr := s.channels.GetActiveByID(ctx, q, route.AccountID, route.BranchID,
				route.ChannelID)
			if channelErr != nil {
				return channelErr
			}
			if !matchesEmailChannel(channel, destination) {
				return fmt.Errorf("%w: email channel no longer matches the mailbox", domain.ErrInvalidInput)
			}
			var found *domain.InboundChannelMessage
			found, findErr = s.messages.FindByExternalID(ctx, q, route.AccountID, route.BranchID,
				route.ChannelID, message.ExternalMessageID)
			if findErr == nil && found.RFQID != nil {
				rfqID = *found.RFQID
			}
			if errors.Is(findErr, domain.ErrNotFound) {
				return nil
			}
			return findErr
		}); err != nil {
			return err
		}
		if findErr == nil {
			return nil
		}
		// An auto-reply or a newsletter is kept for the seller but never opens an order.
		human := !message.Automated && !automatedEmailSender(message.SenderID)
		if !human {
			return s.storeAutomated(ctx, route, message)
		}
		var nameReply *string
		if resolver, ok := s.contacts.(inboundNameReplyResolver); ok && len(message.Attachments) == 0 {
			if err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
				pending, name, err := resolver.NameReply(ctx, q, route, domain.ChannelTypeEmail, message.SenderID, emailReplyText(message.Body))
				if err == nil && pending != nil && pending.RFQID != nil {
					rfqID = *pending.RFQID
					nameReply = &name
				}
				return err
			}); err != nil {
				return err
			}
		}
		if nameReply == nil {
			rfqID = uuid.New()
			opened = &rfqID
		}
		if err := s.storeOne(ctx, route, rfqID, message, nameReply); err != nil {
			return err
		}
		if s.onRecorded != nil {
			s.onRecorded()
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	if completer, ok := s.contacts.(inboundContactCompleter); ok {
		reply := domain.ContactReplyContext{Automated: message.Automated || automatedEmailSender(message.SenderID),
			MessageID: message.InternetMessageID, ReceivedRFQ: opened}
		if err := completer.CompleteInbound(ctx, route, domain.ChannelTypeEmail, message.SenderID, reply); err != nil {
			return uuid.Nil, err
		}
	}
	return rfqID, nil
}

// storeOne retains the message under rfqID; a recognized name reply joins the pending request
// instead of opening one.
func (s *EmailInboundService) storeOne(ctx context.Context, route domain.InboundChannelRoute,
	rfqID uuid.UUID, message domain.InboundEmailMessage, nameReply *string,
) error {
	reuse := nameReply != nil
	files := make([]domain.NewRetainedAttachment, 0, len(message.Attachments)+2)
	addFile := func(filename, contentType, extension string, kind domain.AttachmentType,
		status domain.AttachmentProcessingStatus, data []byte) error {
		id := uuid.New()
		key := attachmentKey(route.AccountID, rfqID, id, extension)
		if err := s.storage.Upload(ctx, key, contentType, bytes.NewReader(data)); err != nil {
			return err
		}
		name := safeEmailText(strings.TrimSpace(filename))
		if utf8.RuneCountInString(name) > 255 {
			name = string([]rune(name)[:255])
		}
		var originalName *string
		if name != "" {
			originalName = &name
		}
		files = append(files, domain.NewRetainedAttachment{ID: id, Type: kind, StorageKey: key,
			OriginalFilename: originalName, Status: status})
		return nil
	}
	if err := addFile("original.eml", "application/octet-stream", "eml", domain.AttachmentTypeOther,
		domain.AttachmentProcessingDone, message.Original); err != nil {
		return err
	}
	record, err := s.inboundRecord(message)
	if err != nil {
		return err
	}
	text := record.Body
	if text != "" && !reuse {
		if err := addFile("email.txt", "text/plain", "txt", domain.AttachmentTypeText,
			domain.AttachmentProcessingPending, []byte(text)); err != nil {
			return err
		}
	}
	for _, attachment := range message.Attachments {
		contentType := normalizeContentType(attachment.ContentType)
		format, supported := domain.AttachmentFormatFor(contentType)
		status := domain.AttachmentProcessingPending
		if !supported || attachment.Inline || len(attachment.Content) == 0 ||
			int64(len(attachment.Content)) > s.maxFileSize ||
			refuseLegacyExcel(format, attachment.Content) != nil {
			format = domain.AttachmentFormat{Type: domain.AttachmentTypeOther, Extension: "bin"}
			status = domain.AttachmentProcessingFailed
			if attachment.Inline {
				status = domain.AttachmentProcessingDone
			}
			contentType = "application/octet-stream"
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if err := addFile(attachment.Filename, contentType, format.Extension, format.Type,
			status, attachment.Content); err != nil {
			return err
		}
	}
	status := domain.RFQStatusFailed
	for _, file := range files {
		if file.Status == domain.AttachmentProcessingPending {
			status = domain.RFQStatusReceived
			break
		}
	}
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		channel, err := s.channels.GetActiveByIDForShare(ctx, q, route.AccountID, route.BranchID, route.ChannelID)
		if err != nil {
			return err
		}
		destination, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeEmail,
			message.DestinationIdentifier)
		if err != nil || !matchesEmailChannel(channel, destination) {
			return fmt.Errorf("%w: email channel no longer matches the mailbox", domain.ErrInvalidInput)
		}
		if !reuse {
			if _, err := s.rfqs.Create(ctx, q, route.AccountID, domain.NewRFQ{
				ID: &rfqID, BranchID: route.BranchID, ChannelID: route.ChannelID,
				RawText: &text, ClientLabel: record.SenderLabel, Status: status,
			}); err != nil {
				return err
			}
		}
		record.RFQID = &rfqID
		reserved, err := s.messages.Reserve(ctx, q, route.AccountID, route.BranchID, route.ChannelID,
			record)
		if err != nil {
			return err
		}
		if reserved.RFQID == nil || *reserved.RFQID != rfqID {
			return errInboundAlreadyReserved
		}
		if s.contacts != nil {
			hints := channelClientData(message.Body, &message.SenderLabel)
			if nameReply != nil {
				hints.Name = nameReply
			}
			if err := s.contacts.RecordInbound(ctx, q, route, reserved.ID, domain.ChannelTypeEmail, record.SenderID, text, hints); err != nil {
				return err
			}
		}
		return s.attachments.CreateInboundBatch(ctx, q, route.AccountID, route.BranchID, rfqID, files)
	})
}

// storeAutomated retains an automated mail as a non-order, with no order, file or contact record.
func (s *EmailInboundService) storeAutomated(ctx context.Context, route domain.InboundChannelRoute,
	message domain.InboundEmailMessage,
) error {
	record, err := s.inboundRecord(message)
	if err != nil {
		return err
	}
	kind := domain.InboundMessageAutomated
	record.Kind = &kind
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		_, err := s.messages.Reserve(ctx, q, route.AccountID, route.BranchID, route.ChannelID, record)
		return err
	})
}

// inboundRecord is the message as retained: subject and body as one text, and a sender that is a
// bare address or nothing.
func (s *EmailInboundService) inboundRecord(
	message domain.InboundEmailMessage,
) (domain.NewInboundChannelMessage, error) {
	body := safeEmailText(strings.TrimSpace(message.Body))
	subject := safeEmailText(strings.TrimSpace(message.Subject))
	text := body
	if subject != "" {
		text = subject
		if body != "" {
			text += "\n\n" + body
		}
	}
	sender := strings.TrimSpace(message.SenderID)
	if parsed, err := mail.ParseAddress(sender); err != nil || parsed.Address != sender ||
		len(sender) > maxInboundProviderIdentifierLength {
		sender = ""
	}
	label := safeEmailText(strings.TrimSpace(message.SenderLabel))
	if utf8.RuneCountInString(label) > 255 {
		label = string([]rune(label)[:255])
	}
	var senderLabel *string
	if label != "" {
		senderLabel = &label
	}
	receivedAt := message.ProviderReceivedAt
	if receivedAt.IsZero() {
		receivedAt = s.now().UTC()
	}
	payload, err := json.Marshal(map[string]string{"provider": "gmail", "subject": subject})
	if err != nil {
		return domain.NewInboundChannelMessage{}, err
	}
	return domain.NewInboundChannelMessage{ID: uuid.New(), ExternalMessageID: message.ExternalMessageID,
		SenderID: sender, SenderLabel: senderLabel, Body: text, Payload: payload,
		ProviderReceivedAt: receivedAt}, nil
}

func matchesEmailChannel(channel *domain.Channel, destination string) bool {
	if channel == nil || channel.Type != domain.ChannelTypeEmail || channel.Identifier == nil {
		return false
	}
	identifier, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeEmail, *channel.Identifier)
	return err == nil && identifier == destination
}

func safeEmailText(value string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(value, "\uFFFD"), "\x00", "\uFFFD")
}
