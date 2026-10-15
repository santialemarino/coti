package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// stubTx is the transaction the closer opens; the fake queue never reads it.
type stubTx struct{ pgx.Tx }

func (stubTx) Commit(context.Context) error   { return nil }
func (stubTx) Rollback(context.Context) error { return nil }

type stubQueueDB struct{}

func (stubQueueDB) AdminTx(context.Context) (pgx.Tx, error) { return stubTx{}, nil }
func (stubQueueDB) CrossAccount() repository.Querier        { return nil }

// fakeBatchQueue serves claims from pending in order, never one the caller skipped.
type fakeBatchQueue struct {
	mu       sync.Mutex
	pending  []domain.MessageBatch
	skips    [][]uuid.UUID
	released []uuid.UUID
	failures []string
	failTo   domain.MessageBatchStatus
	limits   []int
	races    int
	// reclaimBefore is the cutoff the last reclaim was asked for.
	reclaimBefore time.Time
}

func (f *fakeBatchQueue) LockDueQuotesCrossAccount(context.Context, repository.Querier, time.Time,
	int) ([]uuid.UUID, error) {
	return nil, nil
}

func (f *fakeBatchQueue) CloseDueCrossAccount(context.Context, repository.Querier, []uuid.UUID,
	time.Time) ([]domain.MessageBatch, error) {
	return nil, nil
}

func (f *fakeBatchQueue) ReclaimStuckCrossAccount(_ context.Context, _ repository.Querier,
	startedBefore time.Time, _ int, _ int, _ string,
) ([]domain.MessageBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reclaimBefore = startedBefore
	return nil, nil
}

func (f *fakeBatchQueue) ClaimNextCrossAccount(_ context.Context, _ repository.Querier,
	now time.Time, skip []uuid.UUID,
) (*domain.MessageBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.skips = append(f.skips, slices.Clone(skip))
	if f.races > 0 {
		f.races--
		return nil, domain.ErrConflict
	}
	for _, batch := range f.pending {
		if !slices.Contains(skip, batch.ID) {
			batch.Status = domain.MessageBatchStatusProcessing
			batch.ProcessingStartedAt = &now
			return &batch, nil
		}
	}
	return nil, nil
}

func (f *fakeBatchQueue) MarkProcessedCrossAccount(context.Context, repository.Querier,
	domain.MessageBatch, time.Time) (bool, error) {
	return true, nil
}

func (f *fakeBatchQueue) RecordFailureCrossAccount(_ context.Context, _ repository.Querier,
	batch domain.MessageBatch, maxAttempts int, failure string,
) (*domain.MessageBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure)
	f.limits = append(f.limits, maxAttempts)
	batch.Status = f.failTo
	batch.Attempts++
	return &batch, nil
}

func (f *fakeBatchQueue) ReleaseCrossAccount(_ context.Context, _ repository.Querier,
	batch domain.MessageBatch,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, batch.ID)
	return true, nil
}

type handlerFunc func(ctx context.Context, batch domain.MessageBatch) error

func (h handlerFunc) Handle(ctx context.Context, batch domain.MessageBatch) error {
	return h(ctx, batch)
}

func newTestBatchService(queue *fakeBatchQueue, handler ConversationBatchHandler,
	timeout time.Duration,
) *ConversationBatchService {
	return NewConversationBatchService(stubQueueDB{}, queue, handler, config.ConversationConfig{
		MaxAttempts: 3, ProcessingTimeout: timeout, SweepBatchSize: 10}, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func pendingBatches(n int) []domain.MessageBatch {
	batches := make([]domain.MessageBatch, n)
	for i := range batches {
		batches[i] = domain.MessageBatch{ID: uuid.New(), AccountID: uuid.New(), QuoteID: uuid.New(),
			Status: domain.MessageBatchStatusClosed}
	}
	return batches
}

func TestConversationBatchService_FailingBatchIsTriedOncePerPass(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(2), failTo: domain.MessageBatchStatusClosed}
	service := newTestBatchService(queue, handlerFunc(func(context.Context, domain.MessageBatch) error {
		return errors.New("model unavailable")
	}), time.Minute)

	report, err := service.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() = %v, want no error", err)
	}
	if report.Claimed != 2 || report.Retried != 2 || report.Processed != 0 {
		t.Errorf("report = %+v, want both batches claimed once and returned for a retry", report)
	}
	if len(queue.skips) != 3 || len(queue.skips[2]) != 2 {
		t.Errorf("claim skips = %v, want the third claim to skip both batches tried", queue.skips)
	}
	if !slices.Equal(queue.failures, []string{"model unavailable", "model unavailable"}) {
		t.Errorf("failures = %v, want the handler's error recorded per attempt", queue.failures)
	}
}

func TestConversationBatchService_LastAttemptCountsAsFailed(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(1), failTo: domain.MessageBatchStatusFailed}
	service := newTestBatchService(queue, handlerFunc(func(context.Context, domain.MessageBatch) error {
		return errors.New("bad proposal")
	}), time.Minute)

	report, err := service.Sweep(context.Background())
	if err != nil || report.Failed != 1 || report.Retried != 0 {
		t.Errorf("Sweep() = %+v, %v; want one FAILED batch", report, err)
	}
}

// A refusal no retry can fix spends the batch's last attempt at once.
func TestConversationBatchService_UnprocessableBatchFailsAtOnce(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(2), failTo: domain.MessageBatchStatusClosed}
	service := newTestBatchService(queue, handlerFunc(func(_ context.Context, batch domain.MessageBatch) error {
		if batch.ID == queue.pending[0].ID {
			return fmt.Errorf("%w: archived", domain.ErrBatchUnprocessable)
		}
		return errors.New("model unavailable")
	}), time.Minute)

	if _, err := service.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep() = %v", err)
	}
	if !slices.Equal(queue.limits, []int{1, 3}) {
		t.Errorf("attempt limits = %v, want 1 for the unprocessable batch and 3 for the retryable one",
			queue.limits)
	}
}

// A handler past its deadline is an attempt that failed; a shutdown is not.
func TestConversationBatchService_HandlerTimeoutCountsAnAttempt(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(1), failTo: domain.MessageBatchStatusClosed}
	service := newTestBatchService(queue, handlerFunc(func(ctx context.Context, _ domain.MessageBatch) error {
		<-ctx.Done()
		return ctx.Err()
	}), 10*time.Millisecond)

	if _, err := service.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep() = %v, want no error", err)
	}
	if len(queue.failures) != 1 || len(queue.released) != 0 {
		t.Errorf("failures/released = %v/%v, want the timeout recorded as an attempt",
			queue.failures, queue.released)
	}
}

func TestConversationBatchService_ShutdownReleasesWithoutCountingAnAttempt(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(1), failTo: domain.MessageBatchStatusClosed}
	ctx, cancel := context.WithCancel(context.Background())
	service := newTestBatchService(queue, handlerFunc(func(handleCtx context.Context, _ domain.MessageBatch) error {
		cancel()
		<-handleCtx.Done()
		return handleCtx.Err()
	}), time.Minute)

	_, err := service.Sweep(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Sweep() = %v, want the cancellation", err)
	}
	if len(queue.released) != 1 || len(queue.failures) != 0 {
		t.Errorf("released/failures = %v/%v, want the claim released uncounted",
			queue.released, queue.failures)
	}
}

func TestConversationBatchTicker_StopsOnCancelAndNeverOverlaps(t *testing.T) {
	var inFlight, maxInFlight, passes atomic.Int32
	queue := &fakeBatchQueue{failTo: domain.MessageBatchStatusClosed}
	// Each pass claims one batch whose handler outlasts the interval several times over.
	handler := handlerFunc(func(context.Context, domain.MessageBatch) error {
		now := inFlight.Add(1)
		for {
			seen := maxInFlight.Load()
			if now <= seen || maxInFlight.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		passes.Add(1)
		return nil
	})
	sweeper := newTestBatchService(queue, handler, time.Minute)
	sweeper.cfg.SweepBatchSize = 1
	queue.pending = pendingBatches(1)
	ticker := NewConversationBatchTicker(sweeper, time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for passes.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() still running 5s after cancel, want it stopped")
	}
	if passes.Load() < 3 {
		t.Errorf("passes = %d, want the ticker to keep sweeping", passes.Load())
	}
	if maxInFlight.Load() != 1 {
		t.Errorf("concurrent passes = %d, want 1", maxInFlight.Load())
	}
	stopped := passes.Load()
	time.Sleep(20 * time.Millisecond)
	if passes.Load() != stopped {
		t.Errorf("passes after Run returned = %d, want %d", passes.Load(), stopped)
	}
}

// A claim lost to another worker on the same quote moves on to the next batch.
func TestConversationBatchService_LostClaimRaceKeepsThePassGoing(t *testing.T) {
	queue := &fakeBatchQueue{pending: pendingBatches(1), races: 1}
	service := newTestBatchService(queue, handlerFunc(func(context.Context, domain.MessageBatch) error {
		return nil
	}), time.Minute)

	report, err := service.Sweep(context.Background())
	if err != nil || report.Processed != 1 {
		t.Errorf("Sweep() = %+v, %v; want the batch processed after the lost race", report, err)
	}
}

// A handler finishing right at its deadline is never reclaimed: the cutoff leaves a tick of margin.
func TestConversationBatchService_ReclaimWaitsATickPastTheHandlerDeadline(t *testing.T) {
	queue := &fakeBatchQueue{}
	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	service := NewConversationBatchService(stubQueueDB{}, queue, handlerFunc(
		func(context.Context, domain.MessageBatch) error { return nil }),
		config.ConversationConfig{MaxAttempts: 3, ProcessingTimeout: 10 * time.Minute,
			TickerInterval: 30 * time.Second, SweepBatchSize: 10},
		func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := service.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep() = %v, want no error", err)
	}
	if want := time.Date(2026, time.October, 14, 11, 49, 30, 0, time.UTC); !queue.reclaimBefore.Equal(want) {
		t.Errorf("reclaim cutoff = %s, want %s", queue.reclaimBefore, want)
	}
}
