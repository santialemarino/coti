package services

import (
	"context"
	"errors"
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
}

func (f *fakeBatchQueue) LockDueQuotesCrossAccount(context.Context, repository.Querier, time.Time,
	int) ([]uuid.UUID, error) {
	return nil, nil
}

func (f *fakeBatchQueue) CloseDueCrossAccount(context.Context, repository.Querier, []uuid.UUID,
	time.Time) ([]domain.MessageBatch, error) {
	return nil, nil
}

func (f *fakeBatchQueue) ReclaimStuckCrossAccount(context.Context, repository.Querier, time.Time,
	int, int, string) ([]domain.MessageBatch, error) {
	return nil, nil
}

func (f *fakeBatchQueue) ClaimNextCrossAccount(_ context.Context, _ repository.Querier,
	now time.Time, skip []uuid.UUID,
) (*domain.MessageBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.skips = append(f.skips, slices.Clone(skip))
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
	batch domain.MessageBatch, _ int, failure string,
) (*domain.MessageBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure)
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
