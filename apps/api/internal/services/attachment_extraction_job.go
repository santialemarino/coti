package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// claimableAttachmentRepository is the attachment surface the sweep needs.
type claimableAttachmentRepository interface {
	ClaimPendingByRFQ(ctx context.Context, q repository.Querier, rfqLimit int,
		reclaimAfter time.Duration, now time.Time) ([]domain.ClaimedAttachment, error)
	ListReadByRFQIDs(ctx context.Context, q repository.Querier,
		rfqIDs, accountIDs []uuid.UUID) (map[uuid.UUID][]domain.RFQAttachment, error)
	MarkProcessed(ctx context.Context, q repository.Querier, accountID, attachmentID uuid.UUID,
		extractedText *string, status domain.AttachmentProcessingStatus, processedAt time.Time) error
}

// StoredAttachmentReader turns one stored file into the block a model reads and the text the
// attachment row keeps.
type StoredAttachmentReader interface {
	ReadStoredAttachment(ctx context.Context,
		attachment domain.ClaimedAttachment) (domain.Content, string, error)
}

// attachmentFolder runs the RFQ pipeline over the material an order's files carried and decides,
// against the quote's own state, whether any of it may be written.
type attachmentFolder interface {
	FoldAttachmentsIntoQuote(ctx context.Context, tenant domain.Tenant, rfqID uuid.UUID,
		blocks []domain.Content, extractedText string) (domain.AttachmentFoldOutcome, error)
}

/*
 * AttachmentExtractionJob drains the attachments waiting to be read. Until it existed, an
 * attachment uploaded against an order was stored and never opened: the row sat at PENDING and
 * nothing in the product ever moved it, so a seller who added a voice note to an order got a file
 * and silence.
 *
 * Its unit is the order, not the file. An order regularly arrives as a photo and a recording
 * explaining it, and reading those as two orders would extract twice over half the material each
 * time. So a firing takes whole orders, reads everything each one carries, and extracts once.
 */
type AttachmentExtractionJob struct {
	attachments claimableAttachmentRepository
	reader      StoredAttachmentReader
	folder      attachmentFolder
	cfg         config.AttachmentConfig
	now         func() time.Time
	log         *slog.Logger
}

// NewAttachmentExtractionJob builds the pending-attachment sweep.
func NewAttachmentExtractionJob(attachments claimableAttachmentRepository,
	reader StoredAttachmentReader, folder attachmentFolder, cfg config.AttachmentConfig,
	log *slog.Logger) *AttachmentExtractionJob {
	return &AttachmentExtractionJob{attachments: attachments, reader: reader, folder: folder,
		cfg: cfg, now: time.Now, log: log}
}

// Name identifies the scheduled attachment sweep.
func (j *AttachmentExtractionJob) Name() string { return "attachment-extraction" }

// Run reads one bounded batch of orders and leaves anything it could not read closed as failed, so
// a file the engine cannot make sense of stops being retried forever while the order keeps its
// record.
func (j *AttachmentExtractionJob) Run(ctx context.Context,
	q repository.Querier) (domain.JobReport, error) {
	claimed, err := j.attachments.ClaimPendingByRFQ(ctx, q, j.cfg.ExtractionRFQBatchSize,
		j.cfg.ExtractionReclaimAfter, j.now())
	if err != nil {
		return domain.JobReport{}, err
	}

	report := domain.JobReport{Scanned: len(claimed)}
	orders := groupByRFQ(claimed)
	earlier := j.earlierReadings(ctx, q, orders)

	var failures error
	for _, order := range orders {
		changed, orderErr := j.processOrder(ctx, q, order, earlier[order[0].RFQID])
		report.Changed += changed
		if orderErr != nil {
			failures = errors.Join(failures, orderErr)
		}
	}
	return report, failures
}

/*
 * earlierReadings loads what every order in the batch has already read, in one query rather than
 * one per order. An empty result is not fatal: an order is still extractable from what was just
 * read, and losing earlier context is better than dropping the batch.
 */
func (j *AttachmentExtractionJob) earlierReadings(ctx context.Context, q repository.Querier,
	orders [][]domain.ClaimedAttachment) map[uuid.UUID][]domain.RFQAttachment {
	rfqIDs := make([]uuid.UUID, 0, len(orders))
	accountIDs := make([]uuid.UUID, 0, len(orders))
	for _, order := range orders {
		rfqIDs = append(rfqIDs, order[0].RFQID)
		accountIDs = append(accountIDs, order[0].AccountID)
	}
	earlier, err := j.attachments.ListReadByRFQIDs(ctx, q, rfqIDs, accountIDs)
	if err != nil {
		j.log.WarnContext(ctx, "earlier attachment readings unavailable", slog.Any("error", err))
		return nil
	}
	return earlier
}

/*
 * processOrder reads everything one order is holding and extracts over the lot. A file that could
 * not be read is closed out FAILED on its own — its bytes will not change, so requeueing it would
 * fail forever while hiding the order — and the rest of the order is extracted anyway.
 *
 * An extraction that fails is the opposite case and is left claimed rather than closed: the model
 * may simply have been unreachable, and letting the claim expire is what puts the order back in
 * the queue for a later firing.
 */
func (j *AttachmentExtractionJob) processOrder(ctx context.Context, q repository.Querier,
	order []domain.ClaimedAttachment, earlier []domain.RFQAttachment) (int, error) {
	tenant := domain.Tenant{AccountID: order[0].AccountID, BranchID: order[0].BranchID}
	ctx = domain.WithAIRFQ(domain.WithAIAccount(ctx, tenant), order[0].RFQID)

	var changed int
	var failures error
	read := make([]readAttachment, 0, len(order))
	for _, attachment := range order {
		block, text, readErr := j.reader.ReadStoredAttachment(ctx, attachment)
		if readErr != nil {
			j.log.WarnContext(ctx, "attachment could not be read",
				slog.String("attachment_id", attachment.ID.String()),
				slog.String("rfq_id", attachment.RFQID.String()), slog.Any("error", readErr))
			if err := j.closeOut(ctx, q, attachment, nil,
				domain.AttachmentProcessingFailed); err != nil {
				failures = errors.Join(failures, err)
				continue
			}
			changed++
			continue
		}
		read = append(read, readAttachment{attachment: attachment, block: block, text: text})
	}

	blocks, extracted := orderMaterial(read, earlier)
	outcome, foldErr := j.folder.FoldAttachmentsIntoQuote(ctx, tenant, order[0].RFQID, blocks,
		extracted)
	if foldErr != nil {
		return changed, errors.Join(failures, fmt.Errorf("extract rfq %s: %w", order[0].RFQID,
			foldErr))
	}
	j.log.InfoContext(ctx, "order attachments read", slog.String("rfq_id", order[0].RFQID.String()),
		slog.Int("attachments", len(order)), slog.String("outcome", string(outcome)))

	for _, item := range read {
		var text *string
		if item.text != "" {
			text = &item.text
		}
		if err := j.closeOut(ctx, q, item.attachment, text,
			domain.AttachmentProcessingDone); err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		changed++
	}
	return changed, failures
}

/*
 * orderMaterial assembles what the model reads: the files just read, plus what the order's
 * already-processed files carried. Without the second half an order whose photo was read an hour
 * ago and whose recording arrives now would be extracted from the recording alone, and the new
 * draft version would lose every line the photo had produced.
 *
 * Only stored text is reused — a recording is never transcribed twice — so an earlier image or PDF
 * contributes nothing here. That is a known gap rather than an oversight: re-reading those means
 * downloading them again on every later attachment.
 *
 * Earlier readings are context for something new, never input on their own: with nothing newly
 * read there is nothing to interpret, and extracting over the old material alone would pay for a
 * model call that can only reproduce the draft the order already has.
 */
func orderMaterial(read []readAttachment,
	earlier []domain.RFQAttachment) ([]domain.Content, string) {
	if len(read) == 0 {
		return nil, ""
	}
	blocks := make([]domain.Content, 0, len(read)+len(earlier))
	texts := make([]string, 0, len(read)+len(earlier))

	claimedNow := make(map[uuid.UUID]bool, len(read))
	for _, item := range read {
		claimedNow[item.attachment.ID] = true
	}
	for _, previous := range earlier {
		// A row claimed by this run is in both lists, and its fresh reading is the one to use.
		if claimedNow[previous.ID] || previous.ExtractedText == nil {
			continue
		}
		blocks = append(blocks, domain.TextContent(*previous.ExtractedText))
		texts = append(texts, *previous.ExtractedText)
	}

	for _, item := range read {
		blocks = append(blocks, item.block)
		if item.text != "" {
			texts = append(texts, item.text)
		}
	}
	return blocks, strings.Join(texts, "\n\n")
}

// closeOut records how far one attachment got.
func (j *AttachmentExtractionJob) closeOut(ctx context.Context, q repository.Querier,
	attachment domain.ClaimedAttachment, text *string,
	status domain.AttachmentProcessingStatus) error {
	if err := j.attachments.MarkProcessed(ctx, q, attachment.AccountID, attachment.ID, text,
		status, j.now()); err != nil {
		return fmt.Errorf("close out attachment %s: %w", attachment.ID, err)
	}
	return nil
}

// readAttachment is one attachment the sweep managed to read, with what it yielded.
type readAttachment struct {
	attachment domain.ClaimedAttachment
	block      domain.Content
	text       string
}

// groupByRFQ collects the claimed attachments into the orders they belong to, keeping the order
// the claim returned them in so an order's material reaches the model oldest file first.
func groupByRFQ(claimed []domain.ClaimedAttachment) [][]domain.ClaimedAttachment {
	orders := make([][]domain.ClaimedAttachment, 0)
	at := make(map[uuid.UUID]int, len(claimed))
	for _, attachment := range claimed {
		index, seen := at[attachment.RFQID]
		if !seen {
			at[attachment.RFQID] = len(orders)
			orders = append(orders, []domain.ClaimedAttachment{attachment})
			continue
		}
		orders[index] = append(orders[index], attachment)
	}
	return orders
}

var _ Job = (*AttachmentExtractionJob)(nil)
