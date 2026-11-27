//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// capturedOutcomes stands in for the seller's mail.
type capturedOutcomes struct {
	mu       sync.Mutex
	outcomes []domain.ClientQuoteOutcome
}

func (c *capturedOutcomes) NotifySellerOfOutcome(_ context.Context, _ uuid.UUID,
	outcome domain.ClientQuoteOutcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcomes = append(c.outcomes, outcome)
}

func acceptedReply(confidence int) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		return &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelNoMatch,
			Interpretation: "El cliente acepta la cotización.", CustomerDecision: domain.CustomerDecisionAccept,
			DecisionConfidencePercent: confidence,
			Operations:                []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}}}, nil
	}
}

func (e *env) acceptingHandler(answer func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error),
	notifier *capturedOutcomes,
) *services.BatchChangeHandler {
	return e.batchChangeHandler(&scriptedProposer{answer: answer}, nil).WithReplyAcceptance(
		domain.ReplyAcceptancePolicy{AutomaticMinConfidencePercent: 90, ReviewMinConfidencePercent: 60,
			MaxWords: 8},
		repository.NewQuoteSendRepository(), repository.NewClientActionRepository(),
		repository.NewQuoteRepository(), notifier)
}

func (e *env) acceptanceQuoteService() *services.QuoteService {
	quoteRepo := repository.NewQuoteRepository()
	return services.NewQuoteService(e.db, quoteRepo, repository.NewProductPriceRepository(),
		domain.ARCAEnvironmentHomologation, nil).
		WithDiscountEngine(repository.NewQuoteDiscountRepository(), repository.NewPromotionRepository(),
			quoteRepo).
		WithReplyAcceptance(quoteRepo, repository.NewClientActionRepository(),
			repository.NewQuoteSendRepository(), repository.NewQuoteDiscountRepository(),
			repository.NewHandlerDecisionRepository())
}

type storedAcceptance struct {
	unconfirmed bool
	actions     int
	batchID     *uuid.UUID
	revoked     bool
	systemMoves int
}

func (e *env) acceptanceOf(t *testing.T, quoteID uuid.UUID) storedAcceptance {
	t.Helper()
	var got storedAcceptance
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT q.acceptance_unconfirmed_at IS NOT NULL,
		        (SELECT count(*) FROM client_action a JOIN quote_version v ON v.id = a.version_id
		          WHERE v.quote_id = q.id AND a.type = 'ACCEPT'),
		        (SELECT a.message_batch_id FROM client_action a JOIN quote_version v ON v.id = a.version_id
		          WHERE v.quote_id = q.id AND a.type = 'ACCEPT' ORDER BY a.created_at DESC LIMIT 1),
		        COALESCE((SELECT a.revoked_at IS NOT NULL FROM client_action a
		          JOIN quote_version v ON v.id = a.version_id
		          WHERE v.quote_id = q.id AND a.type = 'ACCEPT' ORDER BY a.created_at DESC LIMIT 1), FALSE),
		        (SELECT count(*) FROM quote_status_change c WHERE c.quote_id = q.id
		          AND c.new_status = 'ACCEPTED' AND c.user_id IS NULL)
		 FROM quote q WHERE q.id = $1`, quoteID).
		Scan(&got.unconfirmed, &got.actions, &got.batchID, &got.revoked, &got.systemMoves); err != nil {
		t.Fatalf("read the acceptance: %v", err)
	}
	return got
}

// A plain, sure «dale» on a quote out for an answer is accepted as the customer's own click would.
func TestReplyAcceptance_APlainSureAcceptanceIsAutomatic(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply accepts")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale, lo llevo")
	notifier := &capturedOutcomes{}

	if err := e.acceptingHandler(acceptedReply(95), notifier).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusAccepted || state.currentID != quote.sentID || state.versions != 1 {
		t.Fatalf("quote = %+v, want ACCEPTED on the sent version", state)
	}
	got := e.acceptanceOf(t, quote.quoteID)
	if got.unconfirmed || got.actions != 1 || got.batchID == nil || *got.batchID != batch.ID ||
		got.systemMoves != 1 {
		t.Errorf("acceptance = %+v, want one standing action tied to the batch, by the system", got)
	}
	if len(notifier.outcomes) != 1 || !notifier.outcomes[0].ByReply || notifier.outcomes[0].Unconfirmed {
		t.Errorf("seller told = %+v, want one automatic acceptance", notifier.outcomes)
	}
	e.finishBatch(t, batch.ID)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	replay, err := service.RespondPublic(context.Background(), quote.sentToken,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if err != nil || replay.CustomerStatus != domain.ClientActionAccept ||
		replay.QuoteStatus != domain.QuoteStatusAccepted {
		t.Errorf("portal answer = %+v, %v, want the standing acceptance replayed", replay, err)
	}
}

// A less sure acceptance is accepted pending the seller: it cannot be paid until confirmed.
func TestReplyAcceptance_AnUnsureAcceptanceWaitsForTheSeller(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply accepts unsure")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale")
	notifier := &capturedOutcomes{}

	if err := e.acceptingHandler(acceptedReply(75), notifier).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	if got := e.acceptanceOf(t, quote.quoteID); !got.unconfirmed || got.actions != 1 {
		t.Fatalf("acceptance = %+v, want ACCEPTED pending confirmation", got)
	}
	if len(notifier.outcomes) != 1 || !notifier.outcomes[0].Unconfirmed {
		t.Errorf("seller told = %+v, want an acceptance to confirm", notifier.outcomes)
	}
	quotes := e.acceptanceQuoteService()
	tenant := quote.seed.tenant
	if _, err := quotes.MarkPaid(context.Background(), tenant, quote.quoteID); domain.CodeOf(err) !=
		domain.CodeQuoteAcceptanceUnconfirmed {
		t.Fatalf("MarkPaid() = %v, want QUOTE_ACCEPTANCE_UNCONFIRMED", err)
	}
	confirmed, err := quotes.ConfirmAcceptance(context.Background(), tenant, quote.quoteID)
	if err != nil || confirmed.AcceptanceUnconfirmedAt != nil {
		t.Fatalf("ConfirmAcceptance() = %+v, %v, want the mark cleared", confirmed, err)
	}
	if _, err := quotes.MarkPaid(context.Background(), tenant, quote.quoteID); err != nil {
		t.Errorf("MarkPaid() after confirming = %v, want it paid", err)
	}
}

// Undoing an acceptance still waiting for confirmation drops the pending mark with it.
func TestReplyAcceptance_UndoClearsAPendingConfirmation(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply undo unconfirmed")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale")
	if err := e.acceptingHandler(acceptedReply(70), &capturedOutcomes{}).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	undone, err := e.acceptanceQuoteService().UndoAcceptance(context.Background(), quote.seed.tenant,
		quote.quoteID)
	if err != nil || undone.CurrentStatus != domain.QuoteStatusSent || undone.AcceptanceUnconfirmedAt != nil {
		t.Fatalf("UndoAcceptance() = %+v, %v, want SENT with no pending mark", undone, err)
	}
}

// A reply that could have meant another live quote only raises a signal.
func TestReplyAcceptance_AnotherLiveQuoteKeepsItSent(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply accepts ambiguous")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale")
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_message SET other_live_quotes = 1 WHERE batch_id = $1`, batch.ID); err != nil {
		t.Fatal(err)
	}
	notifier := &capturedOutcomes{}
	if err := e.acceptingHandler(acceptedReply(97), notifier).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	if state := e.quoteState(t, quote.quoteID); state.status != domain.QuoteStatusSent {
		t.Errorf("status = %s, want SENT with a signal", state.status)
	}
	if got := e.acceptanceOf(t, quote.quoteID); got.actions != 0 || len(notifier.outcomes) != 0 {
		t.Errorf("acceptance = %+v, told %+v, want nothing recorded as accepted", got, notifier.outcomes)
	}
	var tier string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT d.ai_proposal::jsonb ->> 'reply_acceptance' FROM handler_decision d
		 WHERE d.message_batch_id = $1`, batch.ID).Scan(&tier); err != nil || tier != "SIGNAL" {
		t.Errorf("recorded acceptance = %q, %v, want SIGNAL", tier, err)
	}
}

// The seller undoes an acceptance read from a reply; the portal then takes an answer again.
func TestReplyAcceptance_UndoReopensThePortal(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply accepts undo")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "listo")
	if err := e.acceptingHandler(acceptedReply(95), &capturedOutcomes{}).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	e.finishBatch(t, batch.ID)
	quotes := e.acceptanceQuoteService()

	undone, err := quotes.UndoAcceptance(context.Background(), quote.seed.tenant, quote.quoteID)
	if err != nil || undone.CurrentStatus != domain.QuoteStatusSent {
		t.Fatalf("UndoAcceptance() = %+v, %v, want SENT", undone, err)
	}
	if got := e.acceptanceOf(t, quote.quoteID); !got.revoked {
		t.Errorf("acceptance = %+v, want the action revoked", got)
	}
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	result, err := service.RespondPublic(context.Background(), quote.sentToken,
		domain.ClientActionInput{Type: domain.ClientActionReject})
	if err != nil || result.QuoteStatus != domain.QuoteStatusRejected {
		t.Errorf("portal answer after the undo = %+v, %v, want it taken", result, err)
	}
	if _, err := quotes.UndoAcceptance(context.Background(), quote.seed.tenant, quote.quoteID); !errors.Is(err,
		domain.ErrConflict) {
		t.Errorf("a second undo = %v, want a conflict", err)
	}
}

// A paid sale, or an acceptance the customer clicked, cannot be undone.
func TestReplyAcceptance_UndoRefusesWhatTheSystemDidNotAccept(t *testing.T) {
	e := newEnv(t)
	quotes := e.acceptanceQuoteService()

	clicked := e.sentHandlerQuote(t, "Reply undo clicked")
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	if _, err := service.RespondPublic(context.Background(), clicked.sentToken,
		domain.ClientActionInput{Type: domain.ClientActionAccept}); err != nil {
		t.Fatal(err)
	}
	if _, err := quotes.UndoAcceptance(context.Background(), clicked.seed.tenant, clicked.quoteID); domain.CodeOf(err) !=
		domain.CodeQuoteAcceptanceNotUndoable {
		t.Errorf("undo of a clicked acceptance = %v, want QUOTE_ACCEPTANCE_NOT_UNDOABLE", err)
	}

	paid := e.sentHandlerQuote(t, "Reply undo paid")
	batch := e.claimedBatch(t, paid.seed.tenant.AccountID, paid.quoteID, "dale")
	if err := e.acceptingHandler(acceptedReply(95), &capturedOutcomes{}).
		Handle(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	if _, err := quotes.MarkPaid(context.Background(), paid.seed.tenant, paid.quoteID); err != nil {
		t.Fatal(err)
	}
	if _, err := quotes.UndoAcceptance(context.Background(), paid.seed.tenant, paid.quoteID); domain.CodeOf(err) !=
		domain.CodeQuoteAcceptanceNotUndoable {
		t.Errorf("undo of a paid sale = %v, want QUOTE_ACCEPTANCE_NOT_UNDOABLE", err)
	}
}

// A quote stuck in CHANGE_REQUESTED is accepted on the version its customer received; the draft goes.
func TestReplyAcceptance_AcceptTheSentVersionFromAChangeRequest(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Reply accept sent version")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale pero que sean 4")
	if err := e.batchChangeHandler(&scriptedProposer{answer: requantify(4)}, nil).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	e.finishBatch(t, batch.ID)
	if state := e.quoteState(t, quote.quoteID); state.status != domain.QuoteStatusChangeRequested ||
		state.versions != 2 {
		t.Fatalf("quote = %+v, want CHANGE_REQUESTED with a draft", state)
	}

	// The seller previewed the draft: its representation goes with it.
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_representation (account_id, branch_id, quote_id, version_id, schema_version,
		   payload, message, pdf_storage_key, pdf_content_type, pdf_size_bytes, pdf_sha256,
		   logo_fallback_used, selection_pricing_snapshot)
		 SELECT r.account_id, r.branch_id, r.quote_id, q.current_version_id, r.schema_version, r.payload,
		   r.message, r.pdf_storage_key, r.pdf_content_type, r.pdf_size_bytes, r.pdf_sha256,
		   r.logo_fallback_used, r.selection_pricing_snapshot
		 FROM quote_representation r JOIN quote q ON q.id = r.quote_id
		 WHERE r.version_id = $1`, quote.sentID); err != nil {
		t.Fatalf("seed the draft's preview: %v", err)
	}

	accepted, err := e.acceptanceQuoteService().AcceptSentVersion(context.Background(),
		quote.seed.tenant, quote.quoteID)
	if err != nil || accepted.CurrentStatus != domain.QuoteStatusAccepted {
		t.Fatalf("AcceptSentVersion() = %+v, %v, want ACCEPTED", accepted, err)
	}
	state := e.quoteState(t, quote.quoteID)
	if state.currentID != quote.sentID || state.versions != 1 || state.mutable {
		t.Errorf("quote = %+v, want the sent version alone", state)
	}
	decisions := e.decisionsOf(t, quote.quoteID)
	var settled int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM handler_decision WHERE quote_version_id = $1
		   AND seller_decision = 'REJECTED'`, quote.sentID).Scan(&settled); err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].versionID != quote.sentID || settled != 1 {
		t.Errorf("decisions = %+v (%d rejected), want the draft's run kept on the sent version, rejected",
			decisions, settled)
	}
	var previews int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM quote_representation WHERE quote_id = $1`, quote.quoteID).
		Scan(&previews); err != nil || previews != 1 {
		t.Errorf("representations = %d, %v, want only the sent version's", previews, err)
	}

	// A frozen version's representation stays out of the application's reach.
	var deleted int64
	if err := e.db.InTenantTx(context.Background(), quote.seed.tenant, func(q repository.Querier) error {
		tag, err := q.Exec(context.Background(),
			`DELETE FROM quote_representation WHERE version_id = $1`, quote.sentID)
		deleted = tag.RowsAffected()
		return err
	}); err != nil || deleted != 0 {
		t.Errorf("deleting the sent representation = %d rows, %v, want none", deleted, err)
	}
}
