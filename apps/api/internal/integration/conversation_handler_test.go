//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// scriptedProposer stands in for the model: it answers from the input it was given.
type scriptedProposer struct {
	mu     sync.Mutex
	inputs []domain.ChangeRequestInput
	answer func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error)
}

func (p *scriptedProposer) Propose(_ context.Context, in domain.ChangeRequestInput,
) (*domain.ChangeRequestProposal, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, in)
	p.mu.Unlock()
	return p.answer(in)
}

func (p *scriptedProposer) calls() []domain.ChangeRequestInput {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]domain.ChangeRequestInput(nil), p.inputs...)
}

// stagedLineMatcher matches every description to one catalog product.
type stagedLineMatcher struct{ productID uuid.UUID }

func (m stagedLineMatcher) Match(_ context.Context, _ domain.Tenant, descriptions []string,
) ([]domain.LineMatch, error) {
	matches := make([]domain.LineMatch, len(descriptions))
	for i := range descriptions {
		id := m.productID
		matches[i] = domain.LineMatch{ProductID: &id, MatchStatus: domain.ItemMatchStatusMatched,
			Confidence: decimal.RequireFromString("0.95")}
	}
	return matches, nil
}

func requantify(quantity int64) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		target := in.Items[0].ID
		value := decimal.NewFromInt(quantity)
		return &domain.ChangeRequestProposal{Interpretation: "Pide cambiar la cantidad de cemento.",
			Operations: []domain.ChangeRequestOperation{{
				Type: domain.ChangeRequestOperationUpdateQuantity, TargetItemID: &target, Quantity: &value}}}, nil
	}
}

func requiresSeller(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return &domain.ChangeRequestProposal{Interpretation: "El cliente parece aceptar la cotización.",
		Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}}}, nil
}

func (e *env) batchChangeHandler(proposer domain.ChangeRequestHandler,
	matcher interface {
		Match(context.Context, domain.Tenant, []string) ([]domain.LineMatch, error)
	},
) *services.BatchChangeHandler {
	return services.NewBatchChangeHandler(e.db, repository.NewQuoteRepository(),
		repository.NewQuoteMessageRepository(), repository.NewMessageBatchRepository(),
		repository.NewHandlerDecisionRepository(), repository.NewProductPriceRepository(),
		repository.NewQuoteDiscountRepository(), repository.NewPromotionRepository(), proposer, matcher,
		services.BatchChangeHandlerConfig{ContextMessages: 10, MaxOperations: 10},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// handlerQuote is a quote sent to the customer: v1 frozen with one cement line, 2 × 50.
type handlerQuote struct {
	seed      sendableQuote
	quoteID   uuid.UUID
	sentID    uuid.UUID
	cementID  uuid.UUID
	sentToken string
}

func (e *env) sentHandlerQuote(t *testing.T, name string) handlerQuote {
	t.Helper()
	seed := e.seedSendableQuote(t, name, false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, _ := e.sentQuoteToken(t, service, seed)
	e.dropDiscounts(t, seed.tenant.AccountID)
	quote := handlerQuote{seed: seed, quoteID: seed.draft.Quote.ID,
		sentID: *seed.draft.Quote.CurrentVersionID, sentToken: token}
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT product_id FROM quote_item WHERE version_id = $1`, quote.sentID).
		Scan(&quote.cementID); err != nil {
		t.Fatalf("read the sent product: %v", err)
	}
	return quote
}

// claimedBatch writes a batch already claimed for processing, holding bodies as CLIENT messages.
func (e *env) claimedBatch(t *testing.T, accountID, quoteID uuid.UUID, bodies ...string,
) domain.MessageBatch {
	t.Helper()
	ctx := context.Background()
	batch := domain.MessageBatch{ID: uuid.New(), AccountID: accountID, QuoteID: quoteID,
		Status: domain.MessageBatchStatusProcessing}
	if err := e.db.CrossAccount().QueryRow(ctx,
		`INSERT INTO message_batch (id, account_id, quote_id, status, opened_at, closes_at, closed_at,
		                            processing_started_at)
		 VALUES ($1, $2, $3, 'PROCESSING', now() - interval '3 minutes', now() - interval '1 minute',
		         now() - interval '1 minute', now())
		 RETURNING closed_at, processing_started_at`,
		batch.ID, accountID, quoteID).Scan(&batch.ClosedAt, &batch.ProcessingStartedAt); err != nil {
		t.Fatalf("seed claimed batch: %v", err)
	}
	for i, body := range bodies {
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_message (account_id, quote_id, batch_id, author_type, body, received_at)
			 VALUES ($1, $2, $3, 'CLIENT', $4, now() - interval '2 minutes' + make_interval(secs => $5))`,
			accountID, quoteID, batch.ID, body, i); err != nil {
			t.Fatalf("seed batch message: %v", err)
		}
	}
	return batch
}

func (e *env) finishBatch(t *testing.T, batchID uuid.UUID) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE message_batch SET status = 'PROCESSED', processed_at = now() WHERE id = $1`,
		batchID); err != nil {
		t.Fatalf("finish batch: %v", err)
	}
}

type quoteState struct {
	status    domain.QuoteStatus
	currentID uuid.UUID
	mutable   bool
	versions  int
}

func (e *env) quoteState(t *testing.T, quoteID uuid.UUID) quoteState {
	t.Helper()
	var state quoteState
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT q.current_status, q.current_version_id, NOT v.is_immutable,
		        (SELECT count(*) FROM quote_version WHERE quote_id = q.id)
		 FROM quote q JOIN quote_version v ON v.id = q.current_version_id WHERE q.id = $1`,
		quoteID).Scan(&state.status, &state.currentID, &state.mutable, &state.versions); err != nil {
		t.Fatalf("read quote state: %v", err)
	}
	return state
}

type storedDecision struct {
	versionID uuid.UUID
	batchID   *uuid.UUID
	input     string
	state     domain.QuoteStatus
	outcome   string
	reason    *string
}

func (e *env) decisionsOf(t *testing.T, quoteID uuid.UUID) []storedDecision {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT d.quote_version_id, d.message_batch_id, d.client_input, d.state_at_decision,
		        d.ai_proposal::jsonb ->> 'outcome', d.ai_proposal::jsonb ->> 'no_match_reason'
		 FROM handler_decision d JOIN quote_version v ON v.id = d.quote_version_id
		 WHERE v.quote_id = $1 ORDER BY d.created_at, d.id`, quoteID)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	decisions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedDecision, error) {
		var d storedDecision
		err := row.Scan(&d.versionID, &d.batchID, &d.input, &d.state, &d.outcome, &d.reason)
		return d, err
	})
	if err != nil {
		t.Fatalf("scan decisions: %v", err)
	}
	return decisions
}

func (e *env) systemFlips(t *testing.T, quoteID uuid.UUID) int {
	t.Helper()
	var flips int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM quote_status_change WHERE quote_id = $1 AND previous_status = 'SENT'
		   AND new_status = 'CHANGE_REQUESTED' AND user_id IS NULL`, quoteID).Scan(&flips); err != nil {
		t.Fatalf("read status history: %v", err)
	}
	return flips
}

func (e *env) cementPromotion(t *testing.T, quote handlerQuote) {
	t.Helper()
	admin := e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin)
	e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
		"name": "Cemento 10%", "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": "10", "priority": 5,
		"conditions": []map[string]any{{"product_id": quote.cementID}},
	})
}

/*
 * The first batch on a SENT quote moves it to CHANGE_REQUESTED as the system, opens the single
 * draft, applies the proposal to the draft's own lines and prices it through the sweep alone.
 * A second batch edits that same draft and reads the first one as context.
 */
func TestConversationHandler_BatchesAccumulateOnTheSingleDraft(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler accumulates")
	e.cementPromotion(t, quote)
	proposer := &scriptedProposer{answer: requantify(4)}
	sand := e.seedPricedProduct(t, quote.seed.tenant.AccountID, quote.seed.tenant.BranchID,
		"Arena gruesa", "30", nil)
	// The added line points at the sand, so it goes before the product's own teardown.
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, sand) })
	handler := e.batchChangeHandler(proposer, stagedLineMatcher{productID: sand})
	first := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "mejor que sean 4 bolsas",
		"de cemento")

	if err := handler.Handle(context.Background(), first); err != nil {
		t.Fatalf("Handle(first) = %v", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusChangeRequested || !state.mutable || state.versions != 2 {
		t.Fatalf("quote = %+v, want CHANGE_REQUESTED on a fresh mutable v2", state)
	}
	if got := e.systemFlips(t, quote.quoteID); got != 1 {
		t.Errorf("system SENT -> CHANGE_REQUESTED moves = %d, want 1", got)
	}
	inputs := proposer.calls()
	if len(inputs) != 1 || inputs[0].Message != "mejor que sean 4 bolsas\nde cemento" ||
		len(inputs[0].Items) != 1 {
		t.Fatalf("model input = %+v, want both messages in order and the draft's line", inputs)
	}
	var sentLine uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM quote_item WHERE version_id = $1`, quote.sentID).Scan(&sentLine); err != nil {
		t.Fatal(err)
	}
	if inputs[0].Items[0].ID == sentLine {
		t.Error("the model was offered the sent version's line, want the draft's own")
	}
	lines := e.storedLines(t, state.currentID)
	if len(lines) != 1 || !lines[0].quantity.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("draft lines = %+v, want the cement at 4", lines)
	}
	assertStored(t, "draft subtotal", lines[0].subtotal, "200.00")
	// 4 × 50 = 200, and the sweep earns the 10% promotion: 200 − 20 = 180.
	if got := e.storedVersionTotal(t, state.currentID); !got.Equal(decimal.NewFromInt(180)) {
		t.Errorf("draft total = %s, want 180 from the sweep", got)
	}
	if got := e.storedVersionTotal(t, quote.sentID); !got.Equal(decimal.NewFromInt(100)) {
		t.Errorf("sent total = %s, want the frozen 100", got)
	}
	decisions := e.decisionsOf(t, quote.quoteID)
	if len(decisions) != 1 || decisions[0].versionID != state.currentID ||
		decisions[0].batchID == nil || *decisions[0].batchID != first.ID ||
		decisions[0].state != domain.QuoteStatusSent || decisions[0].outcome != "APPLIED" {
		t.Fatalf("decisions = %+v, want one APPLIED on the draft, tied to the batch, taken on SENT",
			decisions)
	}

	e.finishBatch(t, first.ID)
	quantity := decimal.NewFromInt(3)
	description := "arena gruesa"
	proposer.answer = func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		return &domain.ChangeRequestProposal{Interpretation: "Suma arena.",
			Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationAddItem,
				RequestedDescription: &description, Quantity: &quantity}}}, nil
	}
	second := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "y 3 de arena")
	if err := handler.Handle(context.Background(), second); err != nil {
		t.Fatalf("Handle(second) = %v", err)
	}
	after := e.quoteState(t, quote.quoteID)
	if after.currentID != state.currentID || after.versions != 2 || !after.mutable {
		t.Fatalf("after the second batch = %+v, want the same draft %s", after, state.currentID)
	}
	if got := e.systemFlips(t, quote.quoteID); got != 1 {
		t.Errorf("status moves after the second batch = %d, want still 1", got)
	}
	if got := e.storedLines(t, state.currentID); len(got) != 2 {
		t.Errorf("draft lines = %+v, want cement and the added sand", got)
	}
	if history := proposer.calls()[1].Context; len(history) != 2 ||
		history[0].Body != "mejor que sean 4 bolsas" {
		t.Errorf("second batch context = %+v, want the first batch's messages", history)
	}
	decisions = e.decisionsOf(t, quote.quoteID)
	if len(decisions) != 2 || decisions[1].state != domain.QuoteStatusChangeRequested ||
		*decisions[1].batchID != second.ID {
		t.Errorf("decisions = %+v, want the second taken on CHANGE_REQUESTED", decisions)
	}
}

// "dale, acepto" is never an acceptance: the model's escape becomes a no-match for the seller and
// the quote stays in CHANGE_REQUESTED with its draft untouched.
func TestConversationHandler_AcceptLikeTextIsANoMatch(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler accept text")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale, acepto")

	if err := e.batchChangeHandler(&scriptedProposer{answer: requiresSeller}, nil).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusChangeRequested {
		t.Errorf("status = %s, want CHANGE_REQUESTED: the AI never accepts", state.status)
	}
	if lines := e.storedLines(t, state.currentID); len(lines) != 1 ||
		!lines[0].quantity.Equal(decimal.NewFromInt(2)) {
		t.Errorf("draft = %+v, want the sent line unchanged", lines)
	}
	decisions := e.decisionsOf(t, quote.quoteID)
	if len(decisions) != 1 || decisions[0].outcome != "NO_MATCH" || decisions[0].reason == nil ||
		*decisions[0].reason != "REQUIRES_SELLER" {
		t.Errorf("decisions = %+v, want one REQUIRES_SELLER no-match", decisions)
	}
}

// With no model bound the batch still finishes: the quote moves, the draft opens, and a no-match
// tells the seller why. Handle returning nil is what lets the queue mark it PROCESSED.
func TestConversationHandler_AIDisabledLeavesTheBatchToTheSeller(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler AI disabled")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "sacá el cemento")

	if err := e.batchChangeHandler(nil, nil).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v, want nil so the batch is not retried", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusChangeRequested || !state.mutable {
		t.Errorf("quote = %+v, want CHANGE_REQUESTED with a draft for the seller", state)
	}
	decisions := e.decisionsOf(t, quote.quoteID)
	if len(decisions) != 1 || decisions[0].reason == nil || *decisions[0].reason != "AI_DISABLED" ||
		decisions[0].input != "sacá el cemento" {
		t.Errorf("decisions = %+v, want one AI_DISABLED no-match", decisions)
	}
}

// The matrix blocks the handler on a closed or archived quote: the batch fails at once and
// nothing is written.
func TestConversationHandler_MatrixRefusesClosedAndArchivedQuotes(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"archived", `UPDATE quote SET archived_at = now() WHERE id = $1`},
		{"accepted", `UPDATE quote SET current_status = 'ACCEPTED' WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quote := e.sentHandlerQuote(t, "Handler refuses "+tc.name)
			if _, err := e.db.CrossAccount().Exec(context.Background(), tc.sql, quote.quoteID); err != nil {
				t.Fatal(err)
			}
			batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "sumá 2")
			proposer := &scriptedProposer{answer: requantify(4)}

			err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch)
			if !errors.Is(err, domain.ErrBatchUnprocessable) {
				t.Fatalf("Handle() = %v, want ErrBatchUnprocessable", err)
			}
			state := e.quoteState(t, quote.quoteID)
			if state.currentID != quote.sentID || state.versions != 1 || len(proposer.calls()) != 0 ||
				len(e.decisionsOf(t, quote.quoteID)) != 0 {
				t.Errorf("quote = %+v, model calls = %d; want nothing opened, asked or recorded",
					state, len(proposer.calls()))
			}
		})
	}
}

// The matrix is checked again under the quote's lock: a quote archived while the model answered
// takes no change.
func TestConversationHandler_MatrixIsRecheckedInsideTheTransaction(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler recheck")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE quote SET archived_at = now() WHERE id = $1`, quote.quoteID); err != nil {
			return nil, err
		}
		return requantify(4)(in)
	}}

	err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch)
	if !errors.Is(err, domain.ErrBatchUnprocessable) {
		t.Fatalf("Handle() = %v, want ErrBatchUnprocessable", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if lines := e.storedLines(t, state.currentID); !lines[0].quantity.Equal(decimal.NewFromInt(2)) ||
		len(e.decisionsOf(t, quote.quoteID)) != 0 {
		t.Errorf("draft = %+v, want the archived quote's draft untouched and no decision", lines)
	}
}

// A claim reclaimed while the model answered is no longer this run's: nothing is written.
func TestConversationHandler_ALostClaimWritesNothing(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler fence")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE message_batch SET status = 'CLOSED', attempts = 1 WHERE id = $1`, batch.ID); err != nil {
			return nil, err
		}
		return requantify(4)(in)
	}}

	if err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch); err == nil {
		t.Fatal("Handle() = nil, want the lost claim reported")
	}
	state := e.quoteState(t, quote.quoteID)
	if lines := e.storedLines(t, state.currentID); !lines[0].quantity.Equal(decimal.NewFromInt(2)) ||
		len(e.decisionsOf(t, quote.quoteID)) != 0 {
		t.Errorf("draft = %+v, want nothing written by a run that lost its claim", lines)
	}
}

// A draft the seller sent while the model answered is not the draft the proposal was made for:
// the batch goes back to the queue instead of recording against a frozen version.
func TestConversationHandler_ADraftSentMeanwhileIsRetried(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler draft moved")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale")
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE quote_version SET is_immutable = TRUE, frozen_at = now()
			 WHERE quote_id = $1 AND is_immutable = FALSE`, quote.quoteID); err != nil {
			return nil, err
		}
		return requiresSeller(in)
	}}

	err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch)
	if err == nil || errors.Is(err, domain.ErrBatchUnprocessable) {
		t.Fatalf("Handle() = %v, want a retryable error", err)
	}
	if got := e.decisionsOf(t, quote.quoteID); len(got) != 0 {
		t.Errorf("decisions = %+v, want none recorded on a frozen version", got)
	}
}

/*
 * A seller editing the draft holds the quote's lock; the handler waits for it rather than writing
 * underneath, then finds the line it targeted gone and returns the batch to the queue. The retry
 * proposes against the draft as the seller left it.
 */
func TestConversationHandler_WaitsForASellerEditingTheDraft(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler seller edit")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	ctx := context.Background()
	var seller pgx.Tx
	var target uuid.UUID
	locked := make(chan struct{})
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		tx, err := e.db.AdminTx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM quote WHERE id = $1 FOR UPDATE`, quote.quoteID); err != nil {
			return nil, err
		}
		seller, target = tx, in.Items[0].ID
		close(locked)
		return requantify(4)(in)
	}}
	handler := e.batchChangeHandler(proposer, nil)

	done := make(chan error, 1)
	go func() { done <- handler.Handle(ctx, batch) }()
	<-locked
	// Released however the test ends, or the teardown would wait on the seller's lock.
	t.Cleanup(func() { _ = seller.Rollback(context.Background()) })
	select {
	case err := <-done:
		t.Fatalf("Handle() returned %v while the seller held the quote, want it to wait", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := seller.Exec(ctx, `DELETE FROM quote_item WHERE id = $1`, target); err != nil {
		t.Fatal(err)
	}
	if err := seller.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil || errors.Is(err, domain.ErrBatchUnprocessable) {
		t.Fatalf("Handle() = %v, want a retryable error for the line the seller removed", err)
	}
	if got := e.decisionsOf(t, quote.quoteID); len(got) != 0 {
		t.Fatalf("decisions = %+v, want none from the refused run", got)
	}

	proposer.answer = requiresSeller
	if err := handler.Handle(ctx, batch); err != nil {
		t.Fatalf("retry Handle() = %v", err)
	}
	calls := proposer.calls()
	if len(calls[1].Items) != 0 {
		t.Errorf("retry offered %d lines, want the draft as the seller left it", len(calls[1].Items))
	}
	if got := e.decisionsOf(t, quote.quoteID); len(got) != 1 {
		t.Errorf("decisions after the retry = %d, want 1", len(got))
	}
}

// A batch names its account; a batch claiming another account's quote finds nothing there.
func TestConversationHandler_StaysInTheBatchAccount(t *testing.T) {
	e := newEnv(t)
	victim := e.sentHandlerQuote(t, "Handler victim")
	attacker, _ := e.seedAccount(t, "Handler attacker")
	started := time.Now()
	batch := domain.MessageBatch{ID: uuid.New(), AccountID: attacker, QuoteID: victim.quoteID,
		Status: domain.MessageBatchStatusProcessing, ProcessingStartedAt: &started}

	err := e.batchChangeHandler(&scriptedProposer{answer: requantify(4)}, nil).
		Handle(context.Background(), batch)
	if !errors.Is(err, domain.ErrBatchUnprocessable) {
		t.Fatalf("Handle() = %v, want ErrBatchUnprocessable", err)
	}
	if state := e.quoteState(t, victim.quoteID); state.status != domain.QuoteStatusSent ||
		state.versions != 1 {
		t.Errorf("victim quote = %+v, want it untouched", state)
	}
}

/*
 * The webapp's «Pedir cambios» answers the customer at once and queues the message in a batch
 * closed in the same transaction; the handler then applies it to the draft that request opened.
 */
func TestConversationHandler_WebappRequestJoinsTheQueue(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Handler webapp")
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	message := "Necesito 4 bolsas"

	result, err := service.RespondPublic(context.Background(), quote.sentToken,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
	if err != nil {
		t.Fatalf("RespondPublic() = %v", err)
	}
	if result.QuoteStatus != domain.QuoteStatusChangeRequested ||
		result.CustomerStatus != domain.ClientActionRequestChange {
		t.Errorf("result = %+v, want the request acknowledged", result)
	}
	var batch domain.MessageBatch
	var stamped int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT b.id, b.account_id, b.quote_id, b.status, b.closed_at,
		        (SELECT count(*) FROM quote_message m
		          WHERE m.batch_id = b.id AND m.client_action_id IS NOT NULL AND m.body = $2)
		 FROM message_batch b WHERE b.quote_id = $1`, quote.quoteID, message).
		Scan(&batch.ID, &batch.AccountID, &batch.QuoteID, &batch.Status, &batch.ClosedAt,
			&stamped); err != nil {
		t.Fatalf("read the queued batch: %v", err)
	}
	if batch.Status != domain.MessageBatchStatusClosed || batch.ClosedAt == nil || stamped != 1 {
		t.Fatalf("batch = %+v with %d stamped messages, want one CLOSED batch holding the request",
			batch, stamped)
	}
	opened := e.quoteState(t, quote.quoteID)

	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`UPDATE message_batch SET status = 'PROCESSING', processing_started_at = now()
		 WHERE id = $1 RETURNING processing_started_at`, batch.ID).
		Scan(&batch.ProcessingStartedAt); err != nil {
		t.Fatal(err)
	}
	proposer := &scriptedProposer{answer: requantify(4)}
	if err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.currentID != opened.currentID || state.versions != 2 {
		t.Errorf("after handling = %+v, want the draft the request opened (%s)", state, opened.currentID)
	}
	if calls := proposer.calls(); len(calls) != 1 || calls[0].Message != message {
		t.Errorf("model input = %+v, want the webapp message", calls)
	}
	decisions := e.decisionsOf(t, quote.quoteID)
	if len(decisions) != 1 || decisions[0].state != domain.QuoteStatusChangeRequested {
		t.Errorf("decisions = %+v, want one taken on the CHANGE_REQUESTED the request set", decisions)
	}
}
