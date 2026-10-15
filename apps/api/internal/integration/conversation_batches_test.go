//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// batchClock is the sweeps' clock. The queue is global, so every fixture here is dated around it,
// far in the past, and a sweep never touches the live windows other tests open at today's date.
var batchClock = time.Date(2001, time.March, 1, 12, 0, 0, 0, time.UTC)

var batchConfig = config.ConversationConfig{WindowReset: 2 * time.Minute,
	WindowMax: 10 * time.Minute, ContextMessages: 10, TickerInterval: 5 * time.Millisecond,
	MaxAttempts: 3, ProcessingTimeout: 10 * time.Minute, SweepBatchSize: 100}

// recordingBatchHandler records each batch it handles and fails when two batches of one quote
// are handled at once, or one batch twice.
type recordingBatchHandler struct {
	mu       sync.Mutex
	order    []uuid.UUID
	handled  map[uuid.UUID]int
	inFlight map[uuid.UUID]int
	problems []string
	fail     func(batch domain.MessageBatch) error
	hold     time.Duration
	block    bool
}

func newRecordingBatchHandler() *recordingBatchHandler {
	return &recordingBatchHandler{handled: map[uuid.UUID]int{}, inFlight: map[uuid.UUID]int{}}
}

func (h *recordingBatchHandler) Handle(ctx context.Context, batch domain.MessageBatch) error {
	h.mu.Lock()
	h.inFlight[batch.QuoteID]++
	if h.inFlight[batch.QuoteID] > 1 {
		h.problems = append(h.problems, fmt.Sprintf("quote %s handled twice at once", batch.QuoteID))
	}
	h.handled[batch.ID]++
	if h.handled[batch.ID] > 1 {
		h.problems = append(h.problems, fmt.Sprintf("batch %s handled twice", batch.ID))
	}
	h.order = append(h.order, batch.ID)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.inFlight[batch.QuoteID]--
		h.mu.Unlock()
	}()
	if h.block {
		<-ctx.Done()
		return ctx.Err()
	}
	time.Sleep(h.hold)
	if h.fail != nil {
		return h.fail(batch)
	}
	return nil
}

func (h *recordingBatchHandler) report(t *testing.T) []uuid.UUID {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, problem := range h.problems {
		t.Error(problem)
	}
	return slices.Clone(h.order)
}

func (e *env) batchSweeper(handler services.ConversationBatchHandler) *services.ConversationBatchService {
	return services.NewConversationBatchService(e.db, repository.NewMessageBatchRepository(),
		handler, batchConfig, func() time.Time { return batchClock },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// batchQuote seeds a sent quote in a fresh account; the account's teardown removes everything.
func (e *env) batchQuote(t *testing.T, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	return e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
		channel: e.seedWhatsAppInbox(t, accountID, branchID), destination: "+5491155550101",
		status: domain.QuoteStatusSent, sentAgo: time.Hour, expiresIn: 24 * time.Hour})
}

// seedBatch writes a batch in status, closed (or due) at closedAt.
func (e *env) seedBatch(t *testing.T, accountID, quoteID uuid.UUID,
	status domain.MessageBatchStatus, closedAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var closed *time.Time
	if status != domain.MessageBatchStatusOpen {
		closed = &closedAt
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO message_batch (id, account_id, quote_id, status, opened_at, closes_at, closed_at)
		 VALUES ($1, $2, $3, $4, $5::timestamptz - interval '5 minutes', $5, $6)`,
		id, accountID, quoteID, status, closedAt, closed); err != nil {
		t.Fatalf("seed batch: %v", err)
	}
	return id
}

func (e *env) seedConversationMessage(t *testing.T, accountID, quoteID uuid.UUID,
	author domain.MessageAuthorType,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_message (id, account_id, quote_id, author_type, body, received_at)
		 VALUES ($1, $2, $3, $4, 'mensaje', $5)`,
		id, accountID, quoteID, author, batchClock.Add(-time.Hour)); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return id
}

func (e *env) readBatch(t *testing.T, id uuid.UUID) domain.MessageBatch {
	t.Helper()
	var batch domain.MessageBatch
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id, account_id, quote_id, status, closed_at, processed_at, attempts, last_error,
		        processing_started_at
		 FROM message_batch WHERE id = $1`, id,
	).Scan(&batch.ID, &batch.AccountID, &batch.QuoteID, &batch.Status, &batch.ClosedAt,
		&batch.ProcessedAt, &batch.Attempts, &batch.LastError, &batch.ProcessingStartedAt); err != nil {
		t.Fatalf("read batch %s: %v", id, err)
	}
	return batch
}

func (e *env) batchOfMessage(t *testing.T, id uuid.UUID) *uuid.UUID {
	t.Helper()
	var batchID *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT batch_id FROM quote_message WHERE id = $1`, id).Scan(&batchID); err != nil {
		t.Fatalf("read message %s: %v", id, err)
	}
	return batchID
}

func sweepOnce(t *testing.T, sweeper *services.ConversationBatchService) domain.ConversationSweep {
	t.Helper()
	report, err := sweeper.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() = %v, want no error", err)
	}
	return report
}

func TestConversationBatches_CloseStampsClientMessagesOnce(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Cierre")
	quoteID := e.batchQuote(t, accountID, branchID)
	dueID := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusOpen, batchClock.Add(-time.Second))
	first := e.seedConversationMessage(t, accountID, quoteID, domain.MessageAuthorClient)
	second := e.seedConversationMessage(t, accountID, quoteID, domain.MessageAuthorClient)
	seller := e.seedConversationMessage(t, accountID, quoteID, domain.MessageAuthorSeller)
	laterQuote := e.batchQuote(t, accountID, branchID)
	notDue := e.seedBatch(t, accountID, laterQuote, domain.MessageBatchStatusOpen, batchClock.Add(time.Minute))
	sweeper := e.batchSweeper(services.NoopConversationBatchHandler{})

	report := sweepOnce(t, sweeper)
	if report.Closed != 1 || report.Processed != 1 {
		t.Errorf("first sweep = %+v, want one window closed and processed", report)
	}
	due := e.readBatch(t, dueID)
	if due.Status != domain.MessageBatchStatusProcessed || due.ClosedAt == nil ||
		!due.ClosedAt.Equal(batchClock) || due.ProcessedAt == nil {
		t.Errorf("due batch = %+v, want PROCESSED, closed at the sweep's clock", due)
	}
	for _, id := range []uuid.UUID{first, second} {
		if got := e.batchOfMessage(t, id); got == nil || *got != dueID {
			t.Errorf("client message batch = %v, want %s", got, dueID)
		}
	}
	if got := e.batchOfMessage(t, seller); got != nil {
		t.Errorf("seller message batch = %s, want none: the seller's messages are context", got)
	}
	if got := e.readBatch(t, notDue); got.Status != domain.MessageBatchStatusOpen {
		t.Errorf("window not yet due = %s, want OPEN", got.Status)
	}

	if again := sweepOnce(t, sweeper); again != (domain.ConversationSweep{}) {
		t.Errorf("second sweep = %+v, want nothing left to do", again)
	}
	if got := e.readBatch(t, dueID); got.Status != domain.MessageBatchStatusProcessed ||
		!got.ProcessedAt.Equal(*due.ProcessedAt) {
		t.Errorf("batch after a second sweep = %+v, want it untouched", got)
	}
}

// The closer takes the quote's lock before the window's, and skips a quote another writer holds.
func TestConversationBatches_CloseSkipsALockedQuote(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Bloqueo")
	quoteID := e.batchQuote(t, accountID, branchID)
	dueID := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusOpen, batchClock.Add(-time.Second))
	sweeper := e.batchSweeper(services.NoopConversationBatchHandler{})

	tx, err := e.db.AdminTx(context.Background())
	if err != nil {
		t.Fatalf("AdminTx() = %v", err)
	}
	if _, err := tx.Exec(context.Background(), `SELECT id FROM quote WHERE id = $1 FOR UPDATE`,
		quoteID); err != nil {
		t.Fatalf("lock quote: %v", err)
	}
	report := sweepOnce(t, sweeper)
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("release quote: %v", err)
	}
	if report.Closed != 0 || e.readBatch(t, dueID).Status != domain.MessageBatchStatusOpen {
		t.Errorf("sweep under the quote's lock = %+v, want the window left OPEN", report)
	}
	if report := sweepOnce(t, sweeper); report.Closed != 1 {
		t.Errorf("sweep after release = %+v, want the window closed", report)
	}
}

func TestConversationBatches_ProcessesOneQuoteInClosedOrder(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon FIFO")
	quoteID := e.batchQuote(t, accountID, branchID)
	// Seeded out of order, so neither insertion nor id order matches closed_at.
	third := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-time.Minute))
	first := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-3*time.Minute))
	second := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-2*time.Minute))
	handler := newRecordingBatchHandler()

	report := sweepOnce(t, e.batchSweeper(handler))
	if report.Processed != 3 {
		t.Errorf("sweep = %+v, want three processed", report)
	}
	if order := handler.report(t); !slices.Equal(order, []uuid.UUID{first, second, third}) {
		t.Errorf("handled = %v, want %v", order, []uuid.UUID{first, second, third})
	}
}

// Another quote's batch is claimed while one quote waits on its own PROCESSING batch.
func TestConversationBatches_ABusyQuoteDoesNotBlockTheQueue(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Ocupado")
	busyQuote := e.batchQuote(t, accountID, branchID)
	running := e.seedBatch(t, accountID, busyQuote, domain.MessageBatchStatusClosed, batchClock.Add(-3*time.Minute))
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE message_batch SET status = 'PROCESSING', processing_started_at = $2 WHERE id = $1`,
		running, batchClock); err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	waiting := e.seedBatch(t, accountID, busyQuote, domain.MessageBatchStatusClosed, batchClock.Add(-2*time.Minute))
	otherQuote := e.batchQuote(t, accountID, branchID)
	other := e.seedBatch(t, accountID, otherQuote, domain.MessageBatchStatusClosed, batchClock.Add(-time.Minute))
	handler := newRecordingBatchHandler()

	sweepOnce(t, e.batchSweeper(handler))
	if order := handler.report(t); !slices.Equal(order, []uuid.UUID{other}) {
		t.Errorf("handled = %v, want only the other quote's batch %s", order, other)
	}
	if got := e.readBatch(t, waiting); got.Status != domain.MessageBatchStatusClosed {
		t.Errorf("waiting batch = %s, want CLOSED behind its quote's PROCESSING one", got.Status)
	}
}

func TestConversationBatches_ConcurrentWorkersNeverShareAQuote(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Concurrente")
	var want int
	perQuote := map[uuid.UUID][]uuid.UUID{}
	for range 4 {
		quoteID := e.batchQuote(t, accountID, branchID)
		for i := range 3 {
			perQuote[quoteID] = append(perQuote[quoteID], e.seedBatch(t, accountID, quoteID,
				domain.MessageBatchStatusClosed, batchClock.Add(time.Duration(i-10)*time.Minute)))
			want++
		}
	}
	handler := newRecordingBatchHandler()
	handler.hold = 20 * time.Millisecond

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.batchSweeper(handler).Sweep(context.Background())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Sweep() = %v, want no error", err)
		}
	}
	// A worker that found every head busy stops early; one more pass drains what is left.
	sweepOnce(t, e.batchSweeper(handler))

	order := handler.report(t)
	if len(order) != want {
		t.Errorf("handled %d batches, want %d", len(order), want)
	}
	for quoteID, batches := range perQuote {
		var seen []uuid.UUID
		for _, id := range order {
			if slices.Contains(batches, id) {
				seen = append(seen, id)
			}
		}
		if !slices.Equal(seen, batches) {
			t.Errorf("quote %s handled %v, want %v", quoteID, seen, batches)
		}
	}
}

func TestConversationBatches_RetriesThenFailsAndTheQueueMovesOn(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Reintentos")
	quoteID := e.batchQuote(t, accountID, branchID)
	failing := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-2*time.Minute))
	next := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-time.Minute))
	handler := newRecordingBatchHandler()
	handler.fail = func(batch domain.MessageBatch) error {
		if batch.ID == failing {
			return errors.New("handler refused")
		}
		return nil
	}
	sweeper := e.batchSweeper(handler)

	for pass := 1; pass <= 2; pass++ {
		sweepOnce(t, sweeper)
		got := e.readBatch(t, failing)
		if got.Status != domain.MessageBatchStatusClosed || got.Attempts != pass ||
			got.LastError == nil || *got.LastError != "handler refused" {
			t.Fatalf("after pass %d failing batch = %+v, want CLOSED with %d attempts", pass, got, pass)
		}
		if got := e.readBatch(t, next); got.Status != domain.MessageBatchStatusClosed {
			t.Fatalf("after pass %d next batch = %s, want CLOSED behind the retried one", pass, got.Status)
		}
	}
	report := sweepOnce(t, sweeper)
	if got := e.readBatch(t, failing); got.Status != domain.MessageBatchStatusFailed || got.Attempts != 3 {
		t.Errorf("after the last attempt failing batch = %+v, want FAILED with 3 attempts", got)
	}
	if got := e.readBatch(t, next); got.Status != domain.MessageBatchStatusProcessed {
		t.Errorf("next batch = %s, want PROCESSED once the head failed", got.Status)
	}
	if report.Failed != 1 || report.Processed != 1 {
		t.Errorf("last pass = %+v, want one failed and one processed", report)
	}
}

func TestConversationBatches_ReclaimsAStuckClaimAndFencesItsOwner(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Colgado")
	stuckAt := batchClock.Add(-batchConfig.ProcessingTimeout - time.Minute)
	stuck := func(attempts int) uuid.UUID {
		quoteID := e.batchQuote(t, accountID, branchID)
		id := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, stuckAt.Add(-time.Minute))
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE message_batch SET status = 'PROCESSING', processing_started_at = $2, attempts = $3
			 WHERE id = $1`, id, stuckAt, attempts); err != nil {
			t.Fatalf("mark stuck: %v", err)
		}
		return id
	}
	retried := stuck(0)
	exhausted := stuck(batchConfig.MaxAttempts - 1)

	report := sweepOnce(t, e.batchSweeper(services.NoopConversationBatchHandler{}))
	if report.Reclaimed != 2 {
		t.Errorf("sweep = %+v, want two claims reclaimed", report)
	}
	got := e.readBatch(t, retried)
	if got.Status != domain.MessageBatchStatusProcessed || got.Attempts != 1 ||
		got.LastError == nil || *got.LastError != "processing timed out" {
		t.Errorf("reclaimed batch = %+v, want reprocessed with the lost attempt counted", got)
	}
	if got := e.readBatch(t, exhausted); got.Status != domain.MessageBatchStatusFailed ||
		got.Attempts != batchConfig.MaxAttempts {
		t.Errorf("exhausted batch = %+v, want FAILED", got)
	}

	// Reclaimed and claimed again by a second worker: the first worker's late writes miss it.
	lost := stuck(0)
	original := e.readBatch(t, lost)
	repo := repository.NewMessageBatchRepository()
	ctx := context.Background()
	if _, err := repo.ReclaimStuckCrossAccount(ctx, e.db.CrossAccount(),
		batchClock.Add(-batchConfig.ProcessingTimeout), 3, 10, "processing timed out"); err != nil {
		t.Fatalf("ReclaimStuck() = %v", err)
	}
	reclaimed, err := repo.ClaimNextCrossAccount(ctx, e.db.CrossAccount(), batchClock, nil)
	if err != nil || reclaimed == nil || reclaimed.ID != lost {
		t.Fatalf("ClaimNext() = %+v, %v; want %s claimed again", reclaimed, err, lost)
	}
	if kept, err := repo.MarkProcessedCrossAccount(ctx, e.db.CrossAccount(), original, batchClock); err != nil || kept {
		t.Errorf("late MarkProcessed = %v, %v; want the lost claim refused", kept, err)
	}
	if settled, err := repo.RecordFailureCrossAccount(ctx, e.db.CrossAccount(), original, 3, "late"); err != nil || settled != nil {
		t.Errorf("late RecordFailure = %+v, %v; want the lost claim refused", settled, err)
	}
	if kept, err := repo.MarkProcessedCrossAccount(ctx, e.db.CrossAccount(), *reclaimed, batchClock); err != nil || !kept {
		t.Errorf("MarkProcessed(current claim) = %v, %v; want it settled", kept, err)
	}
}

// Messages are stamped only with their own account's batch, and on the owner pool, where RLS is
// off, a claim's writes still refuse another account.
func TestConversationBatches_NeverCrossAccounts(t *testing.T) {
	e := newEnv(t)
	ownID, ownBranch := e.seedAccount(t, "Corralon Propio Lote")
	strangerID, strangerBranch := e.seedAccount(t, "Corralon Ajeno Lote")
	ownQuote := e.batchQuote(t, ownID, ownBranch)
	strangerQuote := e.batchQuote(t, strangerID, strangerBranch)
	ownBatch := e.seedBatch(t, ownID, ownQuote, domain.MessageBatchStatusOpen, batchClock.Add(-time.Second))
	strangerBatch := e.seedBatch(t, strangerID, strangerQuote, domain.MessageBatchStatusOpen,
		batchClock.Add(-time.Second))
	ownMessage := e.seedConversationMessage(t, ownID, ownQuote, domain.MessageAuthorClient)
	strangerMessage := e.seedConversationMessage(t, strangerID, strangerQuote, domain.MessageAuthorClient)
	handler := newRecordingBatchHandler()

	sweepOnce(t, e.batchSweeper(handler))
	if got := e.batchOfMessage(t, ownMessage); got == nil || *got != ownBatch {
		t.Errorf("own message batch = %v, want %s", got, ownBatch)
	}
	if got := e.batchOfMessage(t, strangerMessage); got == nil || *got != strangerBatch {
		t.Errorf("stranger message batch = %v, want %s", got, strangerBatch)
	}
	handler.report(t)

	// Claim a fresh batch, then settle it under the other account's id.
	claimable := e.seedBatch(t, ownID, ownQuote, domain.MessageBatchStatusClosed, batchClock.Add(-time.Hour))
	repo := repository.NewMessageBatchRepository()
	ctx := context.Background()
	claim, err := repo.ClaimNextCrossAccount(ctx, e.db.CrossAccount(), batchClock, nil)
	if err != nil || claim == nil || claim.ID != claimable {
		t.Fatalf("ClaimNext() = %+v, %v; want %s", claim, err, claimable)
	}
	foreign := *claim
	foreign.AccountID = strangerID
	if kept, err := repo.MarkProcessedCrossAccount(ctx, e.db.CrossAccount(), foreign, batchClock); err != nil || kept {
		t.Errorf("MarkProcessed(other account) = %v, %v; want refused", kept, err)
	}
	if settled, err := repo.RecordFailureCrossAccount(ctx, e.db.CrossAccount(), foreign, 3, "x"); err != nil || settled != nil {
		t.Errorf("RecordFailure(other account) = %+v, %v; want refused", settled, err)
	}
	if released, err := repo.ReleaseCrossAccount(ctx, e.db.CrossAccount(), foreign); err != nil || released {
		t.Errorf("Release(other account) = %v, %v; want refused", released, err)
	}
	if kept, err := repo.MarkProcessedCrossAccount(ctx, e.db.CrossAccount(), *claim, batchClock); err != nil || !kept {
		t.Errorf("MarkProcessed(own account) = %v, %v; want it settled", kept, err)
	}
}

// The API's ticker and the scheduled backstop sweep the same queue at once.
func TestConversationBatches_TickerAndJobShareTheQueueSafely(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Ticker Job")
	var want int
	for range 5 {
		quoteID := e.batchQuote(t, accountID, branchID)
		// Two batches already queued, and the quote's one open window due behind them.
		for i, status := range []domain.MessageBatchStatus{domain.MessageBatchStatusClosed,
			domain.MessageBatchStatusClosed, domain.MessageBatchStatusOpen} {
			e.seedBatch(t, accountID, quoteID, status, batchClock.Add(time.Duration(i-10)*time.Minute))
			want++
		}
	}
	handler := newRecordingBatchHandler()
	handler.hold = 10 * time.Millisecond
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ticker := services.NewConversationBatchTicker(e.batchSweeper(handler), batchConfig.TickerInterval, log)
	jobs := e.jobService(t, services.NewConversationBatchJob(e.batchSweeper(handler)))

	ctx, cancel := context.WithCancel(context.Background())
	tickerDone := make(chan struct{})
	go func() {
		defer close(tickerDone)
		ticker.Run(ctx)
	}()
	run, err := jobs.Run(context.Background(), "conversation-batches")
	if run != nil {
		t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM job_run WHERE id = $1`, run.ID) })
	}
	if err != nil {
		t.Fatalf("job Run() = %v, want no error", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(handler.report(t)) < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-tickerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("ticker still running 5s after cancel, want it stopped")
	}
	if got := len(handler.report(t)); got != want {
		t.Errorf("handled %d batches, want %d", got, want)
	}
	var left int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM message_batch WHERE account_id = $1 AND status <> 'PROCESSED'`,
		accountID).Scan(&left); err != nil {
		t.Fatalf("count unprocessed: %v", err)
	}
	if left != 0 {
		t.Errorf("unprocessed batches = %d, want 0", left)
	}
}

// Shutting the ticker down mid-batch hands the claim back without spending an attempt.
func TestConversationBatches_TickerShutdownReleasesItsClaim(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Apagado")
	quoteID := e.batchQuote(t, accountID, branchID)
	batchID := e.seedBatch(t, accountID, quoteID, domain.MessageBatchStatusClosed, batchClock.Add(-time.Minute))
	handler := newRecordingBatchHandler()
	handler.block = true
	ticker := services.NewConversationBatchTicker(e.batchSweeper(handler), time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for e.readBatch(t, batchID).Status != domain.MessageBatchStatusProcessing && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ticker still running 5s after cancel, want it stopped")
	}
	if got := e.readBatch(t, batchID); got.Status != domain.MessageBatchStatusClosed || got.Attempts != 0 {
		t.Errorf("interrupted batch = %+v, want CLOSED with no attempt spent", got)
	}
	handler.report(t)
}
