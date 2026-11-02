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

type flagCall struct {
	statuses []domain.QuoteStatus
	before   time.Time
	limit    int
}

// fakeStalledFlagger hands back one prepared batch per call and records what it was asked.
type fakeStalledFlagger struct {
	batches [][]domain.FlaggedQuote
	err     error
	calls   []flagCall
}

func (f *fakeStalledFlagger) FlagStalledCrossAccount(
	_ context.Context, _ repository.Querier, statuses []domain.QuoteStatus, before time.Time,
	limit int,
) ([]domain.FlaggedQuote, error) {
	f.calls = append(f.calls, flagCall{statuses: statuses, before: before, limit: limit})
	if f.err != nil {
		return nil, f.err
	}
	if len(f.calls) > len(f.batches) {
		return nil, nil
	}
	return f.batches[len(f.calls)-1], nil
}

func flaggedQuotes(n int) []domain.FlaggedQuote {
	quotes := make([]domain.FlaggedQuote, n)
	for i := range quotes {
		quotes[i] = domain.FlaggedQuote{AccountID: uuid.New(), BranchID: uuid.New(),
			QuoteID: uuid.New(), Number: int64(i + 1), Status: domain.QuoteStatusQuoted}
	}
	return quotes
}

func newTestFollowupJob(flagger *fakeStalledFlagger, batch int) *QuoteFollowupJob {
	job := NewQuoteFollowupJob(flagger,
		config.QuoteFollowupConfig{SellerIdleAfter: 2 * 24 * time.Hour,
			CustomerSilentAfter: 5 * 24 * time.Hour, BatchSize: batch},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	job.now = func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) }
	return job
}

// Each origin is swept with its own statuses and its own threshold.
func TestQuoteFollowupJob_Run_SweepsEachOriginOnItsOwnThreshold(t *testing.T) {
	flagger := &fakeStalledFlagger{}
	job := newTestFollowupJob(flagger, 50)

	if _, err := job.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(flagger.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(flagger.calls))
	}
	seller, customer := flagger.calls[0], flagger.calls[1]
	if want := []domain.QuoteStatus{domain.QuoteStatusDraft,
		domain.QuoteStatusQuoted}; !slices.Equal(seller.statuses, want) {
		t.Errorf("seller statuses = %v, want %v", seller.statuses, want)
	}
	// 2026-11-02 12:00 minus 2 days, by hand.
	if want := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC); !seller.before.Equal(want) {
		t.Errorf("seller before = %v, want %v", seller.before, want)
	}
	if want := []domain.QuoteStatus{domain.QuoteStatusSent}; !slices.Equal(customer.statuses, want) {
		t.Errorf("customer statuses = %v, want %v", customer.statuses, want)
	}
	// 2026-11-02 12:00 minus 5 days, by hand.
	if want := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC); !customer.before.Equal(want) {
		t.Errorf("customer before = %v, want %v", customer.before, want)
	}
	if seller.limit != 50 || customer.limit != 50 {
		t.Errorf("limits = %d, %d, want 50 and 50", seller.limit, customer.limit)
	}
}

// A backlog larger than one batch clears in one firing: a full batch asks again, a short one ends
// that origin's sweep and starts the next.
func TestQuoteFollowupJob_Run_DrainsBatchesUntilOneComesBackShort(t *testing.T) {
	flagger := &fakeStalledFlagger{batches: [][]domain.FlaggedQuote{
		flaggedQuotes(2), flaggedQuotes(1), flaggedQuotes(2), flaggedQuotes(0),
	}}
	job := newTestFollowupJob(flagger, 2)

	report, err := job.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if len(flagger.calls) != 4 {
		t.Errorf("calls = %d, want 4", len(flagger.calls))
	}
	if report.Scanned != 5 || report.Changed != 5 {
		t.Errorf("report = %+v, want 5 scanned and 5 changed", report)
	}
	if !slices.Equal(flagger.calls[2].statuses, domain.FollowupCustomerStatuses) {
		t.Errorf("third call statuses = %v, want the customer's", flagger.calls[2].statuses)
	}
}

func TestQuoteFollowupJob_Run_ReportsAFailedBatch(t *testing.T) {
	boom := errors.New("connection reset")
	flagger := &fakeStalledFlagger{err: boom}
	job := newTestFollowupJob(flagger, 2)

	if _, err := job.Run(context.Background(), nil); !errors.Is(err, boom) {
		t.Fatalf("Run() = %v, want %v", err, boom)
	}
	if len(flagger.calls) != 1 {
		t.Errorf("calls = %d, want 1: a failed sweep stops the run", len(flagger.calls))
	}
}

// A run cut short by its timeout stops between batches instead of sweeping on.
func TestQuoteFollowupJob_Run_StopsWhenTheRunIsCancelled(t *testing.T) {
	flagger := &fakeStalledFlagger{batches: [][]domain.FlaggedQuote{flaggedQuotes(2)}}
	job := newTestFollowupJob(flagger, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := job.Run(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}
	if len(flagger.calls) != 1 || report.Changed != 2 {
		t.Errorf("calls = %d, changed = %d, want 1 and 2", len(flagger.calls), report.Changed)
	}
}
