package services

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// mediaNotes is how a file reads where only text fits: the order's message and the conversation.
var mediaNotes = map[domain.InboundMediaKind]string{
	domain.InboundMediaImage:    "[Foto]",
	domain.InboundMediaAudio:    "[Audio]",
	domain.InboundMediaDocument: "[Documento]",
	domain.InboundMediaVideo:    "[Video]",
}

type whatsAppMediaRFQs interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewRFQ) (*domain.RFQ, error)
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID,
		rfqID uuid.UUID) (*domain.RFQ, error)
}

type whatsAppMediaAttachments interface {
	CreateInboundBatch(ctx context.Context, q repository.Querier, accountID, branchID,
		rfqID uuid.UUID, attachments []domain.NewRetainedAttachment) error
}

// WhatsAppMediaIntake opens the order a file sent by WhatsApp starts. The webhook only records
// what Meta holds — the file is fetched and read by the attachment sweep — so Meta gets its answer
// at once and a recording's transcription never runs inside its request.
type WhatsAppMediaIntake struct {
	db          tenantTxRunner
	rfqs        whatsAppMediaRFQs
	attachments whatsAppMediaAttachments
	storage     domain.ObjectStorage
	onRecorded  func()
}

// NewWhatsAppMediaIntake builds a WhatsAppMediaIntake; onRecorded, if set, wakes the sweep.
func NewWhatsAppMediaIntake(db tenantTxRunner, rfqs whatsAppMediaRFQs,
	attachments whatsAppMediaAttachments, storage domain.ObjectStorage, onRecorded func(),
) *WhatsAppMediaIntake {
	return &WhatsAppMediaIntake{db: db, rfqs: rfqs, attachments: attachments, storage: storage,
		onRecorded: onRecorded}
}

// CreateMediaOrder stores the order under rfqID with the file as a pending attachment and the
// caption, if any, as a text one. A file no format accepts stays on the order as failed, so the
// seller sees an order that could not be read rather than nothing at all. A retried delivery
// finds the order already there and changes nothing.
func (m *WhatsAppMediaIntake) CreateMediaOrder(ctx context.Context,
	route domain.InboundChannelRoute, rfqID uuid.UUID, message domain.InboundTextMessage,
) error {
	media := message.Media
	files := make([]domain.NewRetainedAttachment, 0, 2)
	file := domain.NewRetainedAttachment{ID: uuid.New(), Type: domain.AttachmentTypeOther,
		Status: domain.AttachmentProcessingFailed}
	if name := strings.TrimSpace(media.Filename); name != "" {
		file.OriginalFilename = &name
	}
	if format, ok := domain.AttachmentFormatFor(normalizeContentType(media.MediaType)); ok {
		file.Type, file.Status = format.Type, domain.AttachmentProcessingPending
		file.StorageKey = domain.WhatsAppMediaKeyPrefix + media.ProviderID
	}
	files = append(files, file)
	if caption := strings.TrimSpace(message.Body); caption != "" {
		id := uuid.New()
		key := attachmentKey(route.AccountID, rfqID, id, "txt")
		if err := m.storage.Upload(ctx, key, "text/plain", bytes.NewReader([]byte(caption))); err != nil {
			return err
		}
		files = append(files, domain.NewRetainedAttachment{ID: id, Type: domain.AttachmentTypeText,
			StorageKey: key, Status: domain.AttachmentProcessingPending})
	}
	status := domain.RFQStatusFailed
	for _, f := range files {
		if f.Status == domain.AttachmentProcessingPending {
			status = domain.RFQStatusReceived
		}
	}
	text := mediaMessageText(message)
	err := m.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		if _, err := m.rfqs.GetByID(ctx, q, route.AccountID, route.BranchID, rfqID); err == nil {
			return errInboundAlreadyReserved
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if _, err := m.rfqs.Create(ctx, q, route.AccountID, domain.NewRFQ{
			ID: &rfqID, BranchID: route.BranchID, ChannelID: route.ChannelID, RawText: &text,
			ClientLabel: message.SenderLabel, Status: status,
		}); err != nil {
			return err
		}
		return m.attachments.CreateInboundBatch(ctx, q, route.AccountID, route.BranchID, rfqID, files)
	})
	if errors.Is(err, errInboundAlreadyReserved) {
		return nil
	}
	if err != nil {
		return err
	}
	if status == domain.RFQStatusReceived && m.onRecorded != nil {
		m.onRecorded()
	}
	return nil
}

// mediaMessageText is what a file reads as where only text fits: its caption after a note naming
// what was sent, since a caption alone ("esto para el lunes") hides that there was a file.
func mediaMessageText(message domain.InboundTextMessage) string {
	if message.Media == nil {
		return message.Body
	}
	note := mediaNotes[message.Media.Kind]
	if caption := strings.TrimSpace(message.Body); caption != "" {
		return note + " " + caption
	}
	return note
}
