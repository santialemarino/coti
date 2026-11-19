//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func (e *env) followupService() *services.QuoteFollowupService {
	return services.NewQuoteFollowupService(e.db, repository.NewQuoteRepository(),
		repository.NewAccountRepository(), repository.NewClientRepository())
}

func (e *env) setQuoteStatus(t *testing.T, quoteID uuid.UUID, status domain.QuoteStatus) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET current_status = $2 WHERE id = $1`, quoteID, status); err != nil {
		t.Fatalf("set the quote status: %v", err)
	}
}

// Rendering an account reminder does not change the quote's follow-up flag or inactivity.
func TestQuoteFollowupDraft_DraftsASentQuoteAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up draft")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "10 bolsas de cemento", quantity: "10"}})
	var clientID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx,
		`INSERT INTO client(account_id, name) VALUES ($1, 'Juan Pérez') RETURNING id`,
		accountID).Scan(&clientID); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote SET current_status = 'SENT', client_id = $2, expires_at = now() - interval '1 day'
		 WHERE id = $1`, seeded.quoteID, clientID); err != nil {
		t.Fatalf("send the quote: %v", err)
	}
	e.backdateQuote(t, seeded.quoteID, 6*24*time.Hour)
	jobs, name := e.followupJobs(t)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("follow-up Run() = %v, want no error", err)
	}
	before := e.followupOf(t, seeded.quoteID)
	if !before.flagged {
		t.Fatal("the sent quote was not flagged, want a flagged quote to draft for")
	}

	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	message, err := e.followupService().Draft(ctx, tenant, seeded.quoteID)
	if err != nil {
		t.Fatalf("Draft() = %v, want no error", err)
	}
	if message == "" {
		t.Fatal("the deterministic template is empty")
	}
	if after := e.followupOf(t, seeded.quoteID); !after.flagged || !after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("after the draft: flagged = %v, updated_at = %v, want both untouched", after.flagged,
			after.updatedAt)
	}
}

// Only a sent quote, live in the caller's branch, has a client to chase; no model is needed.
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
	service := e.followupService()

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
	if _, err := e.followupService().Draft(ctx, tenant, draft.quoteID); err != nil {
		t.Errorf("template without a model = %v, want no error", err)
	}
}
