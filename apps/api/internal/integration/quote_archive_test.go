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

// seedIdleQuote writes a quote in status whose row last changed idleFor ago, with the offer
// validity it was sent with. The insert sets updated_at itself: only an UPDATE goes through the
// trigger that would reset it.
func (e *env) seedIdleQuote(
	t *testing.T, accountID, branchID uuid.UUID, status domain.QuoteStatus,
	idleFor time.Duration, archivedAt, expiresAt *time.Time,
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
		                    archived_at, expires_at, created_at, updated_at)
		 VALUES ($1::uuid, $2,
		         (('x'||substr(replace($1::uuid::text,'-',''),1,15))::bit(60)::bigint),
		         $3, $4, $5, $6, $8, now() - make_interval(secs => $7),
		         now() - make_interval(secs => $7))`,
		quoteID, accountID, branchID, rfqID, status, archivedAt,
		idleFor.Seconds(), expiresAt); err != nil {
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

// The sweep archives the follow-up stalls quiet past the threshold in every account — a sent one only
// once its offer has lapsed — leaves every other quote alone, changes nothing on a second run, and
// lets an unarchived quote start over.
func TestQuoteAutoArchive_ArchivesOnlyTheStallsQuietPastTheThreshold(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Auto archive")
	otherAccountID, otherBranchID := e.seedAccount(t, "Auto archive other")
	idle, recent := 40*24*time.Hour, 10*24*time.Hour
	boxedBefore := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	// The reviewer's case: sent 40 days ago on a 60-day validity, so the link is still live.
	lapsed, live := time.Now().Add(-24*time.Hour), time.Now().Add(20*24*time.Hour)

	eligible := map[string]uuid.UUID{
		"idle DRAFT":                            e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, idle, nil, nil),
		"idle QUOTED":                           e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusQuoted, idle, nil, nil),
		"idle SENT, lapsed, in another account": e.seedIdleQuote(t, otherAccountID, otherBranchID, domain.QuoteStatusSent, idle, nil, &lapsed),
	}
	left := map[string]uuid.UUID{
		"idle CHANGE_REQUESTED": e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusChangeRequested, idle, nil, nil),
		"idle ACCEPTED":         e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusAccepted, idle, nil, nil),
		"idle REJECTED":         e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusRejected, idle, nil, nil),
		"recent DRAFT":          e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, recent, nil, nil),
		"recent SENT":           e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, recent, nil, &lapsed),
		"idle SENT, link live":  e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, idle, nil, &live),
		"idle SENT, no expiry":  e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusSent, idle, nil, nil),
	}
	alreadyBoxed := e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft, idle,
		&boxedBefore, nil)

	// The closed window is longer than the idle REJECTED quote has waited, so it is left for now.
	job := services.NewQuoteAutoArchiveJob(repository.NewQuoteRepository(),
		config.QuoteArchiveConfig{InactiveAfter: 30 * 24 * time.Hour, ClosedAfter: 60 * 24 * time.Hour,
			BatchSize: 2},
		domain.ARCAEnvironmentHomologation, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

// backdateQuote makes a quote look idle for idleFor. The update trigger would stamp now() over any
// value written, so the write runs with triggers off for its own transaction only.
func (e *env) backdateQuote(t *testing.T, quoteID uuid.UUID, idleFor time.Duration) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("switch triggers off: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE quote SET updated_at = now() - make_interval(secs => $2) WHERE id = $1`,
		quoteID, idleFor.Seconds()); err != nil {
		t.Fatalf("backdate the quote: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func (e *env) autoArchiveJobs(t *testing.T) (*services.JobService, string) {
	t.Helper()
	job := services.NewQuoteAutoArchiveJob(repository.NewQuoteRepository(),
		config.QuoteArchiveConfig{InactiveAfter: 30 * 24 * time.Hour, ClosedAfter: 30 * 24 * time.Hour,
			BatchSize: 100},
		domain.ARCAEnvironmentHomologation, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.dropRuns(t, job.Name())
	return e.jobService(t, job), job.Name()
}

// closedSale accepts a sale and records its closing facts straight on the row, then makes it look
// idle for idleFor.
func (e *env) closedSale(
	t *testing.T, quoteID uuid.UUID, by domain.AppUser, paid, waived bool, idleFor time.Duration,
) uuid.UUID {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET paid_at = CASE WHEN $2 THEN now() END,
		                  paid_by_user_id = CASE WHEN $2 THEN $4::uuid END,
		                  invoice_waived_at = CASE WHEN $3 THEN now() END,
		                  invoice_waived_by_user_id = CASE WHEN $3 THEN $4::uuid END
		 WHERE id = $1`, quoteID, paid, waived, by.ID); err != nil {
		t.Fatalf("close the sale: %v", err)
	}
	e.backdateQuote(t, quoteID, idleFor)
	return quoteID
}

// seedCreditNote records a credit note in status against the sale's invoice, straight on the owner
// pool and dated with it.
func (e *env) seedCreditNote(t *testing.T, sale invoicedSale, status domain.InvoiceStatus) {
	t.Helper()
	noteID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO invoice (id, account_id, branch_id, quote_id, quote_version_id, environment, kind, status,
		                      invoice_type, point_of_sale, number, issued_on, cae, cae_expires_on, issuer_cuit,
		                      receiver_name, receiver_doc_type, receiver_iva_condition, net_amount, exempt_amount,
		                      vat_amount, total, currency, credited_invoice_id, created_at)
		 SELECT $1, account_id, branch_id, quote_id, quote_version_id, environment, 'CREDIT_NOTE', $2,
		        invoice_type, point_of_sale, CASE WHEN $2::invoice_status = 'ISSUED' THEN 24 END, issued_on,
		        CASE WHEN $2::invoice_status = 'ISSUED' THEN '74123456789013' END,
		        CASE WHEN $2::invoice_status = 'ISSUED' THEN issued_on + 10 END, issuer_cuit, receiver_name,
		        receiver_doc_type, receiver_iva_condition, net_amount, exempt_amount, vat_amount, total,
		        currency, id, created_at
		 FROM invoice WHERE id = $3`, noteID, status, sale.invoiceID); err != nil {
		t.Fatalf("seed the credit note: %v", err)
	}
	// Registered after the invoice's own teardown, so it runs first and frees the invoice.
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM invoice WHERE id = $1`, noteID) })
}

// Complete sales and rejected quotes are archived once quiet past the closed window; a sale still
// waiting to be paid or invoiced, or with an invoice in flight, never is.
func TestQuoteAutoArchive_ArchivesClosedOutcomesPastTheirOwnWindow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	invoicingID, invoicingBranch := e.seedAccount(t, "Archivo cerradas")
	cashID, cashBranch := e.seedAccount(t, "Archivo cerradas sin factura")
	e.setInvoicing(t, invoicingID, true)
	e.setInvoicing(t, cashID, false)
	seller := e.seedUser(t, invoicingID, domain.UserRoleAdmin)
	cashier := e.seedUser(t, cashID, domain.UserRoleAdmin)
	// Past the 30-day stall window, inside the 60-day closed one; and past both.
	quiet, closedQuiet := 40*24*time.Hour, 70*24*time.Hour

	accepted := func(accountID, branchID uuid.UUID, by domain.AppUser) uuid.UUID {
		return e.seedAcceptedSale(t, accountID, branchID, by).quoteID
	}
	// An invoiced sale, paid, quiet as long as its invoice is old, each in an account of its own: an
	// invoice number is unique per account and point of sale.
	invoicedSale := func(name string, env string) (invoicedSale, domain.AppUser) {
		accountID, branchID := e.seedAccount(t, name)
		e.setInvoicing(t, accountID, true)
		user := e.seedUser(t, accountID, domain.UserRoleAdmin)
		sale := e.seedInvoicedSale(t, accountID, branchID, user, domain.InvoiceStatusIssued)
		// An authorized invoice is immutable, so the fixture is aged with triggers off.
		tx, err := e.db.AdminTx(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
			t.Fatalf("switch triggers off: %v", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE invoice SET environment = $2, issued_on = current_date - 70, created_at = now() - interval '70 days'
			 WHERE id = $1`, sale.invoiceID, env); err != nil {
			t.Fatalf("age the invoice: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		e.closedSale(t, sale.quoteID, user, true, false, closedQuiet)
		return sale, user
	}
	invoiced, _ := invoicedSale("Archivo cerradas facturada", "HOMOLOGATION")
	production, _ := invoicedSale("Archivo cerradas producción", "PRODUCTION")
	pending := e.seedInvoicedSale(t, invoicingID, invoicingBranch, seller, domain.InvoiceStatusPending)
	// Paid, but its invoice is cancelled by a credit note: still waiting for ARCA, or issued and
	// never invoiced again.
	crediting, _ := invoicedSale("Archivo cerradas nota pendiente", "HOMOLOGATION")
	e.seedCreditNote(t, crediting, domain.InvoiceStatusPending)
	credited, _ := invoicedSale("Archivo cerradas nota emitida", "HOMOLOGATION")
	e.seedCreditNote(t, credited, domain.InvoiceStatusIssued)

	eligible := map[string]uuid.UUID{
		"paid, invoice waived": e.closedSale(t, accepted(invoicingID, invoicingBranch, seller), seller, true, true, closedQuiet),
		"paid, invoiced":       invoiced.quoteID,
		"paid, invoiced in production on a test server": production.quoteID,
		"paid, invoicing off":                           e.closedSale(t, accepted(cashID, cashBranch, cashier), cashier, true, false, closedQuiet),
		"REJECTED":                                      e.seedIdleQuote(t, invoicingID, invoicingBranch, domain.QuoteStatusRejected, closedQuiet, nil, nil),
	}
	left := map[string]uuid.UUID{
		"REJECTED inside the closed window":                              e.seedIdleQuote(t, invoicingID, invoicingBranch, domain.QuoteStatusRejected, quiet, nil, nil),
		"complete inside the closed window":                              e.closedSale(t, accepted(invoicingID, invoicingBranch, seller), seller, true, true, quiet),
		"unpaid, invoice waived":                                         e.closedSale(t, accepted(invoicingID, invoicingBranch, seller), seller, false, true, closedQuiet),
		"unpaid, invoicing off":                                          e.closedSale(t, accepted(cashID, cashBranch, cashier), cashier, false, false, closedQuiet),
		"paid, waiting for its invoice":                                  e.closedSale(t, accepted(invoicingID, invoicingBranch, seller), seller, true, false, closedQuiet),
		"paid, invoice pending in ARCA":                                  e.closedSale(t, pending.quoteID, seller, true, false, closedQuiet),
		"paid, credit note pending in ARCA":                              crediting.quoteID,
		"paid, invoice cancelled by a credit note, never invoiced again": credited.quoteID,
		"CHANGE_REQUESTED":                                               e.seedIdleQuote(t, invoicingID, invoicingBranch, domain.QuoteStatusChangeRequested, closedQuiet, nil, nil),
	}

	archiveJob := func(env domain.ARCAEnvironment) (*services.JobService, string) {
		job := services.NewQuoteAutoArchiveJob(repository.NewQuoteRepository(),
			config.QuoteArchiveConfig{InactiveAfter: 30 * 24 * time.Hour, ClosedAfter: 60 * 24 * time.Hour,
				BatchSize: 100},
			env, slog.New(slog.NewTextHandler(io.Discard, nil)))
		e.dropRuns(t, job.Name())
		return e.jobService(t, job), job.Name()
	}

	// A server on production counts no test invoice, so that sale still waits for one.
	jobs, name := archiveJob(domain.ARCAEnvironmentProduction)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("Run() on production = %v, want no error", err)
	}
	if at := e.archivedAtOf(t, invoiced.quoteID); at != nil {
		t.Errorf("test invoice on a production server: archived at %v, want it left alone", at)
	}

	jobs, name = archiveJob(domain.ARCAEnvironmentHomologation)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}
	for name, id := range eligible {
		if e.archivedAtOf(t, id) == nil {
			t.Errorf("%s: not archived, want it archived", name)
		}
	}
	for name, id := range left {
		if at := e.archivedAtOf(t, id); at != nil {
			t.Errorf("%s: archived at %v, want it left alone", name, at)
		}
	}
}

// Editing a draft's lines is activity: a quote a seller is working on is never swept as idle.
func TestQuoteAutoArchive_ALineEditKeepsADraftActive(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Auto archive line edit")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	lines := []draftLine{{description: "10 bolsas de cemento", quantity: "10"}}
	edited := e.seedDraftQuote(t, accountID, branchID, admin, lines)
	untouched := e.seedDraftQuote(t, accountID, branchID, admin, lines)
	e.backdateQuote(t, edited.quoteID, 40*24*time.Hour)
	e.backdateQuote(t, untouched.quoteID, 40*24*time.Hour)

	rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + edited.quoteID.String() + "/items",
		token: e.tokenFor(t, admin), branch: branchID.String(),
		body: map[string]any{"requested_description": "arena", "quantity": "2"}})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add item = %d, want it accepted: %s", rec.Code, rec.Body)
	}
	jobs, name := e.autoArchiveJobs(t)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatalf("Run() = %v, want no error", err)
	}

	if at := e.archivedAtOf(t, edited.quoteID); at != nil {
		t.Errorf("edited draft archived at %v, want the edit to count as activity", at)
	}
	if e.archivedAtOf(t, untouched.quoteID) == nil {
		t.Error("untouched idle draft not archived, want it archived")
	}
}

// An edit holds the quote's row from its guard to its write, so the sweep skips it rather than
// archiving it in between, and the edit's own commit leaves it active.
func TestQuoteAutoArchive_SkipsAQuoteAnEditHolds(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Auto archive held")
	quoteID := e.seedIdleQuote(t, accountID, branchID, domain.QuoteStatusDraft,
		40*24*time.Hour, nil, nil)
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}

	claimed, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			if _, err := repository.NewQuoteRepository().ClaimForEdit(ctx, q, accountID, branchID,
				quoteID); err != nil {
				return err
			}
			close(claimed)
			<-release
			return nil
		})
	}()
	<-claimed
	jobs, name := e.autoArchiveJobs(t)
	_, runErr := jobs.Run(ctx, name)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("edit transaction = %v, want no error", err)
	}
	if runErr != nil {
		t.Fatalf("Run() = %v, want no error", runErr)
	}
	if at := e.archivedAtOf(t, quoteID); at != nil {
		t.Errorf("held quote archived at %v, want it skipped", at)
	}
}

// A seller may still archive a sent quote whose link is live. The public side then stops offering
// it: the link reads as unavailable, and an answer is refused by name.
func TestQuoteArchive_APublicLinkStopsOfferingAnArchivedQuote(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Archive public link", false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, _ := e.sentQuoteToken(t, service, seed)
	e.archiveQuote(t, seed.draft.Quote.ID, time.Now())

	resolved, err := service.ResolvePublic(context.Background(), token)
	if err != nil {
		t.Fatalf("ResolvePublic() = %v, want no error", err)
	}
	if resolved.Status != domain.PublicQuoteStatusUnavailable || resolved.Payload != nil {
		t.Errorf("resolved = %+v, want UNAVAILABLE with no quote", resolved)
	}
	_, err = service.RespondPublic(context.Background(), token,
		domain.ClientActionInput{Type: domain.ClientActionAccept})
	if domain.CodeOf(err) != domain.CodeQuoteArchived {
		t.Fatalf("RespondPublic() = %v, want QUOTE_ARCHIVED", err)
	}
}
