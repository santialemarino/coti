package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type closedOut struct {
	text   *string
	status domain.AttachmentProcessingStatus
}

type fakeClaimRepo struct {
	claim      []domain.ClaimedAttachment
	claimErr   error
	claimLimit int
	stored     map[uuid.UUID][]domain.RFQAttachment
	listErr    error
	closed     map[uuid.UUID]closedOut
	markErr    error
}

func (f *fakeClaimRepo) ClaimPendingByRFQ(_ context.Context, _ repository.Querier, rfqLimit int,
	_ time.Duration, _ time.Time) ([]domain.ClaimedAttachment, error) {
	f.claimLimit = rfqLimit
	return f.claim, f.claimErr
}

func (f *fakeClaimRepo) ListReadByRFQIDs(_ context.Context, _ repository.Querier,
	_, _ []uuid.UUID) (map[uuid.UUID][]domain.RFQAttachment, error) {
	return f.stored, f.listErr
}

func (f *fakeClaimRepo) MarkProcessed(_ context.Context, _ repository.Querier,
	_, attachmentID uuid.UUID, extractedText *string, status domain.AttachmentProcessingStatus,
	_ time.Time) error {
	if f.markErr != nil {
		return f.markErr
	}
	if f.closed == nil {
		f.closed = map[uuid.UUID]closedOut{}
	}
	f.closed[attachmentID] = closedOut{text: extractedText, status: status}
	return nil
}

// fakeReader answers per attachment type, which is what lets one order mix a file that reads with
// one that does not.
type fakeReader struct {
	byType map[domain.AttachmentType]readResult
}

type readResult struct {
	block domain.Content
	text  string
	err   error
}

func (r fakeReader) ReadStoredAttachment(_ context.Context,
	attachment domain.ClaimedAttachment) (domain.Content, string, error) {
	answer, ok := r.byType[attachment.Type]
	if !ok {
		return domain.Content{}, "", errors.New("unexpected type in fake reader")
	}
	return answer.block, answer.text, answer.err
}

func textReader(text string) fakeReader {
	return fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeText: {block: domain.TextContent(text), text: text},
	}}
}

type foldCall struct {
	rfqID     uuid.UUID
	blocks    []domain.Content
	extracted string
}

type fakeFolder struct {
	calls   []foldCall
	outcome domain.AttachmentFoldOutcome
	err     error
}

func (f *fakeFolder) FoldAttachmentsIntoQuote(_ context.Context, _ domain.Tenant,
	rfqID uuid.UUID, blocks []domain.Content,
	extracted string) (domain.AttachmentFoldOutcome, error) {
	f.calls = append(f.calls, foldCall{rfqID: rfqID, blocks: blocks, extracted: extracted})
	if f.err != nil {
		return "", f.err
	}
	outcome := f.outcome
	if outcome == "" {
		outcome = domain.AttachmentFoldedIntoDraft
	}
	return outcome, nil
}

func attachmentJob(repo *fakeClaimRepo, reader StoredAttachmentReader,
	folder attachmentFolder) *AttachmentExtractionJob {
	return NewAttachmentExtractionJob(repo, reader, folder,
		config.AttachmentConfig{ExtractionRFQBatchSize: 7, ExtractionReclaimAfter: 15 * time.Minute},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func claimedIn(rfqID uuid.UUID, kind domain.AttachmentType) domain.ClaimedAttachment {
	return domain.ClaimedAttachment{ID: uuid.New(), AccountID: uuid.New(), BranchID: uuid.New(),
		RFQID: rfqID, Type: kind, StorageKey: "accounts/x/order.txt"}
}

// The batch bounds orders, not files. Bounding files would let a batch boundary fall inside one
// order, which extracts over half its material and closes the other half out as read.
func TestAttachmentExtractionJob_Run_BoundsTheBatchByOrder(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{}
	if _, err := attachmentJob(repo, textReader("dos bolsas"), &fakeFolder{}).
		Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if repo.claimLimit != 7 {
		t.Errorf("claim limit = %d, want the configured 7", repo.claimLimit)
	}
}

/*
 * An order that arrived as several files is one order: a photo and the recording explaining it
 * describe the same job. Extracting per file would run the model twice, each time over half of
 * what the client said, and the second draft would replace the first.
 */
func TestAttachmentExtractionJob_Run_ExtractsOncePerOrderOverAllItsMaterial(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	first := claimedIn(rfqID, domain.AttachmentTypeText)
	second := claimedIn(rfqID, domain.AttachmentTypeImage)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{first, second}}
	reader := fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeText:  {block: domain.TextContent("20 bolsas"), text: "20 bolsas"},
		domain.AttachmentTypeImage: {block: domain.ImageContent("image/jpeg", []byte{0xFF})},
	}}
	folder := &fakeFolder{}

	report, err := attachmentJob(repo, reader, folder).Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(folder.calls) != 1 {
		t.Fatalf("extractions = %d, want exactly one for the order", len(folder.calls))
	}
	if got := folder.calls[0]; got.rfqID != rfqID || len(got.blocks) != 2 {
		t.Errorf("extraction = rfq %s with %d blocks, want rfq %s with both files",
			got.rfqID, len(got.blocks), rfqID)
	}
	if report.Changed != 2 {
		t.Errorf("changed = %d, want both attachments closed out", report.Changed)
	}
	for _, attachment := range []domain.ClaimedAttachment{first, second} {
		if repo.closed[attachment.ID].status != domain.AttachmentProcessingDone {
			t.Errorf("attachment %s = %q, want DONE", attachment.ID,
				repo.closed[attachment.ID].status)
		}
	}
	// An image carries no text of its own, so only the text file's reading is stored.
	if stored := repo.closed[second.ID].text; stored != nil {
		t.Errorf("image stored text = %q, want none", *stored)
	}
}

/*
 * An unreadable file does not abort its order. It is closed FAILED on its own — the bytes will not
 * change, so requeueing it fails forever while hiding the order — and whatever else the order
 * carried is still extracted. The failed row is what the seller sees on the order.
 */
func TestAttachmentExtractionJob_Run_ExtractsTheRestWhenOneFileCannotBeRead(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	readable := claimedIn(rfqID, domain.AttachmentTypeText)
	broken := claimedIn(rfqID, domain.AttachmentTypeSpreadsheet)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{readable, broken}}
	reader := fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeText:        {block: domain.TextContent("cemento"), text: "cemento"},
		domain.AttachmentTypeSpreadsheet: {err: errors.New("no rows")},
	}}
	folder := &fakeFolder{}

	if _, err := attachmentJob(repo, reader, folder).Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want the unreadable file recorded rather than returned", err)
	}
	if len(folder.calls) != 1 || len(folder.calls[0].blocks) != 1 {
		t.Fatalf("extraction = %+v, want one extraction over the one file that read", folder.calls)
	}
	if repo.closed[broken.ID].status != domain.AttachmentProcessingFailed {
		t.Errorf("unreadable file = %q, want FAILED", repo.closed[broken.ID].status)
	}
	if repo.closed[readable.ID].status != domain.AttachmentProcessingDone {
		t.Errorf("readable file = %q, want DONE", repo.closed[readable.ID].status)
	}
}

// An order whose files all failed still reaches the engine, with nothing to read. That is what
// lets it decide the order is the seller's to load by hand rather than leaving it looking pending.
func TestAttachmentExtractionJob_Run_ReportsAnOrderWithNothingReadable(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	broken := claimedIn(rfqID, domain.AttachmentTypeText)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{broken}}
	reader := fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeText: {err: errors.New("gone")},
	}}
	folder := &fakeFolder{outcome: domain.AttachmentUnreadable}

	if _, err := attachmentJob(repo, reader, folder).Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(folder.calls) != 1 || len(folder.calls[0].blocks) != 0 {
		t.Fatalf("extraction = %+v, want the order reported with no material", folder.calls)
	}
}

/*
 * An extraction that fails is not the file's fault: the model may simply have been unreachable.
 * The attachments stay claimed rather than being closed out, so the claim expires and a later
 * firing reads them again — closing them DONE here would report material nothing ever read.
 */
func TestAttachmentExtractionJob_Run_LeavesAttachmentsClaimedWhenExtractionFails(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	attachment := claimedIn(rfqID, domain.AttachmentTypeText)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{attachment}}
	folder := &fakeFolder{err: errors.New("provider unreachable")}

	report, err := attachmentJob(repo, textReader("cemento"), folder).Run(context.Background(), nil)
	if err == nil {
		t.Fatal("Run() = nil, want the extraction failure reported")
	}
	if _, closed := repo.closed[attachment.ID]; closed {
		t.Errorf("attachment was closed out as %q, want it left claimed for the reclaim window",
			repo.closed[attachment.ID].status)
	}
	if report.Changed != 0 {
		t.Errorf("changed = %d, want nothing counted when the extraction failed", report.Changed)
	}
}

/*
 * What an order's earlier files yielded goes to the model alongside the new one. Without it an
 * order whose photo was read an hour ago and whose recording arrives now is extracted from the
 * recording alone, and the draft version that replaces the current one loses the photo's lines.
 */
func TestAttachmentExtractionJob_Run_IncludesWhatTheOrdersEarlierFilesYielded(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	arriving := claimedIn(rfqID, domain.AttachmentTypeText)
	earlierText := "10 chapas"
	repo := &fakeClaimRepo{
		claim: []domain.ClaimedAttachment{arriving},
		stored: map[uuid.UUID][]domain.RFQAttachment{rfqID: {{
			ID: uuid.New(), RFQID: rfqID, Type: domain.AttachmentTypeAudio,
			ExtractedText: &earlierText, ProcessingStatus: domain.AttachmentProcessingDone,
		}}},
	}
	folder := &fakeFolder{}

	if _, err := attachmentJob(repo, textReader("20 bolsas"), folder).
		Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(folder.calls) != 1 {
		t.Fatalf("extractions = %d, want one", len(folder.calls))
	}
	if got := len(folder.calls[0].blocks); got != 2 {
		t.Fatalf("blocks = %d, want the arriving file and what the earlier one yielded", got)
	}
	if got := folder.calls[0].blocks[0].Text; got != earlierText {
		t.Errorf("first block = %q, want the earlier reading %q", got, earlierText)
	}
}

/*
 * A file this run just read appears in both lists — it was DONE from an earlier firing that then
 * failed, or it was reclaimed. The fresh reading is the one that counts; folding in the stored one
 * too would hand the model the same order twice and invite duplicate lines.
 */
func TestAttachmentExtractionJob_Run_PrefersTheFreshReadingOverTheStoredOne(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	arriving := claimedIn(rfqID, domain.AttachmentTypeText)
	stale := "what it said last time"
	repo := &fakeClaimRepo{
		claim: []domain.ClaimedAttachment{arriving},
		stored: map[uuid.UUID][]domain.RFQAttachment{rfqID: {{
			ID: arriving.ID, RFQID: rfqID, Type: domain.AttachmentTypeText,
			ExtractedText: &stale, ProcessingStatus: domain.AttachmentProcessingDone,
		}}},
	}
	folder := &fakeFolder{}

	if _, err := attachmentJob(repo, textReader("20 bolsas"), folder).
		Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if got := len(folder.calls[0].blocks); got != 1 {
		t.Fatalf("blocks = %d, want only the fresh reading of the file", got)
	}
	if got := folder.calls[0].blocks[0].Text; got != "20 bolsas" {
		t.Errorf("block = %q, want the reading from this run", got)
	}
}

/*
 * Earlier readings are CONTEXT for something new, never input on their own. When every file this
 * run claimed fails to read, there is nothing new to interpret — extracting over the order's old
 * material would pay for a model call that can only reproduce the draft the order already has, and
 * would write a second draft version saying the same thing.
 */
func TestAttachmentExtractionJob_Run_DoesNotReExtractOldMaterialWhenNothingNewWasRead(t *testing.T) {
	t.Parallel()
	rfqID := uuid.New()
	broken := claimedIn(rfqID, domain.AttachmentTypeText)
	earlierText := "20 bolsas de cemento"
	repo := &fakeClaimRepo{
		claim: []domain.ClaimedAttachment{broken},
		stored: map[uuid.UUID][]domain.RFQAttachment{rfqID: {{
			ID: uuid.New(), RFQID: rfqID, Type: domain.AttachmentTypeText,
			ExtractedText: &earlierText, ProcessingStatus: domain.AttachmentProcessingDone,
		}}},
	}
	reader := fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeText: {err: errors.New("gone")},
	}}
	folder := &fakeFolder{outcome: domain.AttachmentUnreadable}

	if _, err := attachmentJob(repo, reader, folder).Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(folder.calls) != 1 {
		t.Fatalf("extractions = %d, want the order reported once", len(folder.calls))
	}
	if got := len(folder.calls[0].blocks); got != 0 {
		t.Errorf("blocks = %d, want none — the order's old material is not a new order", got)
	}
}

// One order that cannot be closed out must not cost the rest of the batch, or a single bad row
// stalls the queue behind it on every firing.
func TestAttachmentExtractionJob_Run_ReportsAFailedCloseOutWithoutDroppingTheBatch(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{
		claim: []domain.ClaimedAttachment{claimedIn(uuid.New(), domain.AttachmentTypeText),
			claimedIn(uuid.New(), domain.AttachmentTypeText)},
		markErr: errors.New("write refused"),
	}
	folder := &fakeFolder{}

	report, err := attachmentJob(repo, textReader("cemento"), folder).Run(context.Background(), nil)
	if err == nil {
		t.Fatal("Run() = nil, want the close-out failures joined and returned")
	}
	if len(folder.calls) != 2 {
		t.Errorf("extractions = %d, want both orders attempted", len(folder.calls))
	}
	if report.Changed != 0 {
		t.Errorf("changed = %d, want none counted when every close-out failed", report.Changed)
	}
}

func TestAttachmentExtractionJob_Run_ReturnsTheClaimFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{claimErr: errors.New("claim refused")}
	if _, err := attachmentJob(repo, fakeReader{}, &fakeFolder{}).
		Run(context.Background(), nil); err == nil {
		t.Fatal("Run() = nil, want the claim failure")
	}
}
