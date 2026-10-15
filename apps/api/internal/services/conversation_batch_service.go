package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// stuckBatchFailure is the last_error a reclaimed claim records.
const stuckBatchFailure = "processing timed out"

// ConversationBatchHandler acts on a claimed batch after the claim commits, in its own tenant
// transaction, and owns every write it causes — the SENT → CHANGE_REQUESTED move included.
type ConversationBatchHandler interface {
	Handle(ctx context.Context, batch domain.MessageBatch) error
}

type conversationQueueDB interface {
	AdminTx(ctx context.Context) (pgx.Tx, error)
	CrossAccount() repository.Querier
}

type conversationBatchQueue interface {
	LockDueQuotesCrossAccount(ctx context.Context, q repository.Querier, now time.Time,
		limit int) ([]uuid.UUID, error)
	CloseDueCrossAccount(ctx context.Context, q repository.Querier, quoteIDs []uuid.UUID,
		now time.Time) ([]domain.MessageBatch, error)
	ReclaimStuckCrossAccount(ctx context.Context, q repository.Querier, startedBefore time.Time,
		maxAttempts, limit int, failure string) ([]domain.MessageBatch, error)
	ClaimNextCrossAccount(ctx context.Context, q repository.Querier, now time.Time,
		skip []uuid.UUID) (*domain.MessageBatch, error)
	MarkProcessedCrossAccount(ctx context.Context, q repository.Querier, batch domain.MessageBatch,
		now time.Time) (bool, error)
	RecordFailureCrossAccount(ctx context.Context, q repository.Querier, batch domain.MessageBatch,
		maxAttempts int, failure string) (*domain.MessageBatch, error)
	ReleaseCrossAccount(ctx context.Context, q repository.Querier,
		batch domain.MessageBatch) (bool, error)
}

// ConversationBatchService closes due windows and works their queue across every account, one
// batch per quote at a time in closed_at order. Concurrent passes are safe; the database decides.
type ConversationBatchService struct {
	db      conversationQueueDB
	queue   conversationBatchQueue
	handler ConversationBatchHandler
	cfg     config.ConversationConfig
	now     func() time.Time
	log     *slog.Logger
}

// NewConversationBatchService builds a ConversationBatchService. A nil handler leaves processing
// off: windows still close, and closed batches wait for one. A nil now uses time.Now.
func NewConversationBatchService(db conversationQueueDB, queue conversationBatchQueue,
	handler ConversationBatchHandler, cfg config.ConversationConfig, now func() time.Time,
	log *slog.Logger,
) *ConversationBatchService {
	if now == nil {
		now = time.Now
	}
	if handler == nil {
		log.Warn("conversation batch processing is off until a handler is configured; " +
			"closed batches wait")
	}
	return &ConversationBatchService{db: db, queue: queue, handler: handler, cfg: cfg, now: now,
		log: log}
}

// Sweep closes every due window, then, when a handler is configured, reclaims stuck claims and
// processes up to the sweep batch size.
func (s *ConversationBatchService) Sweep(ctx context.Context) (domain.ConversationSweep, error) {
	var report domain.ConversationSweep
	if err := s.closeDue(ctx, &report); err != nil {
		return report, err
	}
	if s.handler == nil {
		return report, nil
	}
	if err := s.reclaimStuck(ctx, &report); err != nil {
		return report, err
	}
	return report, s.processQueue(ctx, &report)
}

func (s *ConversationBatchService) closeDue(ctx context.Context, report *domain.ConversationSweep) error {
	for {
		now := s.now().UTC()
		locked, closed, err := s.closePage(ctx, now)
		if err != nil {
			return err
		}
		for _, batch := range closed {
			s.log.Info("conversation window closed", slog.String("account", batch.AccountID.String()),
				slog.String("quote", batch.QuoteID.String()), slog.String("batch", batch.ID.String()))
		}
		report.Closed += len(closed)
		// A page whose quotes were all held elsewhere would come back the same; the next pass
		// retries them.
		if locked < s.cfg.SweepBatchSize || len(closed) == 0 {
			return nil
		}
	}
}

func (s *ConversationBatchService) closePage(ctx context.Context, now time.Time,
) (int, []domain.MessageBatch, error) {
	tx, err := s.db.AdminTx(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	quoteIDs, err := s.queue.LockDueQuotesCrossAccount(ctx, tx, now, s.cfg.SweepBatchSize)
	if err != nil || len(quoteIDs) == 0 {
		return 0, nil, err
	}
	closed, err := s.queue.CloseDueCrossAccount(ctx, tx, quoteIDs, now)
	if err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, err
	}
	return len(quoteIDs), closed, nil
}

func (s *ConversationBatchService) reclaimStuck(ctx context.Context,
	report *domain.ConversationSweep,
) error {
	for {
		startedBefore := s.now().UTC().Add(-s.cfg.ProcessingTimeout)
		reclaimed, err := s.queue.ReclaimStuckCrossAccount(ctx, s.db.CrossAccount(), startedBefore,
			s.cfg.MaxAttempts, s.cfg.SweepBatchSize, stuckBatchFailure)
		if err != nil {
			return err
		}
		for _, batch := range reclaimed {
			s.logOutcome("conversation batch reclaimed", batch)
		}
		report.Reclaimed += len(reclaimed)
		if len(reclaimed) < s.cfg.SweepBatchSize {
			return nil
		}
	}
}

func (s *ConversationBatchService) processQueue(ctx context.Context,
	report *domain.ConversationSweep,
) error {
	// Each batch gets one attempt per pass, so a failing one waits for the next pass to retry.
	tried := make([]uuid.UUID, 0, s.cfg.SweepBatchSize)
	for len(tried) < s.cfg.SweepBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch, err := s.queue.ClaimNextCrossAccount(ctx, s.db.CrossAccount(), s.now().UTC(), tried)
		if err != nil {
			return err
		}
		if batch == nil {
			return nil
		}
		tried = append(tried, batch.ID)
		report.Claimed++
		if err := s.process(ctx, *batch, report); err != nil {
			return err
		}
	}
	return nil
}

// process hands a claimed batch to the handler and settles the claim. Its writes detach from ctx,
// so a shutdown still releases what it interrupted instead of leaving it PROCESSING.
func (s *ConversationBatchService) process(ctx context.Context, batch domain.MessageBatch,
	report *domain.ConversationSweep,
) error {
	handleCtx, cancel := context.WithTimeout(ctx, s.cfg.ProcessingTimeout)
	handleErr := s.handler.Handle(handleCtx, batch)
	cancel()
	settle := context.WithoutCancel(ctx)
	q := s.db.CrossAccount()

	if handleErr == nil {
		kept, err := s.queue.MarkProcessedCrossAccount(settle, q, batch, s.now().UTC())
		if err != nil {
			return err
		}
		if !kept {
			s.logOutcome("conversation batch claim lost before it finished", batch)
			return nil
		}
		report.Processed++
		s.log.Info("conversation batch processed", slog.String("account", batch.AccountID.String()),
			slog.String("quote", batch.QuoteID.String()), slog.String("batch", batch.ID.String()))
		return nil
	}

	if ctx.Err() != nil {
		if _, err := s.queue.ReleaseCrossAccount(settle, q, batch); err != nil {
			return errors.Join(ctx.Err(), err)
		}
		return ctx.Err()
	}
	maxAttempts := s.cfg.MaxAttempts
	if errors.Is(handleErr, domain.ErrBatchUnprocessable) {
		maxAttempts = 1
	}
	settled, err := s.queue.RecordFailureCrossAccount(settle, q, batch, maxAttempts,
		handleErr.Error())
	if err != nil {
		return err
	}
	if settled == nil {
		s.logOutcome("conversation batch claim lost before it failed", batch)
		return nil
	}
	if settled.Status == domain.MessageBatchStatusFailed {
		report.Failed++
	} else {
		report.Retried++
	}
	s.logOutcome("conversation batch attempt failed", *settled)
	return nil
}

func (s *ConversationBatchService) logOutcome(message string, batch domain.MessageBatch) {
	attrs := []any{slog.String("account", batch.AccountID.String()),
		slog.String("quote", batch.QuoteID.String()), slog.String("batch", batch.ID.String()),
		slog.String("status", string(batch.Status)), slog.Int("attempts", batch.Attempts)}
	if batch.LastError != nil {
		attrs = append(attrs, slog.String("error", *batch.LastError))
	}
	if batch.Status == domain.MessageBatchStatusFailed {
		s.log.Error(message, attrs...)
		return
	}
	s.log.Warn(message, attrs...)
}

// ConversationBatchTicker runs the conversation sweep inside the API process on a fixed interval.
// Passes never overlap: a pass longer than the interval delays the next one.
type ConversationBatchTicker struct {
	sweeper  *ConversationBatchService
	interval time.Duration
	log      *slog.Logger
}

// NewConversationBatchTicker builds a ConversationBatchTicker.
func NewConversationBatchTicker(sweeper *ConversationBatchService, interval time.Duration,
	log *slog.Logger,
) *ConversationBatchTicker {
	return &ConversationBatchTicker{sweeper: sweeper, interval: interval, log: log}
}

// Run sweeps once, then once per interval, until ctx is cancelled; it returns once the pass in
// flight has settled.
func (t *ConversationBatchTicker) Run(ctx context.Context) {
	t.log.Info("conversation ticker started", slog.Duration("interval", t.interval))
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		t.pass(ctx)
		select {
		case <-ctx.Done():
			t.log.Info("conversation ticker stopped")
			return
		case <-ticker.C:
		}
	}
}

func (t *ConversationBatchTicker) pass(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	report, err := t.sweeper.Sweep(ctx)
	if err != nil && ctx.Err() == nil {
		t.log.Error("conversation sweep failed", slog.Any("error", err))
		return
	}
	if report != (domain.ConversationSweep{}) {
		t.log.Info("conversation sweep", slog.Int("closed", report.Closed),
			slog.Int("reclaimed", report.Reclaimed), slog.Int("claimed", report.Claimed),
			slog.Int("processed", report.Processed),
			slog.Int("retried", report.Retried), slog.Int("failed", report.Failed))
	}
}

// ConversationBatchJob is the scheduled backstop for the API's conversation ticker.
type ConversationBatchJob struct {
	sweeper *ConversationBatchService
}

// NewConversationBatchJob builds the scheduled conversation sweep.
func NewConversationBatchJob(sweeper *ConversationBatchService) *ConversationBatchJob {
	return &ConversationBatchJob{sweeper: sweeper}
}

// Name identifies the scheduled conversation sweep.
func (j *ConversationBatchJob) Name() string { return "conversation-batches" }

// Run sweeps once. Each close and claim commits on its own owner-pool statement or transaction,
// so the run's lock connection is not needed for the work.
func (j *ConversationBatchJob) Run(ctx context.Context, _ repository.Querier) (domain.JobReport, error) {
	report, err := j.sweeper.Sweep(ctx)
	return domain.JobReport{
		Scanned: report.Closed + report.Reclaimed + report.Claimed,
		Changed: report.Closed + report.Reclaimed + report.Processed + report.Retried + report.Failed,
	}, err
}

var _ Job = (*ConversationBatchJob)(nil)
