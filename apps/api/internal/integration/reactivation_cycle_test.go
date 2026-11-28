//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

/*
 * TC-11: an accepted quote reactivated and resent unchanged starts a new answer cycle. The new link
 * takes a fresh answer, the seller's history shows the old one as reopened, and reports count only
 * the acceptance that stands.
 */
func TestReactivationCycle_AReopenedAnswerNoLongerStands(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "Reopened answer")
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: run.seller.AccountID, BranchID: run.draft.Quote.BranchID,
		UserID: run.seller.ID, Role: domain.UserRoleSeller}
	if _, err := run.service.RecordClientAction(ctx, run.publicToken, domain.ClientActionAccept); err != nil {
		t.Fatal(err)
	}
	if got := e.acceptedInReport(t, tenant); got != 1 {
		t.Fatalf("accepted quotes before reopening = %d, want 1", got)
	}

	reactivateThroughAPI(t, e, run, domain.QuoteReactivationResend)
	next, err := run.service.Send(ctx, tenant, run.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550191"})
	if err != nil {
		t.Fatal(err)
	}
	token := next.Deliveries[0].PublicURL
	token = token[strings.LastIndex(token, "/")+1:]
	if view, err := run.service.ResolvePublic(ctx, token); err != nil || view.CustomerStatus != nil {
		t.Fatalf("new link = %+v, %v, want it unanswered", view, err)
	}
	var history []domain.ClientAction
	if err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		history, err = repository.NewClientActionRepository().ListByQuote(ctx, q, tenant.AccountID,
			tenant.BranchID, run.draft.Quote.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ReopenedAt == nil {
		t.Fatalf("seller history = %+v, want the old acceptance shown as reopened", history)
	}
	if got := e.acceptedInReport(t, tenant); got != 0 {
		t.Fatalf("accepted quotes after reopening = %d, want 0", got)
	}

	if _, err := run.service.RespondPublic(ctx, token,
		domain.ClientActionInput{Type: domain.ClientActionAccept}); err != nil {
		t.Fatal(err)
	}
	if got := e.acceptedInReport(t, tenant); got != 1 {
		t.Errorf("accepted quotes after the new answer = %d, want 1", got)
	}
}

// acceptedInReport reads today's accepted-quote count the way the seller's report computes it.
func (e *env) acceptedInReport(t *testing.T, tenant domain.Tenant) int {
	t.Helper()
	location, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location).Format("2006-01-02")
	service := services.NewSellerReportService(e.db, repository.NewSellerReportRepository(), nil,
		services.SellerReportSettings{MaxRangeDays: 366, HighConfidence: decimal.RequireFromString("0.8")})
	report, err := service.Get(context.Background(), domain.Tenant{AccountID: tenant.AccountID,
		BranchID: tenant.BranchID, Role: domain.UserRoleAdmin}, today, today, domain.SellerReportFilterInput{})
	if err != nil {
		t.Fatal(err)
	}
	return report.KPIs.QuotesAccepted
}

// Reopening for edits ends the old answer too: it reads as reopened and no longer counts.
func TestReactivationCycle_ReopeningForEditsEndsTheAnswer(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "Reopened for edits")
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: run.seller.AccountID, BranchID: run.draft.Quote.BranchID,
		UserID: run.seller.ID, Role: domain.UserRoleSeller}
	if _, err := run.service.RecordClientAction(ctx, run.publicToken, domain.ClientActionAccept); err != nil {
		t.Fatal(err)
	}

	reactivateThroughAPI(t, e, run, domain.QuoteReactivationEdit)
	var history []domain.ClientAction
	if err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		history, err = repository.NewClientActionRepository().ListByQuote(ctx, q, tenant.AccountID,
			tenant.BranchID, run.draft.Quote.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ReopenedAt == nil {
		t.Fatalf("seller history = %+v, want the acceptance shown as reopened", history)
	}
	if got := e.acceptedInReport(t, tenant); got != 0 {
		t.Errorf("accepted quotes after reopening for edits = %d, want 0", got)
	}
}

// A reply acceptance from before a reactivation is not the one standing: the seller's own later
// acceptance cannot be undone through it.
func TestReactivationCycle_UndoIgnoresAReopenedReplyAcceptance(t *testing.T) {
	e := newEnv(t)
	quotes := e.acceptanceQuoteService()
	ctx := context.Background()
	seed := e.seedSendableQuote(t, "Reopened reply acceptance", false)
	delivery := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	e.sentQuoteToken(t, delivery, seed)
	e.dropDiscounts(t, seed.tenant.AccountID)
	quote := handlerQuote{seed: seed, quoteID: seed.draft.Quote.ID}
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "dale, lo llevo")
	if err := e.acceptingHandler(acceptedReply(95), &capturedOutcomes{}).Handle(ctx, batch); err != nil {
		t.Fatal(err)
	}
	e.finishBatch(t, batch.ID)
	if _, err := quotes.Reactivate(ctx, quote.seed.tenant, quote.quoteID,
		domain.QuoteReactivationResend); err != nil {
		t.Fatalf("Reactivate() = %v", err)
	}
	e.sentQuoteToken(t, delivery, quote.seed)
	if _, err := quotes.Transition(ctx, quote.seed.tenant, quote.quoteID,
		domain.QuoteStatusAccepted); err != nil {
		t.Fatalf("seller acceptance = %v", err)
	}

	if _, err := quotes.UndoAcceptance(ctx, quote.seed.tenant, quote.quoteID); domain.CodeOf(err) !=
		domain.CodeQuoteAcceptanceNotUndoable {
		t.Errorf("undo = %v, want QUOTE_ACCEPTANCE_NOT_UNDOABLE", err)
	}
	if state := e.quoteState(t, quote.quoteID); state.status != domain.QuoteStatusAccepted {
		t.Errorf("quote = %s, want the seller's acceptance to stand", state.status)
	}
	if got := e.acceptanceOf(t, quote.quoteID); got.revoked {
		t.Errorf("acceptance = %+v, want the old reply acceptance left as it was", got)
	}
}

// TC-11 as QA ran it: rejected twice, reactivated and resent unchanged each time, then accepted.
// Every new link takes an answer, and the quote ends accepted.
func TestReactivationCycle_RejectedTwiceThenAccepted(t *testing.T) {
	e := newEnv(t)
	run := firstOperationalFlow(t, e, "Rejected twice")
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: run.seller.AccountID, BranchID: run.draft.Quote.BranchID,
		UserID: run.seller.ID, Role: domain.UserRoleSeller}
	token := run.publicToken
	for round, answer := range []domain.ClientActionType{domain.ClientActionReject,
		domain.ClientActionReject, domain.ClientActionAccept} {
		if round > 0 {
			reactivateThroughAPI(t, e, run, domain.QuoteReactivationResend)
			next, err := run.service.Send(ctx, tenant, run.draft.Quote.ID,
				domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550191"})
			if err != nil {
				t.Fatal(err)
			}
			token = next.Deliveries[0].PublicURL[strings.LastIndex(next.Deliveries[0].PublicURL, "/")+1:]
			if view, err := run.service.ResolvePublic(ctx, token); err != nil || view.CustomerStatus != nil {
				t.Fatalf("round %d: new link = %+v, %v, want it unanswered", round, view, err)
			}
		}
		got, err := run.service.RespondPublic(ctx, token, domain.ClientActionInput{Type: answer})
		if err != nil || got.CustomerStatus != answer {
			t.Fatalf("round %d: answer = %+v, %v, want %s recorded", round, got, err, answer)
		}
	}
	if state := e.quoteState(t, run.draft.Quote.ID); state.status != domain.QuoteStatusAccepted {
		t.Errorf("quote = %s, want ACCEPTED", state.status)
	}
	if got := e.acceptedInReport(t, tenant); got != 1 {
		t.Errorf("accepted quotes = %d, want 1", got)
	}
}
