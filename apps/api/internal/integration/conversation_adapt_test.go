//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// thresholdPromotion is a PER_ITEM percentage on the cement that needs minimum bags on the line.
func (e *env) thresholdPromotion(t *testing.T, quote handlerQuote, name, minimum, percent string,
) uuid.UUID {
	t.Helper()
	admin := e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin)
	return e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
		"name": name, "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": percent, "priority": 5,
		"conditions": []map[string]any{{"product_id": quote.cementID, "min_quantity": minimum}},
	})
}

type originRow struct {
	promotionID *uuid.UUID
	origin      domain.DiscountOrigin
	amount      decimal.Decimal
	suppressed  bool
}

func (e *env) originRows(t *testing.T, versionID uuid.UUID) []originRow {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT promotion_id, origin, amount, suppressed_by_seller FROM quote_discount
		 WHERE quote_version_id = $1 ORDER BY created_at, id`, versionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var discounts []originRow
	for rows.Next() {
		var d originRow
		if err := rows.Scan(&d.promotionID, &d.origin, &d.amount, &d.suppressed); err != nil {
			t.Fatal(err)
		}
		discounts = append(discounts, d)
	}
	return discounts
}

type recordedAdaptation struct {
	Adaptation *struct {
		Lines []struct {
			FromQuantity  string    `json:"from_quantity"`
			ToQuantity    string    `json:"to_quantity"`
			PromotionID   uuid.UUID `json:"promotion_id"`
			PromotionName string    `json:"promotion_name"`
		} `json:"lines"`
		TotalBefore string `json:"total_before"`
		TotalAfter  string `json:"total_after"`
		TotalRaised bool   `json:"total_raised"`
	} `json:"adaptation"`
	PreferredStrategy *string `json:"preferred_strategy"`
}

func (e *env) lastAdaptation(t *testing.T, quoteID uuid.UUID) recordedAdaptation {
	t.Helper()
	var raw []byte
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT d.ai_proposal FROM handler_decision d JOIN quote_version v ON v.id = d.quote_version_id
		 WHERE v.quote_id = $1 ORDER BY d.created_at DESC, d.id DESC LIMIT 1`, quoteID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var recorded recordedAdaptation
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	return recorded
}

func (e *env) preferLowerPrice(t *testing.T, accountID uuid.UUID, strategy domain.HandlerStrategy) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE account SET preferred_lower_price_strategy = $2 WHERE id = $1`, accountID,
		string(strategy)); err != nil {
		t.Fatal(err)
	}
}

func preferring(strategy domain.HandlerStrategy,
) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		proposal, err := intent(domain.ChangeRequestIntentLowerTotalPrice)(in)
		proposal.LowerPriceStrategy = &strategy
		return proposal, err
	}
}

/*
 * «Está caro» with no cheaper alternative: the default order tries cheaper alternatives first,
 * comes up empty, and the adaptation takes the cement from 2 to 3 bags, where 40% off earns:
 * 150 − 60 = 90, below the 100 it stood at. The promotion lands as AI_ADAPTATION.
 */
func TestConversationAdapt_LowerTotalFallsBackToReachingADiscount(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt fallback")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "40")

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "está caro")

	draft := e.quoteState(t, quote.quoteID).currentID
	if lines := e.storedLines(t, draft); len(lines) != 1 || !lines[0].quantity.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("draft lines = %+v, want the cement raised to 3", lines)
	}
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(90)) {
		t.Errorf("draft total = %s, want 150 - 40%% = 90", got)
	}
	if discounts := e.originRows(t, draft); len(discounts) != 1 || discounts[0].promotionID == nil ||
		*discounts[0].promotionID != promotion || discounts[0].origin != domain.DiscountOriginAIAdaptation ||
		!discounts[0].amount.Equal(decimal.NewFromInt(60)) {
		t.Errorf("discounts = %+v, want the promotion earned as AI_ADAPTATION for 60", discounts)
	}
	run := e.lastRun(t, quote.quoteID)
	if run.Outcome != "APPLIED" ||
		!slices.Equal(run.StrategiesTried, []string{"FIND_CHEAPER_ALTERNATIVES", "ADAPT_TO_REACH_DISCOUNT"}) ||
		len(run.Operations) != 1 || run.Operations[0].Type != domain.ChangeRequestOperationUpdateQuantity ||
		!run.Operations[0].Quantity.Equal(decimal.NewFromInt(3)) {
		t.Errorf("run = %+v, want an applied raise after both strategies, in run order", run)
	}
	adapted := e.lastAdaptation(t, quote.quoteID).Adaptation
	if adapted == nil || len(adapted.Lines) != 1 || adapted.Lines[0].FromQuantity != "2.00" ||
		adapted.Lines[0].ToQuantity != "3.00" || adapted.Lines[0].PromotionID != promotion ||
		adapted.TotalBefore != "100.00" || adapted.TotalAfter != "90.00" || adapted.TotalRaised {
		t.Errorf("adaptation = %+v, want 2 → 3 for the promotion, 100 → 90", adapted)
	}
}

/*
 * The account runs the adaptation first, so it wins over a cheaper cement that would also lower the
 * total, and the alternatives never run. A customer asking for cheaper brands overrides it: three
 * bags at 20 end at 60, below the adapted 90.
 */
func TestConversationAdapt_TheStrategyOrderIsCustomerThenAccount(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt order")
	e.thresholdPromotion(t, quote, "Cemento x3", "3", "40")
	cheaper := e.alternativeOf(t, quote, quote.cementID, "Cemento económico", "20",
		domain.ProductAlternativeEconomy)
	e.preferLowerPrice(t, quote.seed.tenant.AccountID, domain.HandlerStrategyAdaptToReachDiscount)

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice), "está caro")

	draft := e.quoteState(t, quote.quoteID).currentID
	if products := e.draftProducts(t, draft); len(products) != 1 || products[0] != quote.cementID {
		t.Fatalf("draft products = %v, want the cement kept by the adaptation", products)
	}
	if run := e.lastRun(t, quote.quoteID); !slices.Equal(run.StrategiesTried, []string{"ADAPT_TO_REACH_DISCOUNT"}) {
		t.Errorf("strategies = %v, want only the account's first choice", run.StrategiesTried)
	}

	e.runBatch(t, quote, preferring(domain.HandlerStrategyFindCheaperAlternatives), "dame marcas más baratas")

	if products := e.draftProducts(t, draft); len(products) != 1 || products[0] != cheaper {
		t.Fatalf("draft products = %v, want the cheaper cement the customer asked for", products)
	}
	run := e.lastRun(t, quote.quoteID)
	if !slices.Equal(run.StrategiesTried, []string{"FIND_CHEAPER_ALTERNATIVES"}) {
		t.Errorf("strategies = %v, want the customer's choice first and alone", run.StrategiesTried)
	}
	if preferred := e.lastAdaptation(t, quote.quoteID).PreferredStrategy; preferred == nil ||
		*preferred != "FIND_CHEAPER_ALTERNATIVES" {
		t.Errorf("preferred = %v, want the customer's wording recorded", preferred)
	}
}

/*
 * Asked for by name, a promotion the quote does not reach is reached: the cement goes from 2 to 3
 * bags and the total rises from 100 to 150 − 15 = 135. The seller reads it on the detail screen.
 */
func TestConversationAdapt_AnAskedPromotionIsReachedEvenIfTheTotalRises(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt asked promo")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")

	e.runBatch(t, quote, addDiscount(promotion), "aplicame la promo del cemento")

	draft := e.quoteState(t, quote.quoteID).currentID
	if lines := e.storedLines(t, draft); !lines[0].quantity.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("draft quantity = %s, want 3", lines[0].quantity)
	}
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.NewFromInt(135)) {
		t.Errorf("draft total = %s, want 135", got)
	}
	if discounts := e.originRows(t, draft); len(discounts) != 1 ||
		discounts[0].origin != domain.DiscountOriginAIAdaptation {
		t.Errorf("discounts = %+v, want the promotion as AI_ADAPTATION", discounts)
	}
	run := e.lastRun(t, quote.quoteID)
	if run.Outcome != "APPLIED" || !slices.Equal(run.StrategiesTried, []string{"ADAPT_TO_REACH_DISCOUNT"}) ||
		len(run.Operations) != 2 || run.Operations[1].Type != domain.ChangeRequestOperationUpdateQuantity {
		t.Errorf("run = %+v, want the promotion and the raise", run)
	}

	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + quote.seed.draft.RFQ.ID.String(),
		token: admin, branch: quote.seed.tenant.BranchID.String()})
	var detail struct {
		ChangesRequested *struct {
			Handler *struct {
				StrategiesTried []string `json:"strategies_tried"`
				Adaptation      *struct {
					TotalRaised bool `json:"total_raised"`
					Lines       []struct {
						ToQuantity    string `json:"to_quantity"`
						PromotionName string `json:"promotion_name"`
					} `json:"lines"`
				} `json:"adaptation"`
			} `json:"handler"`
			Discounts []struct {
				Change    string `json:"change"`
				Requested *struct {
					Origin string `json:"origin"`
				} `json:"requested"`
			} `json:"discounts"`
		} `json:"changes_requested"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &detail) != nil ||
		detail.ChangesRequested == nil || detail.ChangesRequested.Handler == nil {
		t.Fatalf("detail: status = %d: %s", rec.Code, rec.Body)
	}
	handler := detail.ChangesRequested.Handler
	if handler.Adaptation == nil || !handler.Adaptation.TotalRaised || len(handler.Adaptation.Lines) != 1 ||
		handler.Adaptation.Lines[0].ToQuantity != "3.00" ||
		handler.Adaptation.Lines[0].PromotionName != "Cemento x3" ||
		!slices.Equal(handler.StrategiesTried, []string{"ADAPT_TO_REACH_DISCOUNT"}) {
		t.Errorf("handler = %+v, want the raise, its promotion and the higher total", handler)
	}
	if discounts := detail.ChangesRequested.Discounts; len(discounts) != 1 || discounts[0].Change != "ADDED" ||
		discounts[0].Requested.Origin != "AI_ADAPTATION" {
		t.Errorf("diff discounts = %+v, want the promotion added as AI_ADAPTATION", discounts)
	}
}

/*
 * Only what the raise earned is the AI's. At 2 bags the cement already earns a 5% that outranks the
 * 10% from 3; at 3 it keeps that 5% and the quote reaches a 50% over 140:
 * 150 − 7.50 = 142.50, halved to 71.25, below 95. The 5% stays AUTOMATIC.
 */
func TestConversationAdapt_APromotionAlreadyEarnedStaysAutomatic(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt already earned")
	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	earned := e.createPromotion(t, admin, map[string]any{
		"name": "Cemento 5%", "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": "5", "priority": 9, "conditions": []map[string]any{{"product_id": quote.cementID}},
	})
	e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	e.createPromotion(t, admin, map[string]any{
		"name": "Compra grande", "condition_type": "ON_TOTAL", "action_type": "PERCENTAGE",
		"action_value": "50", "priority": 1, "min_total": "140",
	})

	e.runBatch(t, quote, preferring(domain.HandlerStrategyAdaptToReachDiscount), "prefiero llevar más")

	draft := e.quoteState(t, quote.quoteID).currentID
	if got := e.storedVersionTotal(t, draft); !got.Equal(decimal.RequireFromString("71.25")) {
		t.Fatalf("draft total = %s, want 71.25", got)
	}
	for _, discount := range e.originRows(t, draft) {
		if discount.promotionID != nil && *discount.promotionID == earned &&
			discount.origin != domain.DiscountOriginAutomatic {
			t.Errorf("the 5%% the line already earned = %s, want AUTOMATIC", discount.origin)
		}
	}
}

// A goal tied to one line adapts that line only: the cement near its threshold is left alone when
// the customer named the sand, so both strategies come up empty.
func TestConversationAdapt_ActsOnTheNamedLinesOnly(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt named lines")
	accountID, branchID := quote.seed.tenant.AccountID, quote.seed.tenant.BranchID
	e.thresholdPromotion(t, quote, "Cemento x3", "3", "40")
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena gruesa", "30", nil)
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM quote_item WHERE product_id = $1`, sand) })
	e.runBatch(t, quote, requiresSeller, "hola")
	draft := e.quoteState(t, quote.quoteID).currentID
	var sandLine uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description, quantity,
		                         unit_price_snapshot, subtotal)
		 VALUES ($1, $2, $3, 'arena', 3, 30, 90) RETURNING id`, accountID, draft, sand).
		Scan(&sandLine); err != nil {
		t.Fatal(err)
	}

	e.runBatch(t, quote, intent(domain.ChangeRequestIntentLowerTotalPrice, sandLine), "la arena está cara")

	for _, line := range e.storedLines(t, draft) {
		if line.description != "arena" && !line.quantity.Equal(decimal.NewFromInt(2)) {
			t.Errorf("cement quantity = %s, want 2: the customer named the sand", line.quantity)
		}
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "NO_MATCH" || run.NoMatchReason == nil ||
		*run.NoMatchReason != "PIPELINE_EXHAUSTED" {
		t.Errorf("run = %+v, want a pipeline-exhausted no-match", run)
	}
}

// A quantity the customer set in the same message is theirs: the promotion is not reached by
// overriding it, so the request is left to the seller.
func TestConversationAdapt_AnAskedPromotionKeepsTheQuantityTheCustomerSet(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt keeps customer quantity")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	quantity := decimal.NewFromInt(2)
	proposer := func(in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		target := in.Items[0].ID
		return addDiscount(promotion, domain.ChangeRequestOperation{
			Type: domain.ChangeRequestOperationUpdateQuantity, TargetItemID: &target, Quantity: &quantity})(in)
	}

	e.runBatch(t, quote, proposer, "2 bolsas y la promo")

	if lines := e.storedLines(t, e.quoteState(t, quote.quoteID).currentID); !lines[0].quantity.Equal(quantity) {
		t.Errorf("draft quantity = %s, want the 2 the customer asked for", lines[0].quantity)
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "NO_MATCH" || run.NoMatchReason == nil ||
		*run.NoMatchReason != "PROMOTION_NOT_EARNED" {
		t.Errorf("run = %+v, want a promotion-not-earned no-match", run)
	}
}

// Asked for again, a reached promotion the seller suppressed is restored rather than raised for,
// and keeps its origin.
func TestConversationAdapt_AnAskedSuppressedAdaptationIsRestored(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt restore asked")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	e.runBatch(t, quote, addDiscount(promotion), "la promo del cemento")
	draft := e.quoteState(t, quote.quoteID).currentID
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_discount SET suppressed_by_seller = TRUE WHERE quote_version_id = $1`, draft); err != nil {
		t.Fatal(err)
	}

	e.runBatch(t, quote, addDiscount(promotion), "¿y la promo?")

	if discounts := e.originRows(t, draft); len(discounts) != 1 || discounts[0].suppressed ||
		discounts[0].origin != domain.DiscountOriginAIAdaptation {
		t.Errorf("discounts = %+v, want the AI_ADAPTATION row live again", discounts)
	}
	if lines := e.storedLines(t, draft); !lines[0].quantity.Equal(decimal.NewFromInt(3)) {
		t.Errorf("draft quantity = %s, want 3, not raised again", lines[0].quantity)
	}
}

// A suppressed AI_ADAPTATION the draft no longer earns cannot be restored, like any promotion.
func TestConversationAdapt_RestoringAnUnearnedAdaptationIsRefused(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt restore refused")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	e.runBatch(t, quote, addDiscount(promotion), "la promo del cemento")
	draft := e.quoteState(t, quote.quoteID).currentID
	var discountID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`UPDATE quote_discount SET suppressed_by_seller = TRUE WHERE quote_version_id = $1
		 RETURNING id`, draft).Scan(&discountID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_item SET quantity = 2, subtotal = 100 WHERE version_id = $1`, draft); err != nil {
		t.Fatal(err)
	}

	admin := e.tokenFor(t, e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin))
	rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + quote.quoteID.String() + "/discounts/" + discountID.String(),
		token: admin, branch: quote.seed.tenant.BranchID.String(),
		body: map[string]any{"suppressed_by_seller": false}})

	if rec.Code != http.StatusConflict {
		t.Fatalf("restore: status = %d, want 409: %s", rec.Code, rec.Body)
	}
}

// An asked promotion on the whole quote cannot be reached by raising a line: it stays a no-match.
func TestConversationAdapt_AnAskedTotalPromotionIsNotAdapted(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt total promo")
	admin := e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin)
	promotion := e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
		"name": "Compra grande", "condition_type": "ON_TOTAL", "action_type": "PERCENTAGE",
		"action_value": "5", "priority": 5, "min_total": "1000",
	})

	e.runBatch(t, quote, addDiscount(promotion), "¿me hacés lo de compra grande?")

	draft := e.quoteState(t, quote.quoteID).currentID
	if lines := e.storedLines(t, draft); !lines[0].quantity.Equal(decimal.NewFromInt(2)) {
		t.Errorf("draft quantity = %s, want 2 untouched", lines[0].quantity)
	}
	if run := e.lastRun(t, quote.quoteID); run.Outcome != "NO_MATCH" || run.NoMatchReason == nil ||
		*run.NoMatchReason != "PROMOTION_NOT_EARNED" {
		t.Errorf("run = %+v, want a promotion-not-earned no-match", run)
	}
}

/*
 * An AI_ADAPTATION row is the engine's: a later sweep that still earns it keeps the origin, one that
 * no longer does drops it, and in a draft cloned for the next round it is a plain promotion.
 */
func TestConversationAdapt_TheAdaptationOriginFollowsTheSweepAndTheClone(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt lifecycle")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	e.runBatch(t, quote, addDiscount(promotion), "la promo del cemento")
	draft := e.quoteState(t, quote.quoteID).currentID

	e.runBatch(t, quote, requantify(4), "mejor 4")
	if discounts := e.originRows(t, draft); len(discounts) != 1 ||
		discounts[0].origin != domain.DiscountOriginAIAdaptation || !discounts[0].amount.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("after 4 bags discounts = %+v, want the AI_ADAPTATION row re-earned for 20", discounts)
	}

	// Sent as it stands: the next customer message opens a fresh draft from it.
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_version SET is_immutable = TRUE, frozen_at = now() WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	e.runBatch(t, quote, requiresSeller, "gracias")
	next := e.quoteState(t, quote.quoteID).currentID
	if next == draft {
		t.Fatal("no fresh draft was opened")
	}
	if discounts := e.originRows(t, next); len(discounts) != 1 ||
		discounts[0].origin != domain.DiscountOriginAutomatic {
		t.Errorf("cloned discounts = %+v, want the promotion as plain AUTOMATIC", discounts)
	}

	e.runBatch(t, quote, requantify(2), "volvé a 2")
	if discounts := e.originRows(t, next); len(discounts) != 0 {
		t.Errorf("discounts at 2 bags = %+v, want the promotion dropped", discounts)
	}
}

// A promotion reached by the AI that the seller suppressed stays suppressed in the next draft, as
// a promotion the engine owns.
func TestConversationAdapt_ASuppressedAdaptationClonesAsASuppressedPromotion(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "Adapt suppressed clone")
	promotion := e.thresholdPromotion(t, quote, "Cemento x3", "3", "10")
	e.runBatch(t, quote, addDiscount(promotion), "la promo del cemento")
	draft := e.quoteState(t, quote.quoteID).currentID
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_discount SET suppressed_by_seller = TRUE WHERE quote_version_id = $1`, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_version SET is_immutable = TRUE, frozen_at = now() WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}

	e.runBatch(t, quote, requiresSeller, "¿y?")

	next := e.quoteState(t, quote.quoteID).currentID
	if discounts := e.originRows(t, next); len(discounts) != 1 ||
		discounts[0].origin != domain.DiscountOriginAutomatic || !discounts[0].suppressed {
		t.Errorf("cloned discounts = %+v, want one suppressed AUTOMATIC", discounts)
	}
}

// The account's first strategy is an admin setting: it is read back, kept when omitted, and only a
// lower-price strategy is accepted.
func TestAccount_UpdateSetsTheLowerPriceStrategy(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón estrategia")
	token := e.tokenFor(t, e.seedUser(t, accountID, domain.UserRoleAdmin))
	strategyOf := func(rec interface{ Bytes() []byte }) string {
		var body struct {
			Strategy string `json:"preferred_lower_price_strategy"`
		}
		if err := json.Unmarshal(rec.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Strategy
	}

	get := e.do(t, request{method: http.MethodGet, path: "/v1/account", token: token})
	if got := strategyOf(get.Body); got != "FIND_CHEAPER_ALTERNATIVES" {
		t.Errorf("default = %q, want FIND_CHEAPER_ALTERNATIVES", got)
	}
	set := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Corralón estrategia", map[string]any{
			"preferred_lower_price_strategy": "ADAPT_TO_REACH_DISCOUNT"})})
	if set.Code != http.StatusOK || strategyOf(set.Body) != "ADAPT_TO_REACH_DISCOUNT" {
		t.Fatalf("set: status = %d: %s", set.Code, set.Body)
	}
	kept := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Corralón estrategia", nil)})
	if kept.Code != http.StatusOK || strategyOf(kept.Body) != "ADAPT_TO_REACH_DISCOUNT" {
		t.Errorf("omitted: status = %d: %s, want the stored strategy kept", kept.Code, kept.Body)
	}
	refused := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Corralón estrategia", map[string]any{
			"preferred_lower_price_strategy": "IMPROVE_QUALITY"})})
	if refused.Code != http.StatusBadRequest && refused.Code != http.StatusUnprocessableEntity {
		t.Errorf("IMPROVE_QUALITY: status = %d, want it refused", refused.Code)
	}
}
