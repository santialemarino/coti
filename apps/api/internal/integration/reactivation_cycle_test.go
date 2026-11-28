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
