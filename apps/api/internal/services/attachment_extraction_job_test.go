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
	claimTypes []domain.AttachmentType
	claimLimit int
	closed     map[uuid.UUID]closedOut
	markErr    error
}

func (f *fakeClaimRepo) ClaimPending(_ context.Context, _ repository.Querier,
	types []domain.AttachmentType, limit int, _ time.Duration,
	_ time.Time) ([]domain.ClaimedAttachment, error) {
	f.claimTypes, f.claimLimit = types, limit
	return f.claim, f.claimErr
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

type fakeReader struct {
	text string
	err  error
}

func (r fakeReader) ReadStoredAttachmentText(context.Context,
	domain.ClaimedAttachment) (string, error) {
	return r.text, r.err
}

func attachmentJob(repo *fakeClaimRepo, reader StoredAttachmentReader) *AttachmentExtractionJob {
	return NewAttachmentExtractionJob(repo, reader,
		config.AttachmentConfig{ExtractionBatchSize: 7, ExtractionReclaimAfter: 15 * time.Minute},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func claimedAttachment(t domain.AttachmentType) domain.ClaimedAttachment {
	return domain.ClaimedAttachment{ID: uuid.New(), AccountID: uuid.New(), BranchID: uuid.New(),
		RFQID: uuid.New(), Type: t, StorageKey: "accounts/x/order.txt"}
}

// The sweep must only ask for the formats it can finish. Claiming one it cannot close out would
// take the row, skip it, and take it again on every firing once the claim expired.
func TestAttachmentExtractionJob_Run_ClaimsOnlyTheFormatsThatCarryText(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{}
	if _, err := attachmentJob(repo, fakeReader{text: "dos bolsas"}).Run(context.Background(),
		nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}

	wanted := map[domain.AttachmentType]bool{}
	for _, claimType := range repo.claimTypes {
		wanted[claimType] = true
	}
	for _, carries := range []domain.AttachmentType{domain.AttachmentTypeAudio,
		domain.AttachmentTypeSpreadsheet, domain.AttachmentTypeText} {
		if !wanted[carries] {
			t.Errorf("Run() did not claim %q, want it swept", carries)
		}
	}
	for _, needsTheModel := range []domain.AttachmentType{domain.AttachmentTypeImage,
		domain.AttachmentTypePDF} {
		if wanted[needsTheModel] {
			t.Errorf("Run() claimed %q, which it cannot close out", needsTheModel)
		}
	}
	if repo.claimLimit != 7 {
		t.Errorf("claim limit = %d, want the configured 7", repo.claimLimit)
	}
}

func TestAttachmentExtractionJob_Run_StoresWhatItReadAndClosesTheAttachment(t *testing.T) {
	t.Parallel()
	attachment := claimedAttachment(domain.AttachmentTypeText)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{attachment}}

	report, err := attachmentJob(repo, fakeReader{text: "20 bolsas de cemento"}).
		Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if report.Scanned != 1 || report.Changed != 1 {
		t.Errorf("report = %+v, want 1 scanned and 1 changed", report)
	}

	got := repo.closed[attachment.ID]
	if got.status != domain.AttachmentProcessingDone {
		t.Errorf("status = %q, want DONE", got.status)
	}
	if got.text == nil || *got.text != "20 bolsas de cemento" {
		t.Errorf("stored text = %v, want what the reader returned", got.text)
	}
}

/*
 * A file the engine cannot read is closed as FAILED rather than left in the queue. The bytes will
 * not change, so a later run fails on the same file forever — and an order whose attachment could
 * not be read is the seller's to look at, which it cannot be while the row still says pending.
 */
func TestAttachmentExtractionJob_Run_ClosesAnUnreadableAttachmentAsFailed(t *testing.T) {
	t.Parallel()
	attachment := claimedAttachment(domain.AttachmentTypeSpreadsheet)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{attachment}}

	report, err := attachmentJob(repo, fakeReader{err: errors.New("no rows")}).
		Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want the failure recorded rather than returned", err)
	}
	if report.Scanned != 1 || report.Changed != 1 {
		t.Errorf("report = %+v, want the attachment scanned and closed", report)
	}

	got := repo.closed[attachment.ID]
	if got.status != domain.AttachmentProcessingFailed {
		t.Errorf("status = %q, want FAILED", got.status)
	}
	if got.text != nil {
		t.Errorf("stored text = %v, want none for a file that could not be read", *got.text)
	}
}

// One attachment that cannot even be closed out must not cost the rest of the batch, or a single
// bad row stalls the queue behind it on every firing.
func TestAttachmentExtractionJob_Run_ReportsAFailedCloseOutWithoutDroppingTheBatch(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{
		claim: []domain.ClaimedAttachment{claimedAttachment(domain.AttachmentTypeText),
			claimedAttachment(domain.AttachmentTypeText)},
		markErr: errors.New("write refused"),
	}

	report, err := attachmentJob(repo, fakeReader{text: "cemento"}).Run(context.Background(), nil)
	if err == nil {
		t.Fatal("Run() = nil, want the close-out failures joined and returned")
	}
	if report.Scanned != 2 {
		t.Errorf("scanned = %d, want both attachments attempted", report.Scanned)
	}
	if report.Changed != 0 {
		t.Errorf("changed = %d, want none counted when every close-out failed", report.Changed)
	}
}

func TestAttachmentExtractionJob_Run_ReturnsTheClaimFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeClaimRepo{claimErr: errors.New("claim refused")}
	if _, err := attachmentJob(repo, fakeReader{}).Run(context.Background(), nil); err == nil {
		t.Fatal("Run() = nil, want the claim failure")
	}
}
