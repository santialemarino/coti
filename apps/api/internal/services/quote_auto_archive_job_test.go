package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type archiveCall struct {
	statuses []domain.QuoteStatus
	before   time.Time
	now      time.Time
	limit    int
}

type closedArchiveCall struct {
	env    domain.ARCAEnvironment
	before time.Time
	limit  int
}

// fakeInactiveArchiver hands back one prepared batch per call of each sweep and records what it
// was asked.
type fakeInactiveArchiver struct {
	batches       [][]domain.ArchivedQuote
	err           error
	calls         []archiveCall
	closedBatches [][]domain.ArchivedQuote
	closedErr     error
	closedCalls   []closedArchiveCall
}

func (f *fakeInactiveArchiver) ArchiveClosedCrossAccount(
	_ context.Context, _ repository.Querier, env domain.ARCAEnvironment, before time.Time, limit int,
) ([]domain.ArchivedQuote, error) {
	f.closedCalls = append(f.closedCalls, closedArchiveCall{env: env, before: before, limit: limit})
	if f.closedErr != nil {
		return nil, f.closedErr
	}
	if len(f.closedCalls) > len(f.closedBatches) {
		return nil, nil
	}
	return f.closedBatches[len(f.closedCalls)-1], nil
}

func (f *fakeInactiveArchiver) ArchiveInactiveCrossAccount(
	_ context.Context, _ repository.Querier, statuses []domain.QuoteStatus, before, now time.Time,
	limit int,
) ([]domain.ArchivedQuote, error) {
	f.calls = append(f.calls, archiveCall{statuses: statuses, before: before, now: now,
		limit: limit})
	if f.err != nil {
		return nil, f.err
	}
	if len(f.calls) > len(f.batches) {
		return nil, nil
	}
	return f.batches[len(f.calls)-1], nil
}

func archivedQuotes(n int) []domain.ArchivedQuote {
	quotes := make([]domain.ArchivedQuote, n)
	for i := range quotes {
		quotes[i] = domain.ArchivedQuote{AccountID: uuid.New(), BranchID: uuid.New(),
			QuoteID: uuid.New(), Number: int64(i + 1), Status: domain.QuoteStatusSent}
	}
	return quotes
}

func newTestAutoArchiveJob(archiver *fakeInactiveArchiver, batch int) *QuoteAutoArchiveJob {
	job := NewQuoteAutoArchiveJob(archiver,
		config.QuoteArchiveConfig{InactiveAfter: 30 * 24 * time.Hour, ClosedAfter: 45 * 24 * time.Hour,
			BatchSize: batch},
		domain.ARCAEnvironmentProduction, slog.New(slog.NewTextHandler(io.Discard, nil)))
	job.now = func() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) }
	return job
}

func TestQuoteAutoArchiveJob_Run_AsksForTheFollowupStallsQuietPastTheThreshold(t *testing.T) {
	archiver := &fakeInactiveArchiver{}
	job := newTestAutoArchiveJob(archiver, 50)

	if _, err := job.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(archiver.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(archiver.calls))
	}
	call := archiver.calls[0]
	// 2026-10-31 12:00 minus 30 days, by hand.
	if want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC); !call.before.Equal(want) {
		t.Errorf("before = %v, want %v", call.before, want)
	}
	// A SENT quote's offer is judged against the same instant, not a second read of the clock.
	if want := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC); !call.now.Equal(want) {
		t.Errorf("now = %v, want %v", call.now, want)
	}
	if call.limit != 50 {
		t.Errorf("limit = %d, want 50", call.limit)
	}
	want := []domain.QuoteStatus{domain.QuoteStatusDraft, domain.QuoteStatusQuoted,
		domain.QuoteStatusSent}
	if !slices.Equal(call.statuses, want) {
		t.Errorf("statuses = %v, want %v", call.statuses, want)
	}
}

// A backlog larger than one batch clears in one firing: a full batch asks again, a short one ends.
func TestQuoteAutoArchiveJob_Run_DrainsBatchesUntilOneComesBackShort(t *testing.T) {
	archiver := &fakeInactiveArchiver{batches: [][]domain.ArchivedQuote{
		archivedQuotes(2), archivedQuotes(2), archivedQuotes(1),
	}}
	job := newTestAutoArchiveJob(archiver, 2)

	report, err := job.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(archiver.calls) != 3 {
		t.Errorf("calls = %d, want 3", len(archiver.calls))
	}
	if report.Scanned != 5 || report.Changed != 5 {
		t.Errorf("report = %+v, want 5 scanned and 5 changed", report)
	}
}

// A backlog that is an exact multiple of the batch needs one empty batch to know it is done.
func TestQuoteAutoArchiveJob_Run_StopsOnAnEmptyBatch(t *testing.T) {
	archiver := &fakeInactiveArchiver{batches: [][]domain.ArchivedQuote{archivedQuotes(2)}}
	job := newTestAutoArchiveJob(archiver, 2)

	report, err := job.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(archiver.calls) != 2 || report.Changed != 2 {
		t.Errorf("calls = %d, changed = %d, want 2 and 2", len(archiver.calls), report.Changed)
	}
}

func TestQuoteAutoArchiveJob_Run_ReportsAFailedBatch(t *testing.T) {
	boom := errors.New("connection reset")
	archiver := &fakeInactiveArchiver{err: boom}
	job := newTestAutoArchiveJob(archiver, 2)

	if _, err := job.Run(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("Run() = %v, want %v", err, boom)
	}
}

// A run cut short by its timeout stops between batches instead of sweeping on.
func TestQuoteAutoArchiveJob_Run_StopsWhenTheRunIsCancelled(t *testing.T) {
	archiver := &fakeInactiveArchiver{batches: [][]domain.ArchivedQuote{
		archivedQuotes(2), archivedQuotes(2),
	}}
	job := newTestAutoArchiveJob(archiver, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := job.Run(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}
	if len(archiver.calls) != 1 || report.Changed != 2 {
		t.Errorf("calls = %d, changed = %d, want 1 and 2", len(archiver.calls), report.Changed)
	}
}

// Closed outcomes are swept on their own window, with the server's ARCA environment deciding which
// invoices close a sale.
func TestQuoteAutoArchiveJob_Run_SweepsClosedOutcomesOnTheirOwnWindow(t *testing.T) {
	archiver := &fakeInactiveArchiver{}
	job := newTestAutoArchiveJob(archiver, 50)

	if _, err := job.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(archiver.closedCalls) != 1 {
		t.Fatalf("closed calls = %d, want 1", len(archiver.closedCalls))
	}
	call := archiver.closedCalls[0]
	// 2026-10-31 12:00 minus 45 days, by hand.
	if want := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC); !call.before.Equal(want) {
		t.Errorf("before = %v, want %v", call.before, want)
	}
	if call.env != domain.ARCAEnvironmentProduction || call.limit != 50 {
		t.Errorf("call = %+v, want PRODUCTION and 50", call)
	}
}

// Both sweeps drain into one report, and a failed closed sweep is reported.
func TestQuoteAutoArchiveJob_Run_DrainsTheClosedSweepToo(t *testing.T) {
	archiver := &fakeInactiveArchiver{
		batches:       [][]domain.ArchivedQuote{archivedQuotes(1)},
		closedBatches: [][]domain.ArchivedQuote{archivedQuotes(2), archivedQuotes(1)},
	}
	job := newTestAutoArchiveJob(archiver, 2)

	report, err := job.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(archiver.closedCalls) != 2 || report.Changed != 4 {
		t.Errorf("closed calls = %d, changed = %d, want 2 and 4", len(archiver.closedCalls),
			report.Changed)
	}

	boom := errors.New("connection reset")
	failing := &fakeInactiveArchiver{closedErr: boom}
	if _, err := newTestAutoArchiveJob(failing, 2).Run(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("Run() = %v, want %v", err, boom)
	}
}
