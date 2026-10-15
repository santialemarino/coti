//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// alternativeOf seeds a priced product (unpriced when price is empty) and links it to base.
func (e *env) alternativeOf(t *testing.T, quote handlerQuote, base uuid.UUID, name, price string,
	kind domain.ProductAlternativeType,
) uuid.UUID {
	t.Helper()
	accountID, branchID := quote.seed.tenant.AccountID, quote.seed.tenant.BranchID
	var product uuid.UUID
	if price == "" {
		product = e.seedProduct(t, accountID, name, name)
		e.stock(t, accountID, branchID, product)
	} else {
		product = e.seedPricedProduct(t, accountID, branchID, name, price, nil)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product_alternative (account_id, base_product_id, alternative_product_id, type)
		 VALUES ($1, $2, $3, $4)`, accountID, base, product, kind); err != nil {
		t.Fatalf("seed alternative: %v", err)
	}
	// Registered after the product, so it runs before the product's own teardown.
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM quote_item_alternative WHERE product_id = $1`, product)
		e.mustCleanup(t, `DELETE FROM quote_discount_item WHERE quote_item_id IN (
			SELECT id FROM quote_item WHERE product_id = $1)`, product)
		e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, product)
		e.mustCleanup(t, `DELETE FROM product_alternative WHERE alternative_product_id = $1`, product)
	})
	return product
}

func intent(goal domain.ChangeRequestIntent, lines ...uuid.UUID,
) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		return &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelComplex,
			Interpretation: "El cliente quiere otra opción.", Intent: &goal, IntentItemIDs: lines}, nil
	}
}

type recordedRun struct {
	Outcome         string                          `json:"outcome"`
	NoMatchOrigin   *string                         `json:"no_match_origin"`
	NoMatchReason   *string                         `json:"no_match_reason"`
	Intent          *string                         `json:"intent"`
	IntentItemIDs   []uuid.UUID                     `json:"intent_item_ids"`
	StrategiesTried []string                        `json:"strategies_tried"`
	Operations      []domain.ChangeRequestOperation `json:"operations"`
}

func (e *env) lastRun(t *testing.T, quoteID uuid.UUID) recordedRun {
	t.Helper()
	var raw []byte
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT d.ai_proposal FROM handler_decision d JOIN quote_version v ON v.id = d.quote_version_id
		 WHERE v.quote_id = $1 ORDER BY d.created_at DESC, d.id DESC LIMIT 1`, quoteID).
		Scan(&raw); err != nil {
		t.Fatalf("read the recorded run: %v", err)
	}
	var run recordedRun
	if err := json.Unmarshal(raw, &run); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return run
}

func (e *env) draftProducts(t *testing.T, versionID uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT product_id FROM quote_item WHERE version_id = $1 ORDER BY requested_description`, versionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var products []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		products = append(products, id)
	}
	return products
}

func (e *env) runBatch(t *testing.T, quote handlerQuote,
	answer func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error), body string,
) *scriptedProposer {
	t.Helper()
	proposer := &scriptedProposer{answer: answer}
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, body)
	if err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	e.finishBatch(t, batch.ID)
	return proposer
}

/*
 * «Está caro» swaps the cement (2 × 50) for its cheapest ECONOMY alternative the branch prices
 * below 50: not the dearer one, not the unpriced one, and never a PREMIUM. The swapped line is
 * priced at once and the quantity is kept.
 */
func TestConversationIntents_LowerTotalPriceTakesTheCheapestCheaperAlternative(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent cheaper")
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico A", "40", domain.ProductAlternativeEconomy)
	cheapest := e.alternativeOf(t, quote, quote.cementID, "Cemento económico B", "35",
		domain.ProductAlternativeEconomy)
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico caro", "60", domain.ProductAlternativeEconomy)
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico sin precio", "", domain.ProductAlternativeEconomy)
	e.alternativeOf(t, quote, quote.cementID, "Cemento premium", "20", domain.ProductAlternativePremium)

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "está caro")

	state := e.quoteState(t, quote.quoteID)
	if products := e.draftProducts(t, state.currentID); len(products) != 1 || products[0] != cheapest {
		t.Fatalf("draft products = %v, want only the cheapest cheaper ECONOMY %s", products, cheapest)
	}
	lines := e.storedLines(t, state.currentID)
	if lines[0].description != "Cemento económico B" || !lines[0].quantity.Equal(decimal.NewFromInt(2)) {
		t.Errorf("line = %+v, want the alternative at the same quantity", lines[0])
	}
	assertStored(t, "unit price", lines[0].unitPrice, "35.00")
	assertStored(t, "subtotal", lines[0].subtotal, "70.00")
	if got := e.storedVersionTotal(t, state.currentID); !got.Equal(decimal.NewFromInt(70)) {
		t.Errorf("draft total = %s, want 70", got)
	}
	run := e.lastRun(t, quote.quoteID)
	if run.Outcome != "APPLIED" || run.Intent == nil || *run.Intent != "LOWER_TOTAL_PRICE" ||
		len(run.StrategiesTried) != 1 || run.StrategiesTried[0] != "FIND_CHEAPER_ALTERNATIVES" ||
		len(run.Operations) != 1 || run.Operations[0].Type != domain.ChangeRequestOperationReplaceItem {
		t.Errorf("run = %+v, want an applied REPLACE_ITEM from the cheaper-alternatives recipe", run)
	}
}

/*
 * The recipe is judged by the engine: with 10% off the cement, the draft stands at 90, and an
 * ECONOMY cement at 46 the promotion does not cover ends at 92. The swap is dropped and the seller
 * gets a no-match saying the recipes ran out.
 */
func TestConversationIntents_LowerTotalPriceIsDroppedUnlessTheEngineEndsLower(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent not lower")
	e.cementPromotion(t, quote)
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "46", domain.ProductAlternativeEconomy)

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "bajame el precio")

	state := e.quoteState(t, quote.quoteID)
	if products := e.draftProducts(t, state.currentID); len(products) != 1 || products[0] != quote.cementID {
		t.Fatalf("draft products = %v, want the cement kept", products)
	}
	run := e.lastRun(t, quote.quoteID)
	if run.Outcome != "NO_MATCH" || run.NoMatchOrigin == nil || *run.NoMatchOrigin != "PIPELINE_EXHAUSTED" ||
		run.NoMatchReason == nil || *run.NoMatchReason != "PIPELINE_EXHAUSTED" ||
		len(run.StrategiesTried) != 1 || run.StrategiesTried[0] != "FIND_CHEAPER_ALTERNATIVES" ||
		run.Intent == nil || len(run.Operations) != 0 {
		t.Errorf("run = %+v, want a pipeline-exhausted no-match naming the strategy", run)
	}
}

// Only an active alternative sold in the line's own unit is a substitute: a cheaper one in another
// pack, or one no longer sold, is never swapped in.
func TestConversationIntents_RecipesSkipInactiveAndOtherUnitAlternatives(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent substitutes")
	inactive := e.alternativeOf(t, quote, quote.cementID, "Cemento discontinuado", "20",
		domain.ProductAlternativeEconomy)
	otherUnit := e.alternativeOf(t, quote, quote.cementID, "Cemento 25 kg", "25",
		domain.ProductAlternativeEconomy)
	valid := e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "40",
		domain.ProductAlternativeEconomy)
	for _, stmt := range []struct {
		sql string
		id  uuid.UUID
	}{
		{`UPDATE product SET is_active = FALSE WHERE id = $1`, inactive},
		{`UPDATE product SET unit = 'bolsa 25 kg' WHERE id = $1`, otherUnit},
	} {
		if _, err := e.db.CrossAccount().Exec(context.Background(), stmt.sql, stmt.id); err != nil {
			t.Fatal(err)
		}
	}

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "más barato")

	if products := e.draftProducts(t, e.quoteState(t, quote.quoteID).currentID); len(products) != 1 ||
		products[0] != valid {
		t.Errorf("draft products = %v, want the active same-unit alternative %s", products, valid)
	}
}

// A goal tied to one line swaps that line only.
func TestConversationIntents_RecipesActOnTheNamedLinesOnly(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent named lines")
	accountID, branchID := quote.seed.tenant.AccountID, quote.seed.tenant.BranchID
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena gruesa", "30", nil)
	e.alternativeOf(t, quote, sand, "Arena de río", "20", domain.ProductAlternativeEconomy)
	cheaperCement := e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "40",
		domain.ProductAlternativeEconomy)
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, sand) })
	e.runBatch(t, quote, requiresSeller, "hola")
	draft := e.quoteState(t, quote.quoteID).currentID
	var cementLine uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM quote_item WHERE version_id = $1`, draft).Scan(&cementLine); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description, quantity,
		                         unit_price_snapshot, subtotal)
		 VALUES ($1, $2, $3, 'arena', 3, 30, 90)`, accountID, draft, sand); err != nil {
		t.Fatal(err)
	}

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice, cementLine), "el cemento está caro")

	products := e.draftProducts(t, draft)
	if len(products) != 2 || !slices.Contains(products, cheaperCement) || !slices.Contains(products, sand) {
		t.Errorf("draft products = %v, want the cement swapped and the sand untouched", products)
	}
	if run := e.lastRun(t, quote.quoteID); len(run.IntentItemIDs) != 1 || run.IntentItemIDs[0] != cementLine {
		t.Errorf("recorded lines = %v, want the cement line the goal was tied to", run.IntentItemIDs)
	}
}

// A complex goal is checked against the matrix again under the lock: a quote archived while the
// model answered takes no recipe.
func TestConversationIntents_ComplexIsRecheckedAgainstTheMatrix(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent matrix recheck")
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "30", domain.ProductAlternativeEconomy)
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "está caro")
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE quote SET archived_at = now() WHERE id = $1`, quote.quoteID); err != nil {
			return nil, err
		}
		return intent(domain.ChangeRequestIntentLowerTotalPrice)(in)
	}}

	err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch)

	if !errors.Is(err, domain.ErrBatchUnprocessable) {
		t.Fatalf("Handle() = %v, want ErrBatchUnprocessable", err)
	}
	state := e.quoteState(t, quote.quoteID)
	if products := e.draftProducts(t, state.currentID); len(products) != 1 || products[0] != quote.cementID ||
		len(e.decisionsOf(t, quote.quoteID)) != 0 {
		t.Errorf("draft products = %v, want the archived quote's draft untouched and no decision", products)
	}
}

// Better quality takes the cheapest PREMIUM the branch prices, with no cap on how much it adds,
// on the lines the customer named.
func TestConversationIntents_ImproveQualityTakesTheCheapestPremium(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent premium")
	e.alternativeOf(t, quote, quote.cementID, "Cemento premium caro", "90", domain.ProductAlternativePremium)
	premium := e.alternativeOf(t, quote, quote.cementID, "Cemento premium", "70",
		domain.ProductAlternativePremium)
	e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "30", domain.ProductAlternativeEconomy)
	e.runBatch(t, quote, requiresSeller, "hola")
	draft := e.quoteState(t, quote.quoteID).currentID
	var line uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM quote_item WHERE version_id = $1`, draft).Scan(&line); err != nil {
		t.Fatal(err)
	}

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentImproveQuality, line), "quiero mejor cemento")

	if products := e.draftProducts(t, draft); len(products) != 1 || products[0] != premium {
		t.Fatalf("draft products = %v, want the cheapest PREMIUM %s", products, premium)
	}
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(140)) {
		t.Errorf("draft total = %s, want 140: quality may raise the total", got)
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "APPLIED" ||
		len(run.StrategiesTried) != 1 || run.StrategiesTried[0] != "IMPROVE_QUALITY" {
		t.Errorf("run = %+v, want an applied quality recipe", run)
	}
}

// A goal with no alternative to reach it is left to the seller, never guessed.
func TestConversationIntents_NoAlternativeIsAPipelineExhaustedNoMatch(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent nothing")

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentImproveQuality), "algo mejor")

	if run := e.lastRun(t, quote.quoteID); run.Outcome != "NO_MATCH" || run.NoMatchReason == nil ||
		*run.NoMatchReason != "PIPELINE_EXHAUSTED" || len(run.StrategiesTried) != 1 {
		t.Errorf("run = %+v, want a pipeline-exhausted no-match", run)
	}
	state := e.quoteState(t, quote.quoteID)
	if products := e.draftProducts(t, state.currentID); len(products) != 1 || products[0] != quote.cementID {
		t.Errorf("draft products = %v, want the cement untouched", products)
	}
}

func addDiscount(promotionID uuid.UUID, extra ...domain.ChangeRequestOperation,
) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		return &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelAtomic,
			Interpretation: "Pide la promoción.", Operations: append([]domain.ChangeRequestOperation{
				{Type: domain.ChangeRequestOperationAddDiscount, PromotionID: &promotionID}}, extra...)}, nil
	}
}

func (e *env) promotionOn(t *testing.T, quote handlerQuote, name string, product uuid.UUID) uuid.UUID {
	t.Helper()
	admin := e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin)
	return e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
		"name": name, "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": "10", "priority": 5, "conditions": []map[string]any{{"product_id": product}},
	})
}

func (e *env) liveDiscounts(t *testing.T, versionID, promotionID uuid.UUID) (live, suppressed int) {
	t.Helper()
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE NOT suppressed_by_seller), count(*) FILTER (WHERE suppressed_by_seller)
		 FROM quote_discount WHERE quote_version_id = $1 AND promotion_id = $2`, versionID, promotionID).
		Scan(&live, &suppressed); err != nil {
		t.Fatal(err)
	}
	return live, suppressed
}

/*
 * The promotion the customer asks for was suppressed by the seller: the handler restores it and the
 * engine computes its amount. The model was told which promotions are in force.
 */
func TestConversationIntents_AddDiscountRestoresASuppressedPromotion(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent restore promo")
	promotion := e.promotionOn(t, quote, "Cemento 10%", quote.cementID)
	e.runBatch(t, quote, requantify(2), "dejalo igual")
	draft := e.quoteState(t, quote.quoteID).currentID
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_discount SET suppressed_by_seller = TRUE WHERE quote_version_id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	if live, suppressed := e.liveDiscounts(t, draft, promotion); live != 0 || suppressed != 1 {
		t.Fatalf("before: live = %d, suppressed = %d, want the promotion suppressed", live, suppressed)
	}

	proposer := e.runBatch(t, quote, addDiscount(promotion), "aplicame la promo del cemento")

	if calls := proposer.calls(); len(calls[0].Promotions) != 1 || calls[0].Promotions[0].ID != promotion {
		t.Errorf("model promotions = %+v, want the one in force", calls[0].Promotions)
	}
	if live, _ := e.liveDiscounts(t, draft, promotion); live != 1 {
		t.Fatalf("live applications = %d, want the promotion restored", live)
	}
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(90)) {
		t.Errorf("draft total = %s, want 100 - 10%% = 90 from the engine", got)
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "APPLIED" {
		t.Errorf("run = %+v, want applied", run)
	}
}

/*
 * A promotion the quote does not earn is a no-match, and it takes the rest of the proposal with
 * it: the requantify riding beside it is rolled back too.
 */
func TestConversationIntents_AddDiscountTheQuoteDoesNotEarnIsANoMatch(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent unearned promo")
	other := e.seedPricedProduct(t, quote.seed.tenant.AccountID, quote.seed.tenant.BranchID, "Arena", "30", nil)
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM promotion_condition_item WHERE product_id = $1`, other)
	})
	promotion := e.promotionOn(t, quote, "Arena 10%", other)
	quantity := decimal.NewFromInt(5)
	var target uuid.UUID
	proposer := &scriptedProposer{answer: func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		target = in.Items[0].ID
		return addDiscount(promotion, domain.ChangeRequestOperation{
			Type: domain.ChangeRequestOperationUpdateQuantity, TargetItemID: &target, Quantity: &quantity})(in)
	}}
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, "5 bolsas y la promo de arena")
	if err := e.batchChangeHandler(proposer, nil).Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}

	state := e.quoteState(t, quote.quoteID)
	if lines := e.storedLines(t, state.currentID); !lines[0].quantity.Equal(decimal.NewFromInt(2)) {
		t.Errorf("draft quantity = %s, want the whole proposal rolled back", lines[0].quantity)
	}
	run := e.lastRun(t, quote.quoteID)
	if run.Outcome != "NO_MATCH" || run.NoMatchReason == nil || *run.NoMatchReason != "PROMOTION_NOT_EARNED" ||
		run.NoMatchOrigin == nil || *run.NoMatchOrigin != "PIPELINE_EXHAUSTED" {
		t.Errorf("run = %+v, want a promotion-not-earned no-match", run)
	}
}

// A promotion already applied needs nothing: the run is recorded as applied and the total stands.
func TestConversationIntents_AddDiscountAlreadyLiveChangesNothing(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent live promo")
	promotion := e.promotionOn(t, quote, "Cemento 10%", quote.cementID)

	e.runBatch(t, quote, addDiscount(promotion), "¿me hacés la promo?")

	draft := e.quoteState(t, quote.quoteID).currentID
	if live, _ := e.liveDiscounts(t, draft, promotion); live != 1 {
		t.Errorf("live applications = %d, want 1", live)
	}
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(90)) {
		t.Errorf("draft total = %s, want 90", got)
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "APPLIED" {
		t.Errorf("run = %+v, want applied", run)
	}
}

/*
 * Each successful send is the seller's message in the conversation, with the link left as a
 * placeholder so the token is stored nowhere else. It never opens a window, and the panel shows it
 * beside the customer's messages rather than as one of them.
 */
func TestConversationIntents_SellerSendsAreRecordedWithoutTheToken(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Intent seller message")
	var author, channel *uuid.UUID
	var body string
	var batch *uuid.UUID
	var count, windows int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT author_user_id, channel_id, body, batch_id,
		        (SELECT count(*) FROM quote_message WHERE quote_id = $1 AND author_type = 'SELLER'),
		        (SELECT count(*) FROM message_batch WHERE quote_id = $1)
		 FROM quote_message WHERE quote_id = $1 AND author_type = 'SELLER'`, quote.quoteID).
		Scan(&author, &channel, &body, &batch, &count, &windows); err != nil {
		t.Fatalf("read the seller message: %v", err)
	}
	if count != 1 || author == nil || *author != quote.seed.tenant.UserID || channel == nil || batch != nil ||
		windows != 0 {
		t.Errorf("seller messages = %d by %v on %v in %v, windows = %d; want one by the sender, on the send's channel, opening nothing",
			count, author, channel, batch, windows)
	}
	if strings.Contains(body, quote.sentToken) || !strings.Contains(body, domain.QuotePublicURLPlaceholder) {
		t.Errorf("body = %q, want the link placeholder and never the token", body)
	}

	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	detail := func() map[string]json.RawMessage {
		rec := e.do(t, request{method: http.MethodGet,
			path: "/v1/rfqs/" + quote.seed.draft.RFQ.ID.String(), token: admin,
			branch: quote.seed.tenant.BranchID.String()})
		var got map[string]json.RawMessage
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("detail: status = %d: %s", rec.Code, rec.Body)
		}
		return got
	}
	if conversation := detail()["conversation"]; string(conversation) != "null" && conversation != nil {
		t.Errorf("conversation = %s, want none until the customer writes", conversation)
	}

	e.runBatch(t, quote, requiresSeller, "¿tenés en otro color?")
	var conversation struct {
		Pending        []struct{ Author string } `json:"pending"`
		SellerMessages []struct {
			Author string `json:"author"`
			Body   string `json:"body"`
		} `json:"seller_messages"`
		Batches []struct {
			Messages []struct{ Author string } `json:"messages"`
		} `json:"batches"`
	}
	if err := json.Unmarshal(detail()["conversation"], &conversation); err != nil {
		t.Fatal(err)
	}
	if len(conversation.SellerMessages) != 1 || conversation.SellerMessages[0].Author != "SELLER" ||
		len(conversation.Pending) != 0 || len(conversation.Batches) != 1 ||
		len(conversation.Batches[0].Messages) != 1 || conversation.Batches[0].Messages[0].Author != "CLIENT" {
		t.Errorf("conversation = %+v, want the send beside the customer's batch, not pending", conversation)
	}
}
