//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type settledDecision struct {
	outcome   string
	decision  *string
	userID    *uuid.UUID
	decidedAt *time.Time
}

func (d settledDecision) String() string {
	decision := "undecided"
	if d.decision != nil {
		decision = *d.decision
	}
	return fmt.Sprintf("%s→%s (by user: %t, dated: %t)", d.outcome, decision, d.userID != nil,
		d.decidedAt != nil)
}

func (e *env) settledDecisionsOf(t *testing.T, quoteID uuid.UUID) []settledDecision {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT d.ai_proposal::jsonb ->> 'outcome', d.seller_decision, d.user_id, d.decided_at
		 FROM handler_decision d JOIN quote_version v ON v.id = d.quote_version_id
		 WHERE v.quote_id = $1 ORDER BY d.created_at, d.id`, quoteID)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	decisions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (settledDecision, error) {
		var d settledDecision
		err := row.Scan(&d.outcome, &d.decision, &d.userID, &d.decidedAt)
		return d, err
	})
	if err != nil {
		t.Fatalf("scan decisions: %v", err)
	}
	return decisions
}

// resendDraft prices the change-requested draft and sends it, which freezes it.
func (e *env) resendDraft(t *testing.T, quote handlerQuote, token string) {
	t.Helper()
	e.acceptMaterials(t, quote.quoteID, token, quote.seed.tenant.BranchID.String())
	if _, err := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil).
		Send(context.Background(), quote.seed.tenant, quote.quoteID,
			domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199"}); err != nil {
		t.Fatalf("resend: %v", err)
	}
}

type conversationBody struct {
	Batches []struct {
		ID            uuid.UUID `json:"id"`
		Status        string    `json:"status"`
		FailureReason *string   `json:"failure_reason"`
		Retryable     bool      `json:"retryable"`
		Messages      []struct {
			Author string `json:"author"`
			Body   string `json:"body"`
		} `json:"messages"`
		Decision *struct {
			Interpretation string  `json:"interpretation"`
			Outcome        string  `json:"outcome"`
			SellerDecision *string `json:"seller_decision"`
		} `json:"decision"`
	} `json:"batches"`
	Pending []struct {
		Body string `json:"body"`
	} `json:"pending"`
	CanDiscardAIChanges bool `json:"can_discard_ai_changes"`
}

func (e *env) reviewConversation(t *testing.T, quote handlerQuote, token string) conversationBody {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet,
		path: "/v1/rfqs/" + quote.seed.draft.RFQ.ID.String(), token: token,
		branch: quote.seed.tenant.BranchID.String()})
	var detail struct {
		Conversation *conversationBody `json:"conversation"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &detail) != nil ||
		detail.Conversation == nil {
		t.Fatalf("detail: status = %d, want a conversation: %s", rec.Code, rec.Body)
	}
	return *detail.Conversation
}

/*
 * Sending the reworked draft settles every handler run on it: an applied change the seller left
 * alone is APPROVED_AS_IS even though pricing it changed its figures, a no-match was resolved by
 * hand, and both carry who sent it and when.
 */
func TestConversationReview_SendSettlesEveryRunOnTheDraft(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Review settles")
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	applied := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	if err := e.batchChangeHandler(&scriptedProposer{answer: requantify(4)}, nil).
		Handle(context.Background(), applied); err != nil {
		t.Fatalf("Handle(applied) = %v", err)
	}
	e.finishBatch(t, applied.ID)
	noMatch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "¿y el flete?")
	if err := e.batchChangeHandler(&scriptedProposer{answer: requiresSeller}, nil).
		Handle(context.Background(), noMatch); err != nil {
		t.Fatalf("Handle(noMatch) = %v", err)
	}
	if got := e.settledDecisionsOf(t, quote.quoteID); len(got) != 2 || got[0].decision != nil ||
		got[1].decision != nil {
		t.Fatalf("before the send = %+v, want two undecided runs", got)
	}
	// The cement goes up before the seller prices the draft: repricing is not an edit.
	e.openPricePeriod(t, quote.seed.tenant.AccountID, quote.seed.tenant.BranchID, quote.cementID,
		"70.00", nil)

	e.resendDraft(t, quote, admin)
	if lines := e.storedLines(t, e.quoteState(t, quote.quoteID).currentID); len(lines) != 1 ||
		!lines[0].unitPrice.Valid || !lines[0].unitPrice.Decimal.Equal(decimal.NewFromInt(70)) {
		t.Fatalf("sent lines = %+v, want the cement repriced at 70", lines)
	}

	got := e.settledDecisionsOf(t, quote.quoteID)
	if len(got) != 2 || got[0].decision == nil || *got[0].decision != "APPROVED_AS_IS" ||
		got[1].decision == nil || *got[1].decision != "MANUAL_OVERRIDE" {
		t.Fatalf("after the send = %+v, want APPROVED_AS_IS then MANUAL_OVERRIDE", got)
	}
	for i, decision := range got {
		if decision.userID == nil || *decision.userID != quote.seed.tenant.UserID ||
			decision.decidedAt == nil {
			t.Errorf("decision %d = %+v, want the sender and a decision time", i, decision)
		}
	}
	if state := e.quoteState(t, quote.quoteID); state.status != domain.QuoteStatusSent || state.mutable {
		t.Errorf("quote = %+v, want SENT on a frozen version", state)
	}
}

// A seller's own edit after the AI's change makes that run EDITED.
func TestConversationReview_ASellerEditMarksTheRunEdited(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Review edited")
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	if err := e.batchChangeHandler(&scriptedProposer{answer: requantify(4)}, nil).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	draft := e.quoteState(t, quote.quoteID).currentID
	line := e.lineID(t, draft, "two cement bags")
	if rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + quote.quoteID.String() + "/items/" + line.String(),
		token: admin, branch: quote.seed.tenant.BranchID.String(),
		body: map[string]any{"quantity": "5"}}); rec.Code != http.StatusOK {
		t.Fatalf("seller edit: status = %d: %s", rec.Code, rec.Body)
	}

	e.resendDraft(t, quote, admin)

	if got := e.settledDecisionsOf(t, quote.quoteID); len(got) != 1 || got[0].decision == nil ||
		*got[0].decision != "EDITED" {
		t.Fatalf("decisions = %+v, want EDITED", got)
	}
}

/*
 * Discarding resets the draft to what the customer last received and rejects the AI's pending
 * changes; the quote stays in CHANGE_REQUESTED, and a second discard has nothing to undo.
 */
func TestConversationReview_DiscardResetsTheDraftAndRejectsTheRun(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Review discards")
	e.cementPromotion(t, quote)
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	branch := quote.seed.tenant.BranchID.String()
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "que sean 4")
	if err := e.batchChangeHandler(&scriptedProposer{answer: requantify(4)}, nil).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	draft := e.quoteState(t, quote.quoteID).currentID
	before := e.reviewConversation(t, quote, admin)
	if !before.CanDiscardAIChanges || len(before.Batches) != 1 || before.Batches[0].Decision == nil ||
		before.Batches[0].Decision.Outcome != "APPLIED" || before.Batches[0].Decision.SellerDecision != nil {
		t.Fatalf("conversation = %+v, want one pending APPLIED run the seller can discard", before)
	}

	discard := request{method: http.MethodPost,
		path:  "/v1/quotes/" + quote.quoteID.String() + "/discard-ai-changes",
		token: admin, branch: branch}
	if rec := e.do(t, discard); rec.Code != http.StatusOK {
		t.Fatalf("discard: status = %d: %s", rec.Code, rec.Body)
	}

	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusChangeRequested || state.currentID != draft || !state.mutable {
		t.Fatalf("quote = %+v, want the same draft, still CHANGE_REQUESTED", state)
	}
	lines := e.storedLines(t, draft)
	if len(lines) != 1 || !lines[0].quantity.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("draft has %d lines, the first at %s; want the sent 2 bags back", len(lines),
			lines[0].quantity)
	}
	// 2 × 50 = 100, and the sweep earns the 10% promotion again: 100 − 10 = 90.
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(90)) {
		t.Errorf("draft total = %s, want 90 from the sweep", got)
	}
	decisions := e.settledDecisionsOf(t, quote.quoteID)
	if len(decisions) != 1 || decisions[0].decision == nil || *decisions[0].decision != "REJECTED" ||
		decisions[0].userID == nil || decisions[0].decidedAt == nil {
		t.Fatalf("decisions = %+v, want one REJECTED by the seller", decisions)
	}
	if after := e.reviewConversation(t, quote, admin); after.CanDiscardAIChanges {
		t.Errorf("conversation = %+v, want nothing left to discard", after)
	}
	rec := e.do(t, request{method: http.MethodGet,
		path: "/v1/rfqs/" + quote.seed.draft.RFQ.ID.String(), token: admin, branch: branch})
	var detail struct {
		ChangesRequested *struct {
			Handler *struct {
				SellerDecision *string `json:"seller_decision"`
			} `json:"handler"`
		} `json:"changes_requested"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &detail) != nil || detail.ChangesRequested == nil ||
		detail.ChangesRequested.Handler == nil || detail.ChangesRequested.Handler.SellerDecision == nil ||
		*detail.ChangesRequested.Handler.SellerDecision != "REJECTED" {
		t.Errorf("change request note carries no REJECTED decision (status %d)", rec.Code)
	}
	rec = e.do(t, discard)
	if rec.Code != http.StatusConflict || !containsCode(rec.Body.Bytes(), "NO_AI_CHANGES") {
		t.Errorf("second discard: status = %d, want 409 NO_AI_CHANGES: %s", rec.Code, rec.Body)
	}

	e.resendDraft(t, quote, admin)
	if got := e.settledDecisionsOf(t, quote.quoteID); len(got) != 1 || *got[0].decision != "REJECTED" {
		t.Errorf("after the send = %+v, want the rejection kept", got)
	}
}

// A batch that ran out of attempts goes back to the queue on the seller's word; one the handler
// can never process does not, and neither does any batch for another account.
func TestConversationReview_RetryOnlyRequeuesAnExhaustedBatch(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Review retries")
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	branch := quote.seed.tenant.BranchID.String()
	failed := func(reason domain.MessageBatchFailureReason, body string) uuid.UUID {
		id := uuid.New()
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`INSERT INTO message_batch (id, account_id, quote_id, status, opened_at, closes_at,
			                            closed_at, attempts, last_error, failure_reason)
			 VALUES ($1, $2, $3, 'FAILED', now() - interval '10 minutes', now() - interval '5 minutes',
			         now() - interval '5 minutes', 3, 'model unavailable', $4)`,
			id, quote.seed.tenant.AccountID, quote.quoteID, reason); err != nil {
			t.Fatalf("seed failed batch: %v", err)
		}
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`INSERT INTO quote_message (account_id, quote_id, batch_id, author_type, body)
			 VALUES ($1, $2, $3, 'CLIENT', $4)`,
			quote.seed.tenant.AccountID, quote.quoteID, id, body); err != nil {
			t.Fatalf("seed batch message: %v", err)
		}
		return id
	}
	exhausted := failed(domain.MessageBatchFailureAttemptsExhausted, "sumá arena")
	unprocessable := failed(domain.MessageBatchFailureUnprocessable, "¿llega el lunes?")
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_message (account_id, quote_id, author_type, body)
		 VALUES ($1, $2, 'CLIENT', 'y una carretilla')`,
		quote.seed.tenant.AccountID, quote.quoteID); err != nil {
		t.Fatalf("seed pending message: %v", err)
	}

	conversation := e.reviewConversation(t, quote, admin)
	retryable := map[uuid.UUID]bool{}
	for _, batch := range conversation.Batches {
		retryable[batch.ID] = batch.Retryable
		if len(batch.Messages) != 1 || batch.Messages[0].Author != "CLIENT" {
			t.Errorf("batch %s messages = %+v, want its one customer message", batch.ID, batch.Messages)
		}
	}
	if len(conversation.Batches) != 2 || !retryable[exhausted] || retryable[unprocessable] {
		t.Errorf("batches = %+v, want only the exhausted one retryable", conversation.Batches)
	}
	if len(conversation.Pending) != 1 || conversation.Pending[0].Body != "y una carretilla" {
		t.Errorf("pending = %+v, want the message still in its window", conversation.Pending)
	}

	retry := func(token string, batchID uuid.UUID) *http.Response {
		rec := e.do(t, request{method: http.MethodPost,
			path:  "/v1/quotes/" + quote.quoteID.String() + "/message-batches/" + batchID.String() + "/retry",
			token: token, branch: branch})
		return rec.Result()
	}
	stranger, _ := e.seedAccount(t, "Review stranger")
	strangerToken := e.tokenFor(t, e.seedUser(t, stranger, domain.UserRoleAdmin))
	if got := retry(strangerToken, exhausted); got.StatusCode != http.StatusNotFound &&
		got.StatusCode != http.StatusForbidden {
		t.Errorf("another account's retry: status = %d, want it refused", got.StatusCode)
	}
	if got := e.readBatch(t, exhausted); got.Status != domain.MessageBatchStatusFailed {
		t.Fatalf("after a refused retry batch = %+v, want it untouched", got)
	}
	if got := retry(admin, exhausted); got.StatusCode != http.StatusOK {
		t.Fatalf("retry: status = %d, want 200", got.StatusCode)
	}
	if got := e.readBatch(t, exhausted); got.Status != domain.MessageBatchStatusClosed ||
		got.Attempts != 0 || got.LastError != nil || got.FailureReason != nil {
		t.Errorf("requeued batch = %+v, want CLOSED with a fresh count", got)
	}
	for name, id := range map[string]uuid.UUID{"requeued": exhausted, "unprocessable": unprocessable} {
		if got := retry(admin, id); got.StatusCode != http.StatusConflict {
			t.Errorf("retry %s: status = %d, want 409", name, got.StatusCode)
		}
	}

	// Once the quote is archived the handler may not act on it, so nothing is retried for it.
	late := failed(domain.MessageBatchFailureAttemptsExhausted, "¿me lo reservás?")
	if rec := e.do(t, request{method: http.MethodPost,
		path: "/v1/quotes/" + quote.quoteID.String() + "/archive", token: admin,
		branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("archive: status = %d: %s", rec.Code, rec.Body)
	}
	for _, batch := range e.reviewConversation(t, quote, admin).Batches {
		if batch.ID == late && batch.Retryable {
			t.Errorf("batch on an archived quote = %+v, want it not retryable", batch)
		}
	}
	if got := retry(admin, late); got.StatusCode != http.StatusConflict {
		t.Errorf("retry on an archived quote: status = %d, want 409", got.StatusCode)
	}
	if got := e.readBatch(t, late); got.Status != domain.MessageBatchStatusFailed {
		t.Errorf("batch on an archived quote = %+v, want it left FAILED", got)
	}
}
