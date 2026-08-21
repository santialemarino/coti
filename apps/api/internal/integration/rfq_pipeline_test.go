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
}

func (s stagedExtractor) Extract(
	_ context.Context, _ string,
) ([]domain.ExtractedRFQLine, error) {
	return s.lines, nil
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
		// The candidates go first: a foreign key on quote_item_alternative points at the lines.
		e.mustCleanup(t, `DELETE FROM quote_item_alternative WHERE quote_item_id IN (
		  SELECT i.id FROM quote_item i
		  JOIN quote_version v ON v.id = i.version_id
		  JOIN quote c ON c.id = v.quote_id WHERE c.rfq_id = $1)`, rfqID)
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
	// Deliberately named nothing like the second line: a shared word would let the lexical half
	// offer it, and then whichever half won would decide the status instead of the arithmetic.
	other := e.seedProduct(t, accountID, "Cal hidratada", "cal")
	e.stock(t, accountID, branchID, cement)
	e.stock(t, accountID, branchID, other)
	e.embedOn(t, cement, 0, 0.95)
	e.embedOn(t, other, 2, 0.20)
	axes := map[string]int{"10 bolsas de cemento": 0, "membrana liquida": 2}

	unit := "bolsa"
	rationale := "el cliente pidió 10 bolsas"
	unresolvedRationale := "no indicó cuánta membrana"
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{
		{
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("10"),
			Unit:                 &unit,
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    rationale,
		},
		{
			RequestedDescription: "membrana liquida",
			Quantity:             decimal.Zero,
			Source:               domain.QuantitySourceUnresolved,
			QuantityRationale:    unresolvedRationale,
		},
	}}, axes).CreateTextDraft(ctx,
		domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
			Role: domain.UserRoleAdmin},
		domain.TextRFQDraftInput{
			ChannelID: channelID, RawText: "10 bolsas de cemento y membrana liquida",
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

	// The second line is flagged and kept: nothing came close enough, and the quantity was never
	// given. It stays in the quote either way — that is the whole of US-07's premise.
	flagged := lines[1]
	if flagged.description != "membrana liquida" {
		t.Fatalf("second line = %q, want the unresolved one", flagged.description)
	}
	if flagged.status != string(domain.ItemMatchStatusNoMatch) {
		t.Errorf("sand status = %q, want NO_MATCH", flagged.status)
	}
	if flagged.productID != nil {
		t.Errorf("sand product = %v, want none", flagged.productID)
	}
	if !flagged.quantity.IsZero() {
		t.Errorf("sand quantity = %s, want zero", flagged.quantity)
	}
	// It kept the score of the candidate it rejected — 0.20 is the alignment the product was
	// embedded at — which is what tells a near miss from a line nothing was offered for.
	wantRejected := decimal.RequireFromString("0.2000")
	if !flagged.confidence.Valid || !flagged.confidence.Decimal.Equal(wantRejected) {
		t.Errorf("flagged confidence = %v, want the rejected candidate's %s", flagged.confidence,
			wantRejected)
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

func TestRFQPipeline_PersistsDerivedQuantitiesAndDifferentUnitsForOneProduct(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "RFQ units")
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)

	cement := e.seedProduct(t, accountID, "Cemento Portland 50kg", "bolsa de cemento")
	e.stock(t, accountID, branchID, cement)
	e.embedOn(t, cement, 0, 0.95)

	bag := "bolsa"
	pallet := "pallet"
	lines := []domain.ExtractedRFQLine{
		{
			RequestedDescription: "10 bolsas de cemento",
			Quantity:             decimal.RequireFromString("10"),
			Unit:                 &bag,
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    "el cliente pidió 10 bolsas",
		},
		{
			RequestedDescription: "2 pallets de cemento",
			Quantity:             decimal.RequireFromString("2"),
			Unit:                 &pallet,
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    "el cliente pidió 2 pallets",
		},
		{
			RequestedDescription: "3 pallets de 50 bolsas de cemento",
			Quantity:             decimal.RequireFromString("150"),
			Unit:                 &bag,
			Source:               domain.QuantitySourceDerived,
			QuantityRationale:    "3 pallets de 50 bolsas son 150 bolsas",
		},
	}
	axes := map[string]int{
		"10 bolsas de cemento":              0,
		"2 pallets de cemento":              0,
		"3 pallets de 50 bolsas de cemento": 0,
	}
	draft, err := e.pipeline(t, stagedExtractor{lines: lines}, axes).CreateTextDraft(
		context.Background(),
		domain.Tenant{
			AccountID: accountID,
			BranchID:  branchID,
			UserID:    seller.ID,
			Role:      domain.UserRoleAdmin,
		},
		domain.TextRFQDraftInput{
			ChannelID: channelID,
			RawText:   "10 bolsas de cemento, 2 pallets de cemento y 3 pallets de 50 bolsas",
		},
	)
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want no error", err)
	}
	e.dropDraft(t, draft.RFQ.ID)
	if draft.Version == nil {
		t.Fatal("draft carries no version")
	}

	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT requested_description, quantity, unit, product_id, match_status, quantity_rationale
		 FROM quote_item WHERE version_id = $1`, draft.Version.ID)
	if err != nil {
		t.Fatalf("read lines back: %v", err)
	}
	defer rows.Close()

	type storedLine struct {
		quantity  decimal.Decimal
		unit      *string
		productID *uuid.UUID
		status    domain.ItemMatchStatus
		rationale *string
	}
	stored := make(map[string]storedLine, len(lines))
	for rows.Next() {
		var description string
		var line storedLine
		if err := rows.Scan(&description, &line.quantity, &line.unit, &line.productID, &line.status,
			&line.rationale); err != nil {
			t.Fatalf("scan line: %v", err)
		}
		stored[description] = line
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read lines: %v", err)
	}
	if len(stored) != len(lines) {
		t.Fatalf("stored %d distinct lines, want %d", len(stored), len(lines))
	}

	for _, want := range lines {
		got, ok := stored[want.RequestedDescription]
		if !ok {
			t.Errorf("line %q was discarded", want.RequestedDescription)
			continue
		}
		if !got.quantity.Equal(want.Quantity) {
			t.Errorf("line %q quantity = %s, want %s", want.RequestedDescription, got.quantity,
				want.Quantity)
		}
		if got.unit == nil || want.Unit == nil || *got.unit != *want.Unit {
			t.Errorf("line %q unit = %v, want %v", want.RequestedDescription, got.unit, want.Unit)
		}
		if got.productID == nil || *got.productID != cement {
			t.Errorf("line %q product = %v, want cement %s", want.RequestedDescription,
				got.productID, cement)
		}
		if got.status != domain.ItemMatchStatusMatched {
			t.Errorf("line %q status = %q, want MATCHED", want.RequestedDescription, got.status)
		}
		if got.rationale == nil || *got.rationale != want.QuantityRationale {
			t.Errorf("line %q rationale = %v, want %q", want.RequestedDescription, got.rationale,
				want.QuantityRationale)
		}
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

func TestWhatsAppMockRoute_IsAbsentInProduction(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) { cfg.Environment = config.EnvironmentProduction })
	accountID, branchID := e.seedAccount(t, "WhatsApp mock in production")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	channelID := e.seedIntakeChannel(t, accountID, branchID)

	rec := e.do(t, request{
		method: http.MethodPost, path: "/v1/dev/whatsapp/messages",
		token: e.tokenFor(t, seller), branch: branchID.String(),
		body: map[string]any{"channel_id": channelID, "from": "+5491122334455", "text": "cemento"},
	})

	// The route is registered on the environment, not guarded inside the handler: shipped to
	// production it would let anyone post an order attributed to WhatsApp with no webhook behind it.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body %s, want 404: the development route reached a production router",
			rec.Code, rec.Body)
	}
}

func TestRFQTextDraftRoute_CarriesItsOwnAllowance(t *testing.T) {
	// The route is billed per call by a provider, so it must not be sharing the global allowance:
	// 300 a minute is a spend nobody authorised, and the global counter is what it would fall back
	// to if the middleware were left off.
	const allowance = 2
	e := newEnv(t, func(cfg *config.Config) {
		cfg.RateLimit = config.RateLimitConfig{
			Enabled: true, Window: time.Minute, Global: 100, Credentials: 100, Signup: 100,
			Mail: 100, MailPerAddress: 100, AI: allowance,
		}
	})
	accountID, branchID := e.seedAccount(t, "RFQ allowance")
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, seller)

	send := func() int {
		order := "cemento " + uuid.NewString()
		rec := e.do(t, request{
			method: http.MethodPost, path: "/v1/rfqs/text-drafts",
			token: token, branch: branchID.String(),
			body: map[string]any{"channel_id": channelID, "raw_text": order},
		})
		var rfqID uuid.UUID
		if err := e.db.CrossAccount().QueryRow(context.Background(),
			`SELECT id FROM rfq WHERE raw_text = $1`, order).Scan(&rfqID); err == nil {
			e.dropDraft(t, rfqID)
		}
		return rec.Code
	}

	// No model is bound, so a call that gets through answers 503 — which is still a call that
	// reached the pipeline and would have been paid for.
	for i := range allowance {
		if code := send(); code != http.StatusServiceUnavailable {
			t.Fatalf("call %d = %d, want %d inside the allowance", i+1, code,
				http.StatusServiceUnavailable)
		}
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Errorf("call past the allowance = %d, want 429", code)
	}
}
