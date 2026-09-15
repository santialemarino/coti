package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// claimableAttachmentRepository is the attachment surface the sweep needs.
type claimableAttachmentRepository interface {
	ClaimPending(ctx context.Context, q repository.Querier, types []domain.AttachmentType,
		limit int, reclaimAfter time.Duration, now time.Time) ([]domain.ClaimedAttachment, error)
	MarkProcessed(ctx context.Context, q repository.Querier, accountID, attachmentID uuid.UUID,
		extractedText *string, status domain.AttachmentProcessingStatus, processedAt time.Time) error
}

// StoredAttachmentReader turns one stored file into the text its format yields.
type StoredAttachmentReader interface {
	ReadStoredAttachmentText(ctx context.Context, attachment domain.ClaimedAttachment) (string, error)
}

/*
 * AttachmentExtractionJob drains the attachments waiting to be read. Until it existed, an
 * attachment uploaded against an order was stored and never opened: the row sat at PENDING and
 * nothing in the product ever moved it, so a seller who added a voice note to an order got a file
 * and silence.
 *
 * It covers the formats that yield text on their own — a recording, a spreadsheet, a text file.
 * An image and a PDF have no text step: the model reads them as they are, which means running the
 * extraction, which in turn needs the rule for an order whose quote a seller has already reviewed.
 * Those two formats are deliberately left in the queue rather than closed out here, because
 * marking them done without having read them would report work that did not happen.
 */
type AttachmentExtractionJob struct {
	attachments claimableAttachmentRepository
	reader      StoredAttachmentReader
	cfg         config.AttachmentConfig
	now         func() time.Time
	log         *slog.Logger
}

// NewAttachmentExtractionJob builds the pending-attachment sweep.
func NewAttachmentExtractionJob(attachments claimableAttachmentRepository,
	reader StoredAttachmentReader, cfg config.AttachmentConfig,
	log *slog.Logger) *AttachmentExtractionJob {
	return &AttachmentExtractionJob{attachments: attachments, reader: reader, cfg: cfg,
		now: time.Now, log: log}
}

// Name identifies the scheduled attachment sweep.
func (j *AttachmentExtractionJob) Name() string { return "attachment-extraction" }

// Run reads one bounded batch and leaves anything it could not read closed as failed, so a file
// the engine cannot make sense of stops being retried forever while the order keeps its record.
func (j *AttachmentExtractionJob) Run(ctx context.Context,
	q repository.Querier) (domain.JobReport, error) {
	claimed, err := j.attachments.ClaimPending(ctx, q, textBearingAttachments,
		j.cfg.ExtractionBatchSize, j.cfg.ExtractionReclaimAfter, j.now())
	if err != nil {
		return domain.JobReport{}, err
	}

	report := domain.JobReport{Scanned: len(claimed)}
	var failures error
	for _, attachment := range claimed {
		if err := j.process(ctx, q, attachment); err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		report.Changed++
	}
	return report, failures
}

/*
 * process reads one attachment and closes it out either way. A read that fails is recorded as
 * FAILED rather than returned to the queue: the file is what it is, so a later run would fail on
 * the same bytes, and an order whose attachment could not be read is the seller's to look at.
 *
 * The close-out runs on the job's own connection rather than a tenant transaction because the
 * sweep already holds one across every account; the account id comes from the claimed row.
 */
func (j *AttachmentExtractionJob) process(ctx context.Context, q repository.Querier,
	attachment domain.ClaimedAttachment) error {
	text, readErr := j.reader.ReadStoredAttachmentText(ctx, attachment)
	status := domain.AttachmentProcessingDone
	var extracted *string
	if readErr != nil {
		status = domain.AttachmentProcessingFailed
		j.log.WarnContext(ctx, "attachment could not be read",
			slog.String("attachment_id", attachment.ID.String()),
			slog.String("rfq_id", attachment.RFQID.String()), slog.Any("error", readErr))
	} else if text != "" {
		extracted = &text
	}

	if err := j.attachments.MarkProcessed(ctx, q, attachment.AccountID, attachment.ID, extracted,
		status, j.now()); err != nil {
		return fmt.Errorf("close out attachment %s: %w", attachment.ID, err)
	}
	return nil
}

// textBearingAttachments are the formats that carry text of their own, and the only ones this
// sweep claims. An image and a PDF reach the model as they are, so there is nothing to extract out
// of them short of running the extraction — see the type comment.
var textBearingAttachments = []domain.AttachmentType{
	domain.AttachmentTypeAudio, domain.AttachmentTypeSpreadsheet, domain.AttachmentTypeText,
}

var _ Job = (*AttachmentExtractionJob)(nil)
