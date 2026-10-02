//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// orderedQuantity is what the client asked for; reviewedQuantity is what the seller corrected it
// to. They differ so the priced total can only come out right if the correction reached it.
const (
	orderedQuantity  = "10"
	reviewedQuantity = "12"
	cementUnitPrice  = "1200.50"
	reviewedTotal    = "14406.00"
)

// flowRun is one order carried to the point where only the customer's answer is left.
type flowRun struct {
	draft       *domain.TextRFQDraft
	seller      domain.AppUser
	service     *services.QuoteDeliveryService
	publicToken string
}

type reactivatedQuote struct {
	CurrentVersionID *uuid.UUID `json:"current_version_id"`
	CurrentStatus    string     `json:"current_status"`
}

/*
 * firstOperationalFlow walks a plain-text order through every stage a real one crosses: the
 * pipeline drafts it, the seller corrects a line through the route the backoffice calls, accepts
 * the materials, and sends it. It stops with the quote SENT and the customer's link in hand.
 *
 * The seller's correction goes through the endpoint rather than an UPDATE, because "with human
 * review" is the criterion: a flow that proves a seller could have edited proves nothing about
 * whether the product lets them.
 */
func firstOperationalFlow(t *testing.T, e *env, name string) flowRun {
	t.Helper()
	accountID, branchID := e.seedAccount(t, name)
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento Portland 50kg",
		cementUnitPrice, nil)
	e.embedOn(t, cement, 0, 0.98)

	unit := "bolsa"
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
		Role: domain.UserRoleSeller}
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "10 bolsas de cemento",
		Quantity:             decimal.RequireFromString(orderedQuantity),
		Unit:                 &unit, Source: domain.QuantitySourceExplicit,
		QuantityRationale: "el cliente pidió 10 bolsas",
	}}}, map[string]int{"10 bolsas de cemento": 0}).CreateTextDraft(context.Background(), tenant,
		domain.TextRFQDraftInput{ChannelID: channelID, RawText: "10 bolsas de cemento"})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want the order drafted", err)
	}
	e.dropDraft(t, draft.RFQ.ID)

	if draft.RFQ.Status != domain.RFQStatusGenerated {
		t.Fatalf("rfq status = %q, want GENERATED once the pipeline answered", draft.RFQ.Status)
	}
	if draft.Quote == nil || draft.Quote.CurrentStatus != domain.QuoteStatusDraft {
		t.Fatalf("quote = %+v, want one at DRAFT for the seller to review", draft.Quote)
	}
	if len(draft.Items) != 1 {
		t.Fatalf("drafted %d lines, want the one the order named", len(draft.Items))
	}

	token := e.tokenFor(t, seller)
	rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + draft.Quote.ID.String() + "/items/" + draft.Items[0].ID.String(),
		token: token, branch: branchID.String(),
		body: map[string]any{"quantity": reviewedQuantity}})
	if rec.Code != http.StatusOK {
		t.Fatalf("review PATCH = %d, want 200: %s", rec.Code, rec.Body)
	}
	var reviewed struct {
		Quantity string `json:"quantity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reviewed); err != nil {
		t.Fatalf("decode the reviewed line %s: %v", rec.Body, err)
	}
	if !decimal.RequireFromString(reviewed.Quantity).Equal(
		decimal.RequireFromString(reviewedQuantity)) {
		t.Fatalf("reviewed quantity = %s, want the seller's %s", reviewed.Quantity,
			reviewedQuantity)
	}

	priced := e.acceptMaterials(t, draft.Quote.ID, token, branchID.String())
	if priced.Quote.CurrentStatus != string(domain.QuoteStatusQuoted) {
		t.Fatalf("quote status = %q, want QUOTED once the materials are accepted",
			priced.Quote.CurrentStatus)
	}

	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	publicToken, _ := e.sentQuoteToken(t, service, sendableQuote{tenant: tenant, draft: draft})
	return flowRun{draft: draft, seller: seller, service: service, publicToken: publicToken}
}

/*
 * The first operational flow, end to end: a plain-text order becomes an RFQ, a pre-built quote, a
 * reviewed quote, a sent quote and finally a closed one. Every stage is exercised in the order a
 * real order crosses them, which is what the pieces tested separately cannot show — each of those
 * builds its own starting state, so none of them proves the seam to the stage before it holds.
 */
func TestFirstOperationalFlow_PlainTextOrderReachesAcceptanceWithHumanReview(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "First flow accepted")

	outcome, err := run.service.RecordClientAction(context.Background(), run.publicToken,
		domain.ClientActionAccept)
	if err != nil {
		t.Fatalf("RecordClientAction() = %v, want the customer's answer recorded", err)
	}
	if outcome.Status != domain.QuoteStatusAccepted {
		t.Fatalf("closing status = %q, want ACCEPTED", outcome.Status)
	}

	// The correction is what the money was built from: priced off what the client asked for, the
	// total would be 12005.00, so this is the assertion that puts human review inside the flow
	// rather than beside it.
	if total := e.storedVersionTotal(t, run.draft.Version.ID); !total.Equal(
		decimal.RequireFromString(reviewedTotal)) {
		t.Errorf("total = %s, want %s — the seller's corrected quantity priced", total,
			reviewedTotal)
	}

	assertFlowLedger(t, e, run, domain.QuoteStatusAccepted)
}

// The other closure the customer can reach on their own. REQUEST_CHANGE is deliberately not a
// third case here: it is absent from the client surface until the conversation entities exist,
// so a test for it would be asserting a decision rather than a behaviour.
func TestFirstOperationalFlow_PlainTextOrderReachesRejection(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "First flow rejected")

	outcome, err := run.service.RecordClientAction(context.Background(), run.publicToken,
		domain.ClientActionReject)
	if err != nil {
		t.Fatalf("RecordClientAction() = %v, want the customer's answer recorded", err)
	}
	if outcome.Status != domain.QuoteStatusRejected {
		t.Fatalf("closing status = %q, want REJECTED", outcome.Status)
	}

	assertFlowLedger(t, e, run, domain.QuoteStatusRejected)
}

func TestFirstOperationalFlow_AcceptedQuoteReactivatesForResend(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "Reactivation resend")

	if _, err := run.service.RecordClientAction(context.Background(), run.publicToken,
		domain.ClientActionAccept); err != nil {
		t.Fatalf("RecordClientAction() = %v, want the customer's answer recorded", err)
	}
	reactivated := reactivateThroughAPI(t, e, run, domain.QuoteReactivationResend)

	if reactivated.CurrentStatus != string(domain.QuoteStatusQuoted) {
		t.Errorf("status = %q, want QUOTED", reactivated.CurrentStatus)
	}
	if reactivated.CurrentVersionID == nil || *reactivated.CurrentVersionID != run.draft.Version.ID {
		t.Errorf("current version = %v, want original %v", reactivated.CurrentVersionID,
			run.draft.Version.ID)
	}
	assertLedger(t, e, "quote", `SELECT COALESCE(previous_status::text, '<none>'), new_status::text,
	          user_id
	     FROM quote_status_change WHERE quote_id = $1 ORDER BY changed_at`,
		run.draft.Quote.ID, run.seller.ID,
		[]string{
			"<none>->DRAFT by seller",
			"DRAFT->QUOTED by seller",
			"QUOTED->SENT by seller",
			"SENT->ACCEPTED by nobody",
			"ACCEPTED->QUOTED by seller",
		})
}

func TestFirstOperationalFlow_RejectedQuoteReactivatesWithAnEditableVersion(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "Reactivation edit")

	if _, err := run.service.RecordClientAction(context.Background(), run.publicToken,
		domain.ClientActionReject); err != nil {
		t.Fatalf("RecordClientAction() = %v, want the customer's answer recorded", err)
	}
	reactivated := reactivateThroughAPI(t, e, run, domain.QuoteReactivationEdit)

	if reactivated.CurrentStatus != string(domain.QuoteStatusChangeRequested) {
		t.Errorf("status = %q, want CHANGE_REQUESTED", reactivated.CurrentStatus)
	}
	if reactivated.CurrentVersionID == nil || *reactivated.CurrentVersionID == run.draft.Version.ID {
		t.Fatalf("current version = %v, want a new version", reactivated.CurrentVersionID)
	}
	var versionNumber int
	var immutable bool
	var authorID *uuid.UUID
	err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT version_number, is_immutable, author_id
		 FROM quote_version WHERE id = $1 AND account_id = $2`,
		*reactivated.CurrentVersionID, run.draft.Quote.AccountID).Scan(
		&versionNumber, &immutable, &authorID)
	if err != nil {
		t.Fatalf("read reactivated version: %v", err)
	}
	if versionNumber != 2 || immutable {
		t.Errorf("version = v%d immutable=%v, want mutable v2", versionNumber, immutable)
	}
	if authorID == nil || *authorID != run.seller.ID {
		t.Errorf("version author = %v, want seller %v", authorID, run.seller.ID)
	}
}

func reactivateThroughAPI(t *testing.T, e *env, run flowRun,
	mode domain.QuoteReactivationMode) reactivatedQuote {
	t.Helper()
	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + run.draft.Quote.ID.String() + "/reactivate",
		token:  e.tokenFor(t, run.seller),
		branch: run.draft.Quote.BranchID.String(),
		body:   map[string]any{"mode": mode},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivate POST = %d, want 200: %s", rec.Code, rec.Body)
	}
	var quote reactivatedQuote
	if err := json.Unmarshal(rec.Body.Bytes(), &quote); err != nil {
		t.Fatalf("decode reactivated quote %s: %v", rec.Body, err)
	}
	return quote
}

/*
 * assertFlowLedger pins the whole transition record, in order, with who moved it. Asserting only
 * the closing row would pass on a flow that jumped straight to it, and asserting the rows without
 * their actor would pass on one that credited the customer's answer to the seller who sent it.
 *
 * The two ledgers are read separately rather than unioned and sorted together: the order's move to
 * GENERATED and the quote's birth share a transaction, so they share a changed_at to the
 * microsecond and any ordering across the two is a coin toss.
 */
func assertFlowLedger(t *testing.T, e *env, run flowRun, closing domain.QuoteStatus) {
	t.Helper()
	// The order is born RECEIVED, and being born is not a transition, so its ledger opens at the
	// move the pipeline made rather than at its arrival.
	assertLedger(t, e, "rfq", `SELECT COALESCE(previous_status::text, '<none>'), new_status::text,
	          user_id
	     FROM rfq_status_change WHERE rfq_id = $1 ORDER BY changed_at`,
		run.draft.RFQ.ID, run.seller.ID,
		[]string{"RECEIVED->GENERATED by seller"})

	assertLedger(t, e, "quote", `SELECT COALESCE(previous_status::text, '<none>'), new_status::text,
	          user_id
	     FROM quote_status_change WHERE quote_id = $1 ORDER BY changed_at`,
		run.draft.Quote.ID, run.seller.ID,
		[]string{
			"<none>->DRAFT by seller",
			"DRAFT->QUOTED by seller",
			"QUOTED->SENT by seller",
			fmt.Sprintf("SENT->%s by nobody", closing),
		})
}

// assertLedger reads one status-change table for one row and compares the whole sequence.
func assertLedger(t *testing.T, e *env, label, query string, id, sellerID uuid.UUID,
	want []string) {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(), query, id)
	if err != nil {
		t.Fatalf("read the %s ledger: %v", label, err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var previous, next string
		var userID *uuid.UUID
		if err := rows.Scan(&previous, &next, &userID); err != nil {
			t.Fatalf("scan the %s ledger: %v", label, err)
		}
		who := "nobody"
		switch {
		case userID == nil:
			// A customer is not one of ours, so their answer is recorded against no user at all.
			// Naming that here is what makes crediting it to the seller a failure.
		case *userID == sellerID:
			who = "seller"
		default:
			who = "someone else"
		}
		got = append(got, fmt.Sprintf("%s->%s by %s", previous, next, who))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the %s ledger: %v", label, err)
	}

	if len(got) != len(want) {
		t.Fatalf("%s ledger = %v, want the %d transitions %v", label, got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s transition %d = %q, want %q", label, i, got[i], want[i])
		}
	}
}
