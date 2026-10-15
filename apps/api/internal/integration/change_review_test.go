//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// sentDecisions gives the sent version what a seller decided on it: a 5% discount typed on the
// cement line, and the cement promotion suppressed.
func (e *env) sentDecisions(t *testing.T, quote handlerQuote) {
	t.Helper()
	ctx := context.Background()
	promotion := e.promotionOn(t, quote, "Cemento 10%", quote.cementID)
	var line uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM quote_item WHERE version_id = $1`, quote.sentID).Scan(&line); err != nil {
		t.Fatal(err)
	}
	account, sent := quote.seed.tenant.AccountID, quote.sentID
	for _, stmt := range []struct {
		sql  string
		args []any
	}{{
		`WITH typed AS (
		   INSERT INTO quote_discount (account_id, quote_version_id, condition_type, scope, origin,
		          amount, action_type, action_value, description)
		   VALUES ($1, $2, 'PER_ITEM', 'ITEM', 'MANUAL_SELLER', 5, 'PERCENTAGE', 5, 'Cliente frecuente')
		   RETURNING id)
		 INSERT INTO quote_discount_item (account_id, quote_discount_id, quote_item_id)
		 SELECT $1, typed.id, $3 FROM typed`, []any{account, sent, line}}, {
		`WITH earned AS (
		   INSERT INTO quote_discount (account_id, quote_version_id, promotion_id, condition_type,
		          scope, origin, amount, action_type, action_value, suppressed_by_seller)
		   VALUES ($1, $2, $4, 'PER_ITEM', 'ITEM', 'AUTOMATIC', 10, 'PERCENTAGE', 10, TRUE)
		   RETURNING id)
		 INSERT INTO quote_discount_item (account_id, quote_discount_id, quote_item_id)
		 SELECT $1, earned.id, $3 FROM earned`, []any{account, sent, line, promotion}},
	} {
		if _, err := e.db.CrossAccount().Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed the sent decisions: %v", err)
		}
	}
}

type draftDiscount struct {
	name       string
	origin     string
	amount     decimal.Decimal
	suppressed bool
	covers     int
}

func (e *env) draftDiscounts(t *testing.T, versionID uuid.UUID) []draftDiscount {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT coalesce(p.name, d.description), d.origin, d.amount, d.suppressed_by_seller,
		        (SELECT count(*) FROM quote_discount_item l
		          JOIN quote_item i ON i.id = l.quote_item_id AND i.version_id = d.quote_version_id
		          WHERE l.quote_discount_id = d.id)
		 FROM quote_discount d LEFT JOIN promotion p ON p.id = d.promotion_id
		 WHERE d.quote_version_id = $1 ORDER BY d.origin, 1`, versionID)
	if err != nil {
		t.Fatal(err)
	}
	discounts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (draftDiscount, error) {
		var d draftDiscount
		err := row.Scan(&d.name, &d.origin, &d.amount, &d.suppressed, &d.covers)
		return d, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return discounts
}

// assertCarried checks a fresh draft holds the sent decisions, each on the copied cement line:
// the typed 5% re-computed on 2 × 50, the promotion still suppressed, and a total of 100 − 5.
func (e *env) assertCarried(t *testing.T, draftID uuid.UUID) {
	t.Helper()
	got := e.draftDiscounts(t, draftID)
	if len(got) != 2 {
		t.Fatalf("draft discounts = %+v, want the typed one and the suppressed promotion", got)
	}
	automatic, typed := got[0], got[1]
	if automatic.origin != "AUTOMATIC" || !automatic.suppressed || automatic.covers != 1 {
		t.Errorf("promotion = %+v, want it still suppressed on the copied line", automatic)
	}
	if typed.origin != "MANUAL_SELLER" || typed.suppressed || typed.covers != 1 ||
		!typed.amount.Equal(decimal.NewFromInt(5)) {
		t.Errorf("typed discount = %+v, want 5%% of 100 on the copied line", typed)
	}
	if total := e.storedVersionTotal(t, draftID); !total.Equal(decimal.NewFromInt(95)) {
		t.Errorf("draft total = %s, want 95", total)
	}
}

// A draft the handler opens starts from what the customer saw, even when the AI changes nothing.
func TestChangeReview_TheHandlerDraftCarriesTheSellersDiscounts(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Carry handler")
	e.sentDecisions(t, quote)

	e.runBatch(t, quote, requiresSeller, "¿y el flete?")

	e.assertCarried(t, e.quoteState(t, quote.quoteID).currentID)
}

// The webapp's «Pedir cambios» opens the draft itself, before the handler runs.
func TestChangeReview_TheWebappDraftCarriesTheSellersDiscounts(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Carry webapp")
	e.sentDecisions(t, quote)
	message := "más barato"

	if _, err := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}).
		RespondPublic(context.Background(), quote.sentToken,
			domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message}); err != nil {
		t.Fatalf("RespondPublic() = %v", err)
	}

	e.assertCarried(t, e.quoteState(t, quote.quoteID).currentID)
}

// Reactivating a closed quote to edit it starts from the discounts the customer saw.
func TestChangeReview_AReactivatedDraftCarriesTheSellersDiscounts(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Carry reactivate")
	e.sentDecisions(t, quote)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET current_status = 'REJECTED' WHERE id = $1`, quote.quoteID); err != nil {
		t.Fatal(err)
	}
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))

	if rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + quote.quoteID.String() + "/reactivate",
		token: admin, branch: quote.seed.tenant.BranchID.String(),
		body: map[string]any{"mode": "EDIT"}}); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: status = %d: %s", rec.Code, rec.Body)
	}

	e.assertCarried(t, e.quoteState(t, quote.quoteID).currentID)
}

// Discarding the AI's changes goes back to exactly what the customer received, the seller's own
// discounts included.
func TestChangeReview_DiscardKeepsTheSellersDiscounts(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Carry discard")
	e.sentDecisions(t, quote)
	e.runBatch(t, quote, requantify(4), "que sean 4")
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))

	if rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + quote.quoteID.String() + "/discard-ai-changes",
		token: admin, branch: quote.seed.tenant.BranchID.String()}); rec.Code != http.StatusOK {
		t.Fatalf("discard: status = %d: %s", rec.Code, rec.Body)
	}

	e.assertCarried(t, e.quoteState(t, quote.quoteID).currentID)
}

type comparedLine struct {
	Change   string `json:"change"`
	Original *struct {
		Description string `json:"description"`
	} `json:"original"`
	Requested *struct {
		Description string `json:"description"`
	} `json:"requested"`
}

/*
 * The swapped cement is rewritten in place, which moves its row. The draft keeps the order the
 * customer sent, and the comparison pairs each line with the one it was copied from.
 */
func TestChangeReview_ASwapKeepsTheLineOrderAndPairsWithItsLine(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Pairing swap")
	accountID, branchID := quote.seed.tenant.AccountID, quote.seed.tenant.BranchID
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena gruesa", "30", nil)
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, sand) })
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description, quantity,
		                         unit_price_snapshot, subtotal, created_at)
		 VALUES ($1, $2, $3, 'arena', 3, 30, 90, now() + interval '1 second')`,
		accountID, quote.sentID, sand); err != nil {
		t.Fatal(err)
	}
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "40", domain.ProductAlternativeEconomy)

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "está caro")

	draft := e.quoteState(t, quote.quoteID).currentID
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT requested_description FROM quote_item WHERE version_id = $1 ORDER BY created_at, id`, draft)
	if err != nil {
		t.Fatal(err)
	}
	order, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "Cemento económico" || order[1] != "arena" {
		t.Errorf("draft order = %v, want the swapped cement still first", order)
	}
	var instants int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(DISTINCT created_at) FROM quote_item WHERE version_id = $1`, draft).
		Scan(&instants); err != nil {
		t.Fatal(err)
	}
	if instants != 2 {
		t.Errorf("copied lines share %d instants for 2 lines, want one each to keep their order", instants)
	}

	admin := e.tokenFor(t, e.seedUser(t, accountID, domain.UserRoleAdmin))
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + quote.seed.draft.RFQ.ID.String(),
		token: admin, branch: branchID.String()})
	var detail struct {
		ChangesRequested *struct {
			Lines []comparedLine `json:"lines"`
		} `json:"changes_requested"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &detail) != nil ||
		detail.ChangesRequested == nil {
		t.Fatalf("detail: status = %d: %s", rec.Code, rec.Body)
	}
	lines := detail.ChangesRequested.Lines
	if len(lines) != 2 || lines[0].Change != "MODIFIED" || lines[0].Requested == nil ||
		lines[0].Requested.Description != "Cemento económico" || lines[1].Change != "UNCHANGED" ||
		lines[1].Original == nil || lines[1].Original.Description != "arena" {
		t.Errorf("comparison = %+v, want the cement paired with its swap and the sand unchanged", lines)
	}
}

func (e *env) firstLine(t *testing.T, versionID uuid.UUID) (uuid.UUID, decimal.NullDecimal, string) {
	t.Helper()
	var id uuid.UUID
	var price decimal.NullDecimal
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id, unit_price_snapshot, match_status FROM quote_item WHERE version_id = $1
		 ORDER BY created_at, id LIMIT 1`, versionID).Scan(&id, &price, &status); err != nil {
		t.Fatal(err)
	}
	return id, price, status
}

/*
 * A priced quote with a line the catalog left unsure cannot be sent, and says so with its own
 * code. Confirming the line settles it on its product and keeps its price, and the send goes out.
 */
func TestChangeReview_AnUnresolvedLineBlocksTheSendUntilConfirmed(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Unresolved line", false)
	version := *seed.draft.Quote.CurrentVersionID
	line, _, _ := e.firstLine(t, version)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_item SET match_status = 'AMBIGUOUS' WHERE id = $1`, line); err != nil {
		t.Fatal(err)
	}
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	send := func() error {
		_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
			domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199"})
		return err
	}

	if err := send(); domain.CodeOf(err) != domain.CodeQuoteItemsUnresolved {
		t.Fatalf("Send() = %v (code %q), want QUOTE_ITEMS_UNRESOLVED", err, domain.CodeOf(err))
	}

	admin := e.tokenFor(t, e.seedUser(t, seed.tenant.AccountID, domain.UserRoleAdmin))
	if rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + seed.draft.Quote.ID.String() + "/items/" + line.String(),
		token: admin, branch: seed.tenant.BranchID.String(),
		body: map[string]any{"confirm_match": true}}); rec.Code != http.StatusOK {
		t.Fatalf("confirm: status = %d: %s", rec.Code, rec.Body)
	}
	_, price, status := e.firstLine(t, version)
	if status != "MATCHED" {
		t.Errorf("status = %s, want MATCHED", status)
	}
	assertStored(t, "confirmed price", price, "50.00")
	if err := send(); err != nil {
		t.Fatalf("Send() after confirming = %v, want it out", err)
	}
}

// On a priced quote, the product chosen for a line arrives with the branch's price, and a line
// with no product cannot be confirmed.
func TestChangeReview_AProductChosenOnAPricedQuoteTakesTheBranchPrice(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Priced choice", false)
	version := *seed.draft.Quote.CurrentVersionID
	line, _, _ := e.firstLine(t, version)
	floor := "60"
	lime := e.seedPricedProduct(t, seed.tenant.AccountID, seed.tenant.BranchID, "Cal hidratada", "70", &floor)
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, lime) })
	admin := e.tokenFor(t, e.seedUser(t, seed.tenant.AccountID, domain.UserRoleAdmin))
	patch := func(body map[string]any) int {
		return e.do(t, request{method: http.MethodPatch,
			path:  "/v1/quotes/" + seed.draft.Quote.ID.String() + "/items/" + line.String(),
			token: admin, branch: seed.tenant.BranchID.String(), body: body}).Code
	}

	if code := patch(map[string]any{"product_id": lime}); code != http.StatusOK {
		t.Fatalf("choose: status = %d", code)
	}
	lines := e.storedLines(t, version)
	assertStored(t, "unit price", lines[0].unitPrice, "70.00")
	assertStored(t, "subtotal", lines[0].subtotal, "140.00")
	assertStored(t, "price floor", lines[0].minPrice, "60.00")

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_item SET product_id = NULL, match_status = 'NO_MATCH' WHERE id = $1`, line); err != nil {
		t.Fatal(err)
	}
	if code := patch(map[string]any{"confirm_match": true}); code != http.StatusUnprocessableEntity {
		t.Errorf("confirming a line with no product: status = %d, want 422", code)
	}
}
