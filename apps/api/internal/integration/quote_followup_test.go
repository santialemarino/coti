//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type followupState struct {
	flagged   bool
	flaggedAt *time.Time
	updatedAt time.Time
}

func (e *env) followupOf(t *testing.T, quoteID uuid.UUID) followupState {
	t.Helper()
	var state followupState
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT needs_followup, followup_flagged_at, updated_at FROM quote WHERE id = $1`,
		quoteID).Scan(&state.flagged, &state.flaggedAt, &state.updatedAt); err != nil {
		t.Fatalf("read the follow-up flag: %v", err)
	}
	return state
}

func (e *env) followupJobs(t *testing.T) (*services.JobService, string) {
	t.Helper()
	job := services.NewQuoteFollowupJob(repository.NewQuoteRepository(),
		config.QuoteFollowupConfig{SellerIdleAfter: 2 * 24 * time.Hour,
			CustomerSilentAfter: 5 * 24 * time.Hour, BatchSize: 2},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.dropRuns(t, job.Name())
	return e.jobService(t, job), job.Name()
}

// The sweep flags the seller's stalls and the client's silences past their own thresholds in every
// account, leaves every other quote alone, keeps each quote's inactivity where it was, and changes
// nothing on a second run.
func TestQuoteFollowup_FlagsOnlyTheStallsQuietPastTheirThreshold(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up")
	otherAccountID, otherBranchID := e.seedAccount(t, "Follow-up other")
	threeDays, sixDays := 3*24*time.Hour, 6*24*time.Hour
	lapsed := time.Now().Add(-24 * time.Hour)
	boxed := time.Now().Add(-time.Hour)

	eligible := map[string]uuid.UUID{
		"DRAFT idle past the seller's threshold":  e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, threeDays, nil, nil),
		"QUOTED idle in another account":          e.seedIdleQuote(t, otherAccountID, otherBranchID, domain.QuoteStatusQuoted, threeDays, nil, nil),
		"SENT silent past the client's threshold": e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, sixDays, nil, nil),
		"SENT whose offer lapsed unanswered":      e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, sixDays, nil, &lapsed),
	}
	left := map[string]uuid.UUID{
		// Past the seller's threshold but short of the client's: each origin keeps its own.
		"SENT silent three days": e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, threeDays, nil, nil),
		"DRAFT idle a day":       e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, 24*time.Hour, nil, nil),
		"idle CHANGE_REQUESTED":  e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusChangeRequested, sixDays, nil, nil),
		"idle ACCEPTED":          e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusAccepted, sixDays, nil, nil),
		"idle REJECTED":          e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusRejected, sixDays, nil, nil),
		"archived idle DRAFT":    e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, sixDays, &boxed, nil),
		"archived silent SENT":   e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, sixDays, &boxed, nil),
	}
	before := map[uuid.UUID]time.Time{}
	for _, id := range eligible {
		before[id] = e.followupOf(t, id).updatedAt
	}

	jobs, name := e.followupJobs(t)
	run, err := jobs.Run(ctx, name)
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	// A batch of two proves the run drains past its first batch.
	if run.Changed < len(eligible) {
		t.Errorf("changed = %d, want at least %d", run.Changed, len(eligible))
	}
	firstFlag := map[uuid.UUID]time.Time{}
	for label, id := range eligible {
		state := e.followupOf(t, id)
		if !state.flagged || state.flaggedAt == nil {
			t.Errorf("%s: not flagged, want it flagged", label)
			continue
		}
		firstFlag[id] = *state.flaggedAt
		// Flagging is not activity: the archive sweep still measures from the quote's last change.
		if !state.updatedAt.Equal(before[id]) {
			t.Errorf("%s: updated_at moved from %v to %v, want the flag to leave it", label,
				before[id], state.updatedAt)
		}
	}
	for label, id := range left {
		if state := e.followupOf(t, id); state.flagged {
			t.Errorf("%s: flagged, want it left alone", label)
		}
	}

	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("second Run() = %v, want no error", err)
	}
	for label, id := range eligible {
		if state := e.followupOf(t, id); state.flaggedAt == nil || !state.flaggedAt.Equal(firstFlag[id]) {
			t.Errorf("%s: flagged_at = %v after a second run, want %v", label, state.flaggedAt,
				firstFlag[id])
		}
	}
}

// Any write to a flagged quote is activity: it clears the flag and restarts the clock, so the next
// run leaves the quote alone until it goes quiet again.
func TestQuoteFollowup_ActivityClearsTheFlag(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up activity")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	lines := []draftLine{{description: "10 bolsas de cemento", quantity: "10"}}
	edited := e.seedDraftQuote(t, accountID, branchID, admin, lines)
	untouched := e.seedDraftQuote(t, accountID, branchID, admin, lines)
	e.backdateQuote(t, edited.quoteID, 3*24*time.Hour)
	e.backdateQuote(t, untouched.quoteID, 3*24*time.Hour)

	jobs, name := e.followupJobs(t)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	if !e.followupOf(t, edited.quoteID).flagged || !e.followupOf(t, untouched.quoteID).flagged {
		t.Fatal("idle drafts not flagged, want both flagged before the edit")
	}

	editStarted := time.Now().Add(-time.Minute)
	rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + edited.quoteID.String() + "/items",
		token: e.tokenFor(t, admin), branch: branchID.String(),
		body: map[string]any{"requested_description": "arena", "quantity": "2"}})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add item = %d, want it accepted: %s", rec.Code, rec.Body)
	}
	state := e.followupOf(t, edited.quoteID)
	if state.flagged || state.flaggedAt != nil {
		t.Errorf("edited quote: flagged = %v at %v, want the edit to clear it", state.flagged,
			state.flaggedAt)
	}
	if state.updatedAt.Before(editStarted) {
		t.Errorf("edited quote: updated_at = %v, want it moved by the edit", state.updatedAt)
	}

	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("second Run() = %v, want no error", err)
	}
	if e.followupOf(t, edited.quoteID).flagged {
		t.Error("edited quote flagged again, want its inactivity to start over")
	}
	if !e.followupOf(t, untouched.quoteID).flagged {
		t.Error("untouched quote lost its flag, want it kept")
	}
}

// A flagged quote nobody chases is still archived on schedule: the flag never pushes the archive
// sweep's measure forward.
func TestQuoteFollowup_AFlaggedQuoteIsStillArchived(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Follow-up then archive")
	quoteID := e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusQuoted,
		40*24*time.Hour, nil, nil)

	followups, followup := e.followupJobs(t)
	if _, err := followups.Run(ctx, followup); err != nil {
		t.Fatalf("follow-up Run() = %v, want no error", err)
	}
	if !e.followupOf(t, quoteID).flagged {
		t.Fatal("idle quote not flagged, want it flagged")
	}
	archives, archive := e.autoArchiveJobs(t)
	if _, err := archives.Run(ctx, archive); err != nil {
		t.Fatalf("archive Run() = %v, want no error", err)
	}
	if e.archivedAtOf(t, quoteID) == nil {
		t.Error("flagged idle quote not archived, want the flag to leave its inactivity alone")
	}
}

// A write that raises the flag together with any other change is activity, so it does not keep the
// quote's inactivity and the flag does not survive it.
func TestQuoteFollowup_AFlagWrittenWithAnotherChangeIsActivity(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Follow-up mixed write")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	quoteID := e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusQuoted, 6*24*time.Hour,
		nil, nil)
	before := e.followupOf(t, quoteID)

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET needs_followup = TRUE, followup_flagged_at = now(), seller_id = $2
		 WHERE id = $1`, quoteID, seller.ID); err != nil {
		t.Fatalf("write the flag and the seller: %v", err)
	}
	after := e.followupOf(t, quoteID)
	if after.flagged || after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("flagged = %v, updated_at %v -> %v, want the assignment to count as activity",
			after.flagged, before.updatedAt, after.updatedAt)
	}
}
