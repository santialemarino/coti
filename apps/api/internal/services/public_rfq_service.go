package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type publicRFQDB interface {
	tenantTxRunner
	CrossAccount() repository.Querier
	WithAdvisoryLock(context.Context, string, func() error) error
}

type publicRFQDestinations interface {
	GetSiteCrossAccount(context.Context, repository.Querier, string) (*domain.PublicRFQSite, error)
	GetDestinationCrossAccount(context.Context, repository.Querier, uuid.UUID) (*domain.PublicRFQDestination, error)
}

type publicRFQSourceStore interface {
	Create(context.Context, repository.Querier, uuid.UUID, domain.NewRFQ) (*domain.RFQ, error)
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.RFQ, error)
}

type publicRFQFiles interface {
	StorePendingForRFQ(context.Context, domain.Tenant, uuid.UUID, domain.AttachmentUpload, []byte) (uuid.UUID, error)
}

type publicRFQAttachmentReader interface {
	ListByRFQ(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.RFQAttachment, error)
}

type publicRFQProcessor interface {
	CreateInboundTextDraftForMessage(context.Context, domain.InboundChannelRoute, domain.InboundTextRFQDraftInput, uuid.UUID) (*domain.TextRFQDraft, error)
}

type publicRFQEnvelope struct {
	Contact     domain.PublicRFQContact `json:"contact"`
	Fingerprint string                  `json:"fingerprint"`
}

// PublicRFQService retains customer requests through an active branch WEBAPP channel.
type PublicRFQService struct {
	contacts     customerContactRecorder
	db           publicRFQDB
	destinations publicRFQDestinations
	messages     whatsAppInboundMessageStore
	sources      publicRFQSourceStore
	files        publicRFQFiles
	attachments  publicRFQAttachmentReader
	processor    publicRFQProcessor
	cfg          config.RFQConfig
	maxFileSize  int64
}

// NewPublicRFQService builds the public intake over the existing RFQ and attachment pipelines.
func NewPublicRFQService(db publicRFQDB, destinations publicRFQDestinations,
	messages whatsAppInboundMessageStore, sources publicRFQSourceStore, files publicRFQFiles,
	attachments publicRFQAttachmentReader, processor publicRFQProcessor,
	cfg config.RFQConfig, maxFileSize int64,
) *PublicRFQService {
	return &PublicRFQService{db: db, destinations: destinations, messages: messages, sources: sources,
		files: files, attachments: attachments, processor: processor, cfg: cfg, maxFileSize: maxFileSize}
}

// GetDestination returns only the supplier identity of an active public intake channel.
func (s *PublicRFQService) GetDestination(ctx context.Context, channelID uuid.UUID) (*domain.PublicRFQDestination, error) {
	if channelID == uuid.Nil {
		return nil, domain.ErrNotFound
	}
	destination, err := s.destinations.GetDestinationCrossAccount(ctx, s.db.CrossAccount(), channelID)
	if err != nil {
		return nil, err
	}
	destination.MaxTextCharacters = s.cfg.MaxTextCharacters
	destination.MaxFileSize = s.maxFileSize
	destination.AcceptedContentTypes = domain.AcceptedAttachmentContentTypes()
	return destination, nil
}

// Create retains customer declarations and acknowledges only a durably stored request.
func (s *PublicRFQService) Create(ctx context.Context, channelID uuid.UUID, in domain.PublicRFQInput) error {
	destination, err := s.GetDestination(ctx, channelID)
	if err != nil {
		return err
	}
	in.Contact, err = in.Contact.Normalize()
	if err != nil {
		return err
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.RequestID == uuid.Nil || in.RequestID.Version() != 4 || (in.File == nil) == (in.Text == "") {
		return fmt.Errorf("%w: a request ID and exactly one input mode are required", domain.ErrInvalidInput)
	}
	if err := requireMaxRunes(in.Text, "text", s.cfg.MaxTextCharacters); err != nil {
		return err
	}
	var data []byte
	if in.File != nil {
		in.File.ContentType = normalizeContentType(in.File.ContentType)
		format, ok := domain.AttachmentFormatFor(in.File.ContentType)
		if !ok {
			return domain.WithCode(domain.CodeUnsupportedFileType, domain.ErrInvalidInput)
		}
		data, err = readUpload(*in.File, s.maxFileSize)
		if err != nil {
			return err
		}
		if err := refuseLegacyExcel(format, data); err != nil {
			return err
		}
		if len(in.Filename) > 255 {
			return domain.ErrInvalidInput
		}
	}
	payload, err := publicRFQPayload(in, data)
	if err != nil {
		return err
	}
	route := destination.Route
	return s.db.WithAdvisoryLock(ctx, "public-rfq:"+channelID.String()+":"+in.RequestID.String(), func() error {
		var reserved *domain.InboundChannelMessage
		err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			rfqID := uuid.New()
			var reserveErr error
			reserved, reserveErr = s.messages.Reserve(ctx, q, route.AccountID, route.BranchID, route.ChannelID,
				domain.NewInboundChannelMessage{ID: uuid.New(), RFQID: &rfqID,
					ExternalMessageID: in.RequestID.String(), SenderID: in.Contact.Email,
					SenderLabel: &in.Contact.Name, Body: in.Text, Payload: payload, ProviderReceivedAt: time.Now()})
			return reserveErr
		})
		if err != nil {
			return err
		}
		var original, current publicRFQEnvelope
		if err := json.Unmarshal(reserved.Payload, &original); err != nil {
			return err
		}
		if err := json.Unmarshal(payload, &current); err != nil {
			return err
		}
		if original.Fingerprint != current.Fingerprint {
			return domain.ErrConflict
		}
		if reserved.RFQID == nil {
			return domain.ErrConflict
		}
		return s.acceptReserved(ctx, route, *reserved.RFQID, in, data)
	})
}

func (s *PublicRFQService) acceptReserved(ctx context.Context, route domain.InboundChannelRoute,
	rfqID uuid.UUID, in domain.PublicRFQInput, data []byte,
) error {
	var source *domain.RFQ
	var files []domain.RFQAttachment
	err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		var readErr error
		source, readErr = s.sources.GetByID(ctx, q, route.AccountID, route.BranchID, rfqID)
		if readErr != nil && !errors.Is(readErr, domain.ErrNotFound) {
			return readErr
		}
		if source == nil {
			raw := in.Text
			if in.File != nil {
				raw = fileOrderPlaceholder(in.Filename)
			}
			source, readErr = s.sources.Create(ctx, q, route.AccountID, domain.NewRFQ{ID: &rfqID,
				BranchID: route.BranchID, ChannelID: route.ChannelID, RawText: &raw, ClientLabel: &in.Contact.Name})
			if readErr != nil {
				return readErr
			}
		}
		if in.File != nil {
			files, readErr = s.attachments.ListByRFQ(ctx, q, route.AccountID, route.BranchID, rfqID)
		}
		return readErr
	})
	if err != nil {
		return err
	}
	if in.File != nil {
		if len(files) > 0 {
			return nil
		}
		_, err := s.files.StorePendingForRFQ(ctx, route.Tenant(), rfqID, *in.File, data)
		return err
	}
	if source.Status != domain.RFQStatusReceived {
		return nil
	}
	_, processErr := s.processor.CreateInboundTextDraftForMessage(ctx, route,
		domain.InboundTextRFQDraftInput{ClientLabel: &in.Contact.Name, RawText: in.Text}, rfqID)
	if processErr == nil {
		return nil
	}
	// A processing failure does not revoke receipt of an order already kept for seller review.
	return s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		_, err := s.sources.GetByID(ctx, q, route.AccountID, route.BranchID, rfqID)
		return err
	})
}

func publicRFQPayload(in domain.PublicRFQInput, data []byte) ([]byte, error) {
	metadata, err := json.Marshal(struct {
		Contact     domain.PublicRFQContact
		Text        string
		Filename    string
		ContentType string
	}{Contact: in.Contact, Text: in.Text, Filename: in.Filename, ContentType: publicFileContentType(in.File)})
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	_, _ = digest.Write(metadata)
	_, _ = digest.Write(data)
	return json.Marshal(publicRFQEnvelope{Contact: in.Contact, Fingerprint: hex.EncodeToString(digest.Sum(nil))})
}

func publicFileContentType(file *domain.AttachmentUpload) string {
	if file == nil {
		return ""
	}
	return file.ContentType
}

// CreateForCustomer binds the declared CUIT/CUIL and branch to the authenticated supplier.
func (s *PublicRFQService) CreateForCustomer(ctx context.Context, p domain.CustomerPrincipal, channelID uuid.UUID, in domain.PublicRFQInput) error {
	destination, err := s.GetDestination(ctx, channelID)
	if err != nil {
		return err
	}
	if destination.Route.AccountID != p.AccountID {
		return domain.ErrNotFound
	}
	if domain.NormalizeTaxID(in.Contact.TaxID) != p.TaxID {
		return domain.ErrForbidden
	}
	in.Contact.TaxID = p.TaxID
	in.Contact, err = in.Contact.Normalize()
	if err != nil {
		return err
	}
	if err := s.Create(ctx, channelID, in); err != nil {
		return err
	}
	if s.contacts != nil {
		return s.contacts.RecordCustomer(ctx, p, destination.Route, in.RequestID, in.Contact)
	}
	return nil
}
