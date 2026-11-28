package services

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// maxTranscriptionBytes is the largest recording the transcription provider accepts.
const maxTranscriptionBytes int64 = 25 << 20

type whatsAppMediaChannelReader interface {
	GetActiveConfigurationsByIDs(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID, channelIDs []uuid.UUID) (map[uuid.UUID][]byte, error)
}

type whatsAppMediaAttachmentWriter interface {
	SetStoredFile(ctx context.Context, q repository.Querier, accountID, attachmentID uuid.UUID,
		storageKey string, kind domain.AttachmentType) error
}

// WhatsAppMediaSource brings a file a client sent by WhatsApp into storage the moment the sweep
// reads it. Meta keeps the file behind a media ID and hands out a download link that expires in
// minutes, so the link is asked for here rather than when the message arrived.
type WhatsAppMediaSource struct {
	db          tenantTxRunner
	channels    whatsAppMediaChannelReader
	attachments whatsAppMediaAttachmentWriter
	opener      channelConfigOpener
	fetcher     domain.WhatsAppMediaFetcher
	storage     domain.ObjectStorage
	maxFileSize int64
}

// NewWhatsAppMediaSource builds a WhatsAppMediaSource that refuses a file past maxFileSize.
func NewWhatsAppMediaSource(db tenantTxRunner, channels whatsAppMediaChannelReader,
	attachments whatsAppMediaAttachmentWriter, opener channelConfigOpener,
	fetcher domain.WhatsAppMediaFetcher, storage domain.ObjectStorage, maxFileSize int64,
) *WhatsAppMediaSource {
	return &WhatsAppMediaSource{db: db, channels: channels, attachments: attachments,
		opener: opener, fetcher: fetcher, storage: storage, maxFileSize: maxFileSize}
}

// Materialize downloads the file an attachment still names by media ID, stores it under the
// order's key and returns the attachment as the sweep reads any stored file. A file whose bytes
// are not an accepted format is refused, which closes the attachment out as failed.
func (s *WhatsAppMediaSource) Materialize(
	ctx context.Context, attachment domain.ClaimedAttachment,
) (domain.ClaimedAttachment, error) {
	mediaID := strings.TrimPrefix(attachment.StorageKey, domain.WhatsAppMediaKeyPrefix)
	tenant := domain.Tenant{AccountID: attachment.AccountID, BranchID: attachment.BranchID}
	token, err := s.accessToken(ctx, tenant, attachment.ChannelID)
	if err != nil {
		return attachment, err
	}
	limit := s.maxFileSize
	if attachment.Type == domain.AttachmentTypeAudio {
		limit = min(limit, maxTranscriptionBytes)
	}
	media, err := s.fetcher.FetchMedia(ctx, token, mediaID, limit)
	if err != nil {
		return attachment, err
	}
	contentType := normalizeContentType(media.MediaType)
	format, supported := domain.AttachmentFormatFor(contentType)
	if !supported {
		return attachment, fmt.Errorf("%w: WhatsApp media of type %q cannot be read as an order",
			domain.ErrInvalidInput, contentType)
	}
	if format.Type == domain.AttachmentTypeAudio && int64(len(media.Data)) > maxTranscriptionBytes {
		return attachment, fmt.Errorf("%w: recording is over the transcription limit",
			domain.ErrInvalidInput)
	}
	if err := refuseLegacyExcel(format, media.Data); err != nil {
		return attachment, err
	}
	key := attachmentKey(attachment.AccountID, attachment.RFQID, attachment.ID, format.Extension)
	if err := s.storage.Upload(ctx, key, contentType, bytes.NewReader(media.Data)); err != nil {
		return attachment, err
	}
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.attachments.SetStoredFile(ctx, q, attachment.AccountID, attachment.ID, key,
			format.Type)
	}); err != nil {
		return attachment, err
	}
	attachment.StorageKey, attachment.Type = key, format.Type
	return attachment, nil
}

func (s *WhatsAppMediaSource) accessToken(
	ctx context.Context, tenant domain.Tenant, channelID uuid.UUID,
) (string, error) {
	var raw map[uuid.UUID][]byte
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var readErr error
		raw, readErr = s.channels.GetActiveConfigurationsByIDs(ctx, q, tenant.AccountID,
			tenant.BranchID, []uuid.UUID{channelID})
		return readErr
	}); err != nil {
		return "", err
	}
	parsed, err := domain.ParseChannelConfig(domain.ChannelTypeWhatsApp, raw[channelID])
	if err != nil || parsed == nil {
		return "", fmt.Errorf("%w: the order's WhatsApp channel has no usable configuration",
			domain.ErrNotConfigured)
	}
	if err := parsed.MapSecrets(s.opener.Open); err != nil {
		return "", fmt.Errorf("%w: the order's WhatsApp credentials cannot be opened",
			domain.ErrNotConfigured)
	}
	config, ok := parsed.(*domain.WhatsAppChannelConfig)
	if !ok || config.AccessToken == "" {
		return "", fmt.Errorf("%w: the order's WhatsApp channel has no access token",
			domain.ErrNotConfigured)
	}
	return config.AccessToken, nil
}
