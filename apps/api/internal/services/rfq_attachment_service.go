package services

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// rfqAttachmentRepo is the persistence this service needs.
type rfqAttachmentRepo interface {
	ListByRFQ(ctx context.Context, q repository.Querier, accountID, branchID, rfqID uuid.UUID) ([]domain.RFQAttachment, error)
	Create(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, in domain.NewRFQAttachment) (*domain.RFQAttachment, error)
}

// RFQAttachmentService stores the files an RFQ arrived with and hands back links to them.
type RFQAttachmentService struct {
	db          *repository.DB
	attachments rfqAttachmentRepo
	storage     domain.ObjectStorage
	cfg         config.StorageConfig
	now         func() time.Time
}

// NewRFQAttachmentService builds an RFQAttachmentService. A nil now means time.Now.
func NewRFQAttachmentService(
	db *repository.DB, attachments rfqAttachmentRepo, storage domain.ObjectStorage,
	cfg config.StorageConfig, now func() time.Time,
) *RFQAttachmentService {
	if now == nil {
		now = time.Now
	}
	return &RFQAttachmentService{db: db, attachments: attachments, storage: storage, cfg: cfg, now: now}
}

// UploadFile is one file offered for an RFQ. Size is what the transport reported, so it is
// checked before a single byte reaches storage.
type UploadFile struct {
	ContentType string
	Size        int64
	Content     io.Reader
}

// List returns the RFQ's attachments, each with a link that serves it until the link expires.
func (s *RFQAttachmentService) List(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID,
) ([]domain.AttachmentLink, error) {
	if err := requireBranch(tenant, "an RFQ's attachments"); err != nil {
		return nil, err
	}

	var stored []domain.RFQAttachment
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		stored, listErr = s.attachments.ListByRFQ(ctx, q, tenant.AccountID, tenant.BranchID, rfqID)
		return listErr
	})
	if err != nil {
		return nil, err
	}

	// Signing runs outside the transaction: the local adapter touches the filesystem and the
	// bucket one signs in memory, and neither belongs inside a database transaction.
	links := make([]domain.AttachmentLink, 0, len(stored))
	for _, attachment := range stored {
		link, err := s.linkFor(ctx, attachment)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

// Upload stores one file for an RFQ and records the reference. The file is refused for its type
// or its size before anything is stored; the object key leads with the account, so one account's
// file is not addressable from another. Returns domain.ErrNotFound when the RFQ is not this
// caller's.
func (s *RFQAttachmentService) Upload(
	ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID, file UploadFile,
) (*domain.AttachmentLink, error) {
	if err := requireBranch(tenant, "an RFQ's attachments"); err != nil {
		return nil, err
	}
	// Normalized once, so the format lookup and the type the object is stored with agree.
	file.ContentType = normalizeContentType(file.ContentType)
	format, err := s.acceptedFormat(file)
	if err != nil {
		return nil, err
	}

	attachmentID := uuid.New()
	key := attachmentKey(tenant.AccountID, rfqID, attachmentID, format.Extension)
	// Stored before the row exists, so a failed write leaves no reference pointing at nothing.
	// The reverse order would leave a link a screen renders and no object behind it.
	if err := s.storage.Upload(ctx, key, file.ContentType, file.Content); err != nil {
		return nil, err
	}

	var stored *domain.RFQAttachment
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		stored, createErr = s.attachments.Create(ctx, q, tenant.AccountID, tenant.BranchID,
			domain.NewRFQAttachment{
				ID:         attachmentID,
				RFQID:      rfqID,
				Type:       format.Type,
				StorageKey: key,
			})
		return createErr
	})
	if err != nil {
		return nil, err
	}

	link, err := s.linkFor(ctx, *stored)
	if err != nil {
		return nil, err
	}
	return &link, nil
}

// acceptedFormat refuses a file whose type is not accepted or whose size is over the limit,
// before any of it is stored.
func (s *RFQAttachmentService) acceptedFormat(file UploadFile) (domain.AttachmentFormat, error) {
	if file.Size <= 0 {
		return domain.AttachmentFormat{}, fmt.Errorf("%w: the file is empty", domain.ErrInvalidInput)
	}
	if file.Size > s.cfg.MaxFileSize {
		return domain.AttachmentFormat{}, domain.WithCode(domain.CodeFileTooLarge, fmt.Errorf(
			"%w: the file is %d bytes and the limit is %d",
			domain.ErrInvalidInput, file.Size, s.cfg.MaxFileSize))
	}
	format, ok := domain.AttachmentFormatFor(file.ContentType)
	if !ok {
		return domain.AttachmentFormat{}, domain.WithCode(domain.CodeUnsupportedFileType, fmt.Errorf(
			"%w: %q is not an accepted file type, which are: %s",
			domain.ErrInvalidInput, file.ContentType,
			strings.Join(domain.AcceptedAttachmentContentTypes(), ", ")))
	}
	return format, nil
}

// linkFor signs a link for one stored attachment.
func (s *RFQAttachmentService) linkFor(
	ctx context.Context, attachment domain.RFQAttachment,
) (domain.AttachmentLink, error) {
	if attachment.StorageKey == nil {
		return domain.AttachmentLink{}, fmt.Errorf("%w: attachment %s has no stored file",
			domain.ErrNotFound, attachment.ID)
	}
	url, err := s.storage.GenerateSignedURL(ctx, *attachment.StorageKey, s.cfg.SignedURLExpiry)
	if err != nil {
		return domain.AttachmentLink{}, err
	}
	return domain.AttachmentLink{
		Attachment: attachment,
		URL:        url,
		ExpiresAt:  s.now().Add(s.cfg.SignedURLExpiry),
	}, nil
}

// attachmentKey puts the account at the front of the object key, so tenant isolation is visible
// in the file's own path and a key built for one account cannot name another's.
func attachmentKey(accountID, rfqID, attachmentID uuid.UUID, extension string) string {
	return path.Join("accounts", accountID.String(), "rfqs", rfqID.String(),
		attachmentID.String()+"."+extension)
}

// normalizeContentType drops the parameters a browser appends, so "text/csv; charset=utf-8"
// resolves to the same format as "text/csv".
func normalizeContentType(raw string) string {
	parsed, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return strings.TrimSpace(strings.ToLower(raw))
	}
	return parsed
}
