//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/pdf"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

// The backend half of "every edit on an archived quote is blocked": each edit route answers 409
// QUOTE_ARCHIVED, the quote keeps its lines, and the same edit goes through once it is unarchived.
// This router wires no discount store, so the discount routes are pinned by the service tests.
func TestQuoteArchive_EveryEditRouteRefusesAnArchivedQuote(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Archive edit matrix")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token, branch := e.tokenFor(t, admin), branchID.String()
	seeded := e.seedDraftQuote(t, accountID, branchID, admin,
		[]draftLine{{description: "10 bolsas de cemento", quantity: "10"}})
	quotePath := "/v1/quotes/" + seeded.quoteID.String()

	if rec := e.do(t, request{method: http.MethodPost, path: quotePath + "/archive",
		token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d, want 200: %s", rec.Code, rec.Body)
	}

	someID := uuid.NewString()
	edits := []request{
		{method: http.MethodPost, path: quotePath + "/items",
			body: map[string]any{"requested_description": "arena", "quantity": "2"}},
		{method: http.MethodPatch, path: quotePath + "/items/" + someID,
			body: map[string]any{"quantity": "3"}},
		{method: http.MethodDelete, path: quotePath + "/items/" + someID},
		{method: http.MethodPost, path: quotePath + "/accept-materials"},
		{method: http.MethodPatch,
			path: quotePath + "/items/" + someID + "/alternatives/" + uuid.NewString(),
			body: map[string]any{"approved_by_seller": true}},
		{method: http.MethodPost, path: quotePath + "/transition",
			body: map[string]any{"status": "SENT"}},
		{method: http.MethodPost, path: quotePath + "/reactivate",
			body: map[string]any{"mode": "EDIT"}},
		{method: http.MethodPut, path: quotePath + "/client-association",
			body: map[string]any{"client_id": someID}},
	}
	for _, edit := range edits {
		edit.token, edit.branch = token, branch
		t.Run(edit.method+" "+edit.path[len(quotePath):], func(t *testing.T) {
			rec := e.do(t, edit)
			if rec.Code != http.StatusConflict || errorCode(t, rec) != "QUOTE_ARCHIVED" {
				t.Fatalf("status = %d, body = %s, want 409 QUOTE_ARCHIVED", rec.Code, rec.Body)
			}
		})
	}
	if lines := e.storedLines(t, seeded.versionID); len(lines) != 1 {
		t.Fatalf("lines = %d, want the one seeded: an archived quote was edited", len(lines))
	}

	if rec := e.do(t, request{method: http.MethodPost, path: quotePath + "/unarchive",
		token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("unarchive status = %d, want 200: %s", rec.Code, rec.Body)
	}
	rec := e.do(t, request{method: http.MethodPost, path: quotePath + "/items", token: token,
		branch: branch, body: map[string]any{"requested_description": "arena", "quantity": "2"}})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add item after unarchive = %d, want it accepted: %s", rec.Code, rec.Body)
	}
}

// Sending and rendering the PDF are edits too, refused by name rather than as "not sendable".
func TestQuoteArchive_SendAndRepresentationRefuseAnArchivedQuote(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Archive send", false)
	e.archiveQuote(t, seed.draft.Quote.ID, time.Now())
	whatsapp := &captureWhatsAppSender{}
	service := e.quoteDeliveryService(t, whatsapp, &stagedQuoteEmailSender{}, nil)

	_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550101"})
	if domain.CodeOf(err) != domain.CodeQuoteArchived {
		t.Fatalf("Send() = %v, want QUOTE_ARCHIVED", err)
	}
	if whatsapp.count() != 0 {
		t.Fatal("an archived quote reached the client")
	}

	objects, err := storage.NewLocalStorage(t.TempDir(), &url.URL{Scheme: "https", Host: "files.test"},
		storage.NewURLSigner([]byte(testJWTSecret), nil))
	if err != nil {
		t.Fatal(err)
	}
	representations := services.NewQuoteRepresentationService(e.db,
		repository.NewQuoteRepresentationRepository(), repository.NewQuoteRepository(), objects,
		nil, pdf.NewQuoteRenderer(), 15*time.Minute, nil, nil)
	if _, err := representations.Ensure(context.Background(), seed.tenant,
		seed.draft.Quote.ID); domain.CodeOf(err) != domain.CodeQuoteArchived {
		t.Fatalf("Ensure() = %v, want QUOTE_ARCHIVED", err)
	}
}

// archiveQuote sets the flag straight on the row, the way the sweep leaves it.
func (e *env) archiveQuote(t *testing.T, quoteID uuid.UUID, at time.Time) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET archived_at = $2 WHERE id = $1`, quoteID, at); err != nil {
		t.Fatalf("archive the quote: %v", err)
	}
}

// seedIdleQuote writes a quote in status whose row last changed idleFor ago. The insert sets
// updated_at itself: only an UPDATE goes through the trigger that would reset it.
func (e *env) seedIdleQuote(
	t *testing.T, accountID, branchID uuid.UUID, status domain.QuoteStatus,
	idleFor time.Duration, archivedAt *time.Time,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	rfqID, quoteID := uuid.New(), uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, raw_text, status)
		 VALUES ($1, $2, $3, $4, 'pedido quieto', 'GENERATED')`,
		rfqID, accountID, branchID, channelID); err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote (id, account_id, number, branch_id, rfq_id, current_status,
		                    archived_at, created_at, updated_at)
		 VALUES ($1::uuid, $2,
		         (('x'||substr(replace($1::uuid::text,'-',''),1,15))::bit(60)::bigint),
		         $3, $4, $5, $6, now() - make_interval(secs => $7),
		         now() - make_interval(secs => $7))`,
		quoteID, accountID, branchID, rfqID, status, archivedAt,
		idleFor.Seconds()); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	e.dropDraft(t, rfqID)
	return quoteID
}

func (e *env) archivedAtOf(t *testing.T, quoteID uuid.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT archived_at FROM quote WHERE id = $1`, quoteID).Scan(&at); err != nil {
		t.Fatalf("read archived_at: %v", err)
	}
	return at
}

// The sweep archives the follow-up stalls quiet past the threshold in every account, leaves every
// other quote alone, changes nothing on a second run, and lets an unarchived quote start over.
func TestQuoteAutoArchive_ArchivesOnlyTheStallsQuietPastTheThreshold(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Auto archive")
	otherAccountID, otherBranchID := e.seedAccount(t, "Auto archive other")
	idle, recent := 40*24*time.Hour, 10*24*time.Hour
	boxedBefore := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	eligible := map[string]uuid.UUID{
		"idle DRAFT":                   e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, idle, nil),
		"idle QUOTED":                  e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusQuoted, idle, nil),
		"idle SENT in another account": e.seedIdleQuote(t, otherAccountID, otherBranchID, domain.QuoteStatusSent, idle, nil),
	}
	left := map[string]uuid.UUID{
		"idle CHANGE_REQUESTED": e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusChangeRequested, idle, nil),
		"idle ACCEPTED":         e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusAccepted, idle, nil),
		"idle REJECTED":         e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusRejected, idle, nil),
		"recent DRAFT":          e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, recent, nil),
		"recent SENT":           e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, recent, nil),
	}
	alreadyBoxed := e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, idle,
		&boxedBefore)

	job := services.NewQuoteAutoArchiveJob(repository.NewQuoteRepository(),
		config.QuoteArchiveConfig{InactiveAfter: 30 * 24 * time.Hour, BatchSize: 2},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	jobs := e.jobService(t, job)
	e.dropRuns(t, job.Name())

	run, err := jobs.Run(ctx, job.Name())
	if err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	// Other suites' quotes are fresh, so these three are all a clean database holds; a batch of
	// two proves the run drains past its first batch.
	if run.Changed < len(eligible) {
		t.Errorf("changed = %d, want at least %d", run.Changed, len(eligible))
	}
	firstArchive := map[string]time.Time{}
	for name, id := range eligible {
		at := e.archivedAtOf(t, id)
		if at == nil {
			t.Errorf("%s: not archived, want it archived", name)
			continue
		}
		firstArchive[name] = *at
	}
	for name, id := range left {
		if at := e.archivedAtOf(t, id); at != nil {
			t.Errorf("%s: archived at %v, want it left alone", name, at)
		}
	}
	if at := e.archivedAtOf(t, alreadyBoxed); at == nil || !at.Equal(boxedBefore) {
		t.Errorf("already archived: archived_at = %v, want %v untouched", at, boxedBefore)
	}

	// A second firing finds nothing new.
	if _, err := jobs.Run(ctx, job.Name()); err != nil {
		t.Fatalf("second Run() = %v, want no error", err)
	}
	for name, id := range eligible {
		if at := e.archivedAtOf(t, id); at == nil || !at.Equal(firstArchive[name]) {
			t.Errorf("%s: archived_at = %v after a second run, want %v", name, at,
				firstArchive[name])
		}
	}

	// Unarchiving is activity: the next firing must not put the quote straight back.
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	if err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		_, unarchiveErr := repository.NewQuoteRepository().Unarchive(ctx, q, accountID, branchID,
			eligible["idle DRAFT"])
		return unarchiveErr
	}); err != nil {
		t.Fatalf("Unarchive() = %v, want no error", err)
	}
	if _, err := jobs.Run(ctx, job.Name()); err != nil {
		t.Fatalf("third Run() = %v, want no error", err)
	}
	if at := e.archivedAtOf(t, eligible["idle DRAFT"]); at != nil {
		t.Errorf("unarchived DRAFT archived again at %v, want its inactivity to start over", at)
	}
}
