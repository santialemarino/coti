//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// captureDrafter records what it was asked to write and answers with a fixed message.
type captureDrafter struct {
	calls []domain.FollowupDraftInput
}

func (d *captureDrafter) Draft(_ context.Context, in domain.FollowupDraftInput) (string, error) {
	d.calls = append(d.calls, in)
	return "Hola, ¿pudiste ver la cotización?", nil
}

func (e *env) followupService(drafter domain.FollowupDrafter) *services.QuoteFollowupService {
	return services.NewQuoteFollowupService(e.db, repository.NewQuoteRepository(),
		repository.NewAccountRepository(), repository.NewClientRepository(),
		repository.NewUserRepository(), drafter)
}

func (e *env) setQuoteStatus(t *testing.T, quoteID uuid.UUID, status domain.QuoteStatus) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET current_status = $2 WHERE id = $1`, quoteID, status); err != nil {
		t.Fatalf("set the quote status: %v", err)
	}
}

// A sent quote is drafted from its own lines, seller and account, and the draft writes nothing:
// the flag and the quote's inactivity stay as they were.
func TestQuoteFollowupDraft_DraftsASentQuoteAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up draft")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "10 bolsas de cemento", quantity: "10"}})
	e.setQuoteStatus(t, seeded.quoteID, domain.QuoteStatusSent)
	e.backdateQuote(t, seeded.quoteID, 6*24*time.Hour)
	jobs, name := e.followupJobs(t)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("follow-up Run() = %v, want no error", err)
	}
	before := e.followupOf(t, seeded.quoteID)
	if !before.flagged {
		t.Fatal("the sent quote was not flagged, want a flagged quote to draft for")
	}

	drafter := &captureDrafter{}
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	message, err := e.followupService(drafter).Draft(ctx, tenant, seeded.quoteID)
	if err != nil {
		t.Fatalf("Draft() = %v, want no error", err)
	}
	if message == "" || len(drafter.calls) != 1 {
		t.Fatalf("message = %q after %d calls, want one draft", message, len(drafter.calls))
	}
	in := drafter.calls[0]
	if in.SupplierName != "Follow-up draft" || in.SellerName != seller.Name {
		t.Errorf("supplier, seller = %q, %q, want the account and the quote's seller", in.SupplierName,
			in.SellerName)
	}
	if !slices.Equal(in.Items, []string{"10 bolsas de cemento"}) {
		t.Errorf("items = %q, want the quote's line", in.Items)
	}
	if !in.QuietSince.Equal(before.updatedAt) {
		t.Errorf("quiet since = %v, want the quote's last change %v", in.QuietSince, before.updatedAt)
	}
	if after := e.followupOf(t, seeded.quoteID); !after.flagged || !after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("after the draft: flagged = %v, updated_at = %v, want both untouched", after.flagged,
			after.updatedAt)
	}
}

// Only a sent quote, live in the caller's branch, has a client to chase; a missing model is named.
func TestQuoteFollowupDraft_RefusesWhatHasNoClientToChase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up draft refusals")
	otherBranchID := e.seedBranch(t, accountID, "Otra sucursal")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	lines := []draftLine{{description: "arena", quantity: "2"}}
	draft := e.seedDraftQuote(t, accountID, branchID, seller, lines)
	archived := e.seedDraftQuote(t, accountID, branchID, seller, lines)
	e.setQuoteStatus(t, archived.quoteID, domain.QuoteStatusSent)
	e.archiveQuote(t, archived.quoteID, time.Now())
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	drafter := &captureDrafter{}
	service := e.followupService(drafter)

	if _, err := service.Draft(ctx, tenant, draft.quoteID); domain.CodeOf(err) != domain.CodeQuoteNotSent {
		t.Errorf("draft on a DRAFT quote = %v, want QUOTE_NOT_SENT", err)
	}
	if _, err := service.Draft(ctx, tenant, archived.quoteID); domain.CodeOf(err) != domain.CodeQuoteArchived {
		t.Errorf("draft on an archived quote = %v, want QUOTE_ARCHIVED", err)
	}
	otherBranch := domain.Tenant{AccountID: accountID, BranchID: otherBranchID, Role: domain.UserRoleAdmin}
	e.setQuoteStatus(t, draft.quoteID, domain.QuoteStatusSent)
	if _, err := service.Draft(ctx, otherBranch, draft.quoteID); domain.CodeOf(err) != domain.CodeNotFound {
		t.Errorf("draft from another branch = %v, want NOT_FOUND", err)
	}
	if len(drafter.calls) != 0 {
		t.Errorf("drafter called %d times, want none for a refused quote", len(drafter.calls))
	}
	if _, err := e.followupService(nil).Draft(ctx, tenant, draft.quoteID); domain.CodeOf(err) != domain.CodeAIUnavailable {
		t.Errorf("draft with no model = %v, want AI_UNAVAILABLE", err)
	}
}
