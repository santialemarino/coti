//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// The pipeline composed the way the composition root composes it, minus the model: a staged
// extractor stands in for the provider, and everything after it — matching, the real hybrid search
// over a seeded catalog, and every write — is the production path against real SQL.

// stagedExtractor answers with lines the test wrote, so the pipeline can be driven without a
// provider while everything below it stays real.
type stagedExtractor struct {
	lines []domain.ExtractedRFQLine
	err   error
}

func (s stagedExtractor) Extract(
	_ context.Context, _ string,
) ([]domain.ExtractedRFQLine, error) {
	return s.lines, s.err
}

func rfqConfig() config.RFQConfig {
	return config.RFQConfig{MaxTextCharacters: 20000, MaxItems: 200, PipelineTimeout: time.Minute}
}

// pipeline wires the RFQ service over the real repositories, the real matching stack, and the
// extractor the test staged.
func (e *env) pipeline(
	t *testing.T, extractor domain.RFQExtractor, axes map[string]int,
) *services.RFQService {
	t.Helper()
	return services.NewRFQService(e.db, repository.NewRFQRepository(),
		repository.NewQuoteRepository(), repository.NewChannelRepository(), extractor,
		e.matcher(t, matchConfig(), axes),
		slog.New(slog.NewTextHandler(io.Discard, nil)), rfqConfig())
}

// seedIntakeChannel opens a channel to receive an order on, and takes it away with the test.
func (e *env) seedIntakeChannel(t *testing.T, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier, is_active)
		 VALUES ($1, $2, $3, 'WHATSAPP', $4, TRUE)`,
		id, accountID, branchID, uuid.NewString()); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM channel WHERE id = $1`, id) })
	return id
}

// dropDraft removes the whole chain an order produced, children before parents.
func (e *env) dropDraft(t *testing.T, rfqID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM quote_item WHERE version_id IN (
		  SELECT v.id FROM quote_version v JOIN quote c ON c.id = v.quote_id WHERE c.rfq_id = $1)`,
			rfqID)
		e.mustCleanup(t, `UPDATE quote SET current_version_id = NULL WHERE rfq_id = $1`, rfqID)
		e.mustCleanup(t, `DELETE FROM quote_version WHERE quote_id IN (
		  SELECT id FROM quote WHERE rfq_id = $1)`, rfqID)
		e.mustCleanup(t, `DELETE FROM quote_status_change WHERE quote_id IN (
		  SELECT id FROM quote WHERE rfq_id = $1)`, rfqID)
		e.mustCleanup(t, `DELETE FROM quote WHERE rfq_id = $1`, rfqID)
		e.mustCleanup(t, `DELETE FROM rfq_status_change WHERE rfq_id = $1`, rfqID)
		e.mustCleanup(t, `DELETE FROM rfq WHERE id = $1`, rfqID)
	})
}

func TestRFQPipeline_PersistsTheMatchRealSearchDecided(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "RFQ pipeline")
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	ctx := context.Background()

	// Two lines on axes of their own, one product sitting on each. The cement product is a near
	// perfect match; the sand one is far enough away to be rejected.
	cement := e.seedProduct(t, accountID, "Cemento Portland 50kg", "bolsa de cemento")
	sand := e.seedProduct(t, accountID, "Arena fina", "arena")
	e.stock(t, accountID, branchID, cement)
	e.stock(t, accountID, branchID, sand)
	e.embedOn(t, cement, 0, 0.95)
	e.embedOn(t, sand, 4, 0.20)
	axes := map[string]int{"10 bolsas de cemento": 0, "arena": 2}

	unit := "bolsa"
	rationale := "el cliente pidió 10 bolsas"
	sandRationale := "no indicó cuánta arena"
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{
		{
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("10"),
			Unit:                 &unit,
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    rationale,
		},
		{
			RequestedDescription: "arena",
			Quantity:             decimal.Zero,
			Source:               domain.QuantitySourceUnresolved,
			QuantityRationale:    sandRationale,
		},
	}}, axes).CreateTextDraft(ctx,
		domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
			Role: domain.UserRoleAdmin},
		domain.TextRFQDraftInput{
			ChannelID: channelID, RawText: "10 bolsas de cemento y arena",
		})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want no error", err)
	}
	e.dropDraft(t, draft.RFQ.ID)

	if draft.RFQ.Status != domain.RFQStatusGenerated {
		t.Errorf("rfq status = %q, want GENERATED", draft.RFQ.Status)
	}
	if draft.Quote == nil || draft.Version == nil {
		t.Fatalf("draft = %+v, want a quote and its version", draft)
	}

	// Read the rows back rather than trusting the returned structs: the point of this test is the
	// SQL, and the ticket's criteria say persisted.
	rows, err := e.db.CrossAccount().Query(ctx,
		`SELECT requested_description, quantity, unit, product_id, confidence_score, match_status,
		        quantity_rationale
		 FROM quote_item WHERE version_id = $1 ORDER BY created_at, requested_description`,
		draft.Version.ID)
	if err != nil {
		t.Fatalf("read lines back: %v", err)
	}
	defer rows.Close()

	type stored struct {
		description string
		quantity    decimal.Decimal
		unit        *string
		productID   *uuid.UUID
		confidence  decimal.NullDecimal
		status      string
		rationale   *string
	}
	var lines []stored
	for rows.Next() {
		var line stored
		if err := rows.Scan(&line.description, &line.quantity, &line.unit, &line.productID,
			&line.confidence, &line.status, &line.rationale); err != nil {
			t.Fatalf("scan line: %v", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("stored %d lines, want 2", len(lines))
	}

	matched := lines[0]
	if matched.description != "10 bolsas de cemento" {
		t.Fatalf("first line = %q, want the cement one", matched.description)
	}
	if matched.status != string(domain.ItemMatchStatusMatched) {
		t.Errorf("cement status = %q, want MATCHED", matched.status)
	}
	if matched.productID == nil || *matched.productID != cement {
		t.Errorf("cement product = %v, want %v", matched.productID, cement)
	}
	// The alignment is the similarity, so the stored score is arithmetic rather than a guess.
	want := decimal.RequireFromString("0.9500")
	if !matched.confidence.Valid || !matched.confidence.Decimal.Equal(want) {
		t.Errorf("cement confidence = %v, want %s", matched.confidence, want)
	}
	if !matched.quantity.Equal(decimal.RequireFromString("10")) {
		t.Errorf("cement quantity = %s, want 10", matched.quantity)
	}
	if matched.rationale == nil || *matched.rationale != rationale {
		t.Errorf("cement rationale = %v, want %q", matched.rationale, rationale)
	}

	// The sand line is flagged and kept: nothing came close enough, and the quantity was never
	// given. It stays in the quote either way — that is the whole of US-07's premise.
	flagged := lines[1]
	if flagged.status != string(domain.ItemMatchStatusNoMatch) {
		t.Errorf("sand status = %q, want NO_MATCH", flagged.status)
	}
	if flagged.productID != nil {
		t.Errorf("sand product = %v, want none", flagged.productID)
	}
	if !flagged.quantity.IsZero() {
		t.Errorf("sand quantity = %s, want zero", flagged.quantity)
	}
	// It kept the score of the candidate it rejected, which is what tells a near miss from a line
	// nothing was offered for.
	if !flagged.confidence.Valid {
		t.Error("sand confidence is null, want the rejected candidate's score")
	}

	var versionNumber int
	var immutable bool
	var total decimal.Decimal
	if err := e.db.CrossAccount().QueryRow(ctx,
		`SELECT version_number, is_immutable, total FROM quote_version WHERE id = $1`,
		draft.Version.ID).Scan(&versionNumber, &immutable, &total); err != nil {
		t.Fatalf("read version back: %v", err)
	}
	if versionNumber != 1 || immutable || !total.IsZero() {
		t.Errorf("version = (%d, immutable %v, total %s), want (1, false, 0)",
			versionNumber, immutable, total)
	}

	var quoteStatus string
	if err := e.db.CrossAccount().QueryRow(ctx,
		`SELECT current_status FROM quote WHERE rfq_id = $1`, draft.RFQ.ID).Scan(&quoteStatus); err != nil {
		t.Fatalf("read quote back: %v", err)
	}
	if quoteStatus != string(domain.QuoteStatusDraft) {
		t.Errorf("quote status = %q, want DRAFT", quoteStatus)
	}
}

func TestRFQTextDraftRoute_KeepsTheOrderWhenNoModelIsBound(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "RFQ no model")
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	order := "Necesito 10 bolsas de cemento " + uuid.NewString()

	rec := e.do(t, request{
		method: http.MethodPost, path: "/v1/rfqs/text-drafts",
		token: e.tokenFor(t, seller), branch: branchID.String(),
		body: map[string]any{"channel_id": channelID, "raw_text": order},
	})

	// The environment binds no language model, so the pipeline refuses on the call that needed
	// one — an outage rather than the caller's mistake.
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body)
	}
	if code := errorCode(t, rec); code != string(domain.CodeAIUnavailable) {
		t.Errorf("code = %q, want %q", code, domain.CodeAIUnavailable)
	}

	// And the order survived it. This is the guarantee the two transactions exist for: the client
	// wrote something, and a model that could not read it must not lose it.
	var rfqID uuid.UUID
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id, status FROM rfq WHERE raw_text = $1`, order).Scan(&rfqID, &status); err != nil {
		t.Fatalf("the order was not stored: %v", err)
	}
	e.dropDraft(t, rfqID)
	if status != string(domain.RFQStatusReceived) {
		t.Errorf("rfq status = %q, want RECEIVED", status)
	}

	var quotes int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM quote WHERE rfq_id = $1`, rfqID).Scan(&quotes); err != nil {
		t.Fatalf("count quotes: %v", err)
	}
	if quotes != 0 {
		t.Errorf("quotes = %d, want none: the order produced no materials", quotes)
	}
}

func TestChannelsRoute_ListsOnlyTheSelectedBranch(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Channel listing")
	otherAccountID, otherBranchID := e.seedAccount(t, "Channel listing other")
	mine := e.seedIntakeChannel(t, accountID, branchID)
	e.seedIntakeChannel(t, otherAccountID, otherBranchID)
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	rec := e.do(t, request{
		method: http.MethodGet, path: "/v1/channels",
		token: e.tokenFor(t, admin), branch: branchID.String(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var body struct {
		Items []struct {
			ID       uuid.UUID `json:"id"`
			BranchID uuid.UUID `json:"branch_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %s: %v", rec.Body, err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != mine {
		t.Fatalf("items = %+v, want only this branch's channel %v", body.Items, mine)
	}
	if body.Items[0].BranchID != branchID {
		t.Errorf("branch = %v, want %v", body.Items[0].BranchID, branchID)
	}
}
