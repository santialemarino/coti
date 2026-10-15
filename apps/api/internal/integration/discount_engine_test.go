//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// storedDiscount is one quote_discount read straight back out, with the lines it covers.
type storedDiscount struct {
	id          uuid.UUID
	promotionID *uuid.UUID
	origin      string
	scope       string
	amount      decimal.Decimal
	suppressed  bool
	lines       int
}

func (e *env) storedDiscounts(t *testing.T, versionID uuid.UUID) map[string]storedDiscount {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT qd.id, qd.promotion_id, qd.origin, qd.scope, qd.amount, qd.suppressed_by_seller,
		        COALESCE(p.name, qd.description),
		        (SELECT count(*) FROM quote_discount_item qdi WHERE qdi.quote_discount_id = qd.id)
		 FROM quote_discount qd LEFT JOIN promotion p ON p.id = qd.promotion_id
		 WHERE qd.quote_version_id = $1`, versionID)
	if err != nil {
		t.Fatalf("read the discounts back: %v", err)
	}
	defer rows.Close()
	out := map[string]storedDiscount{}
	for rows.Next() {
		var d storedDiscount
		var name string
		if err := rows.Scan(&d.id, &d.promotionID, &d.origin, &d.scope, &d.amount, &d.suppressed,
			&name, &d.lines); err != nil {
			t.Fatalf("scan a discount: %v", err)
		}
		out[name] = d
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the discounts back: %v", err)
	}
	return out
}

// dropDiscounts removes what the engine and the test wrote for an account. Registered after the
// draft and the products, so it runs before their own cleanups: both are referenced from here.
func (e *env) dropDiscounts(t *testing.T, accountID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM quote_discount_item WHERE account_id = $1`,
			`DELETE FROM quote_discount WHERE account_id = $1`,
			`DELETE FROM promotion_tier WHERE account_id = $1`,
			`DELETE FROM promotion_condition_item WHERE account_id = $1`,
			`DELETE FROM promotion WHERE account_id = $1`,
		} {
			e.mustCleanup(t, stmt, accountID)
		}
	})
}

func (e *env) createPromotion(t *testing.T, token string, body map[string]any) uuid.UUID {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/promotions", token: token, body: body})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create promotion: status = %d, want 201: %s", rec.Code, rec.Body)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return created.ID
}

func (e *env) patchDiscount(t *testing.T, quoteID, discountID uuid.UUID, token, branch string,
	body map[string]any, want int,
) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + quoteID.String() + "/discounts/" + discountID.String(),
		token: token, branch: branch, body: body})
	if rec.Code != want {
		t.Fatalf("patch discount: status = %d, want %d: %s", rec.Code, want, rec.Body)
	}
}

func (e *env) lineID(t *testing.T, versionID uuid.UUID, description string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM quote_item WHERE version_id = $1 AND requested_description = $2`,
		versionID, description).Scan(&id); err != nil {
		t.Fatalf("find line %q: %v", description, err)
	}
	return id
}

func requireTotal(t *testing.T, e *env, versionID uuid.UUID, want string) {
	t.Helper()
	if got := e.storedVersionTotal(t, versionID); !got.Equal(decimal.RequireFromString(want)) {
		t.Fatalf("total = %s, want %s", got, want)
	}
}

// The whole seller-facing loop: Aceptar materiales earns the promotions, the seller suppresses
// one and adds their own, every item change re-sweeps, and a price change in between moves nothing.
func TestDiscountEngine_SweepFollowsTheSeller(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Discount engine sweep")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento Portland 50kg", "1000.00", nil)
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena gruesa", "500.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{
		{description: "cemento", quantity: "10", productID: &cement},
		{description: "arena", quantity: "4", productID: &sand},
	})
	e.dropDiscounts(t, accountID)
	adminToken, sellerToken, branch := e.tokenFor(t, admin), e.tokenFor(t, seller), branchID.String()

	e.createPromotion(t, adminToken, map[string]any{
		"name": "Cemento 10%", "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": "10", "priority": 5,
		"conditions": []map[string]any{{"product_id": cement}},
	})
	e.createPromotion(t, adminToken, map[string]any{
		"name": "Compra grande", "condition_type": "ON_TOTAL", "action_type": "PERCENTAGE",
		"action_value": "5", "min_total": "10000",
	})

	// Gross 10 × 1000 + 4 × 500 = 12000. Cemento: 1000. Net 11000 ≥ 10000, so 5% = 550.
	body := e.acceptMaterials(t, seeded.quoteID, sellerToken, branch)
	if body.Version.Total != "10450.00" {
		t.Fatalf("accepted total = %q, want 10450.00 (12000 − 1000 − 550)", body.Version.Total)
	}
	discounts := e.storedDiscounts(t, seeded.versionID)
	line, total := discounts["Cemento 10%"], discounts["Compra grande"]
	if line.origin != "AUTOMATIC" || line.scope != "ITEM" || line.lines != 1 ||
		!line.amount.Equal(decimal.RequireFromString("1000.00")) {
		t.Fatalf("line application = %+v, want AUTOMATIC ITEM 1000.00 on one line", line)
	}
	if total.origin != "AUTOMATIC" || total.scope != "TOTAL" || total.lines != 0 ||
		!total.amount.Equal(decimal.RequireFromString("550.00")) {
		t.Fatalf("total application = %+v, want AUTOMATIC TOTAL 550.00", total)
	}

	// The seller suppresses the line promotion: the total rule now sees a 12000 net, 5% = 600.
	e.patchDiscount(t, seeded.quoteID, line.id, sellerToken, branch,
		map[string]any{"suppressed_by_seller": true}, http.StatusOK)
	requireTotal(t, e, seeded.versionID, "11400.00")
	if got := e.storedDiscounts(t, seeded.versionID)["Cemento 10%"]; !got.suppressed || got.id != line.id {
		t.Fatalf("after suppressing = %+v, want the same row, suppressed", got)
	}

	// An ad hoc discount on top: 12000 − 600 − 400 = 11000.
	rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/discounts",
		token: sellerToken, branch: branch,
		body: map[string]any{"description": "Cliente frecuente", "action_type": "FIXED_AMOUNT",
			"value": "400", "scope": "TOTAL"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("ad hoc discount: status = %d, want 201: %s", rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "11000.00")

	// Restoring brings the same row back: 12000 − 1000 − 550 − 400 = 10050.
	e.patchDiscount(t, seeded.quoteID, line.id, sellerToken, branch,
		map[string]any{"suppressed_by_seller": false}, http.StatusOK)
	requireTotal(t, e, seeded.versionID, "10050.00")

	// An engine discount is suppressed, never edited.
	e.patchDiscount(t, seeded.quoteID, line.id, sellerToken, branch,
		map[string]any{"value": "50", "action_type": "PERCENTAGE", "scope": "ITEM",
			"item_ids": []string{e.lineID(t, seeded.versionID, "cemento").String()}},
		http.StatusUnprocessableEntity)

	// The corralón doubles the cement price. Re-sweeping the same lines changes nothing.
	e.openPricePeriod(t, accountID, branchID, cement, "2000.00", nil)
	cementLine := e.lineID(t, seeded.versionID, "cemento")
	if rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + cementLine.String(),
		token: sellerToken, branch: branch, body: map[string]any{"quantity": "10"}}); rec.Code != http.StatusOK {
		t.Fatalf("touch the cement line: status = %d: %s", rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "10050.00")
	if got := e.storedDiscounts(t, seeded.versionID)["Cemento 10%"]; got.id != line.id ||
		!got.amount.Equal(decimal.RequireFromString("1000.00")) {
		t.Fatalf("line application after the price change = %+v, want the same 1000.00 row", got)
	}

	// The seller also suppresses the total rule: 12000 − 1000 − 400 = 10600.
	e.patchDiscount(t, seeded.quoteID, total.id, sellerToken, branch,
		map[string]any{"suppressed_by_seller": true}, http.StatusOK)
	requireTotal(t, e, seeded.versionID, "10600.00")

	// Deleting the cement line takes its application with it.
	if rec := e.do(t, request{method: http.MethodDelete,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + cementLine.String(),
		token: sellerToken, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("delete the cement line: status = %d: %s", rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "1600.00") // 2000 − 400
	after := e.storedDiscounts(t, seeded.versionID)
	if _, ok := after["Cemento 10%"]; ok {
		t.Errorf("the line application outlived its line: %+v", after)
	}
	if got := after["Compra grande"]; !got.suppressed {
		t.Errorf("the suppressed total application = %+v, want it kept, suppressed", got)
	}

	// Restoring it now, with a 2000 net under its threshold, is refused and nothing moves.
	rec = e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/discounts/" + total.id.String(),
		token: sellerToken, branch: branch, body: map[string]any{"suppressed_by_seller": false}})
	if rec.Code != http.StatusConflict || !json.Valid(rec.Body.Bytes()) ||
		!containsCode(rec.Body.Bytes(), "DISCOUNT_NOT_APPLICABLE") {
		t.Fatalf("restore an unearned application: status = %d, want 409 DISCOUNT_NOT_APPLICABLE: %s",
			rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "1600.00")
	if got := e.storedDiscounts(t, seeded.versionID)["Compra grande"]; !got.suppressed {
		t.Errorf("after the refused restore = %+v, want it still suppressed", got)
	}
}

func containsCode(body []byte, code string) bool {
	var decoded struct {
		Code string `json:"code"`
	}
	return json.Unmarshal(body, &decoded) == nil && decoded.Code == code
}

func TestDiscountEngine_TheFloorBindsTheSweep(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Discount engine floor")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	floor := "950.00"
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento", "1000.00", &floor)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "cemento", quantity: "10", productID: &cement}})
	e.dropDiscounts(t, accountID)

	e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
		"name": "Liquidación", "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
		"action_value": "20", "conditions": []map[string]any{{"product_id": cement}},
	})
	// 20% of 10000 would be 2000; the frozen floor 950 × 10 = 9500 leaves 500.
	body := e.acceptMaterials(t, seeded.quoteID, e.tokenFor(t, seller), branchID.String())
	if body.Version.Total != "9500.00" {
		t.Fatalf("total = %q, want 9500.00 — the floor", body.Version.Total)
	}
}

// Account scoping of promotion reads, proved on the owner pool where row level security is off,
// so the repository's own predicate is the only thing left to refuse another account's rule.
func TestDiscountEngine_PromotionsStayInTheirAccount(t *testing.T) {
	e := newEnv(t)
	accountA, branchA := e.seedAccount(t, "Discount engine A")
	accountB, _ := e.seedAccount(t, "Discount engine B")
	adminA := e.seedUser(t, accountA, domain.UserRoleAdmin)
	sellerA := e.seedUser(t, accountA, domain.UserRoleSeller)
	adminB := e.seedUser(t, accountB, domain.UserRoleAdmin)
	e.assignBranch(t, accountA, sellerA, branchA)
	cement := e.seedPricedProduct(t, accountA, branchA, "Cemento", "1000.00", nil)
	seeded := e.seedDraftQuote(t, accountA, branchA, sellerA,
		[]draftLine{{description: "cemento", quantity: "10", productID: &cement}})
	e.dropDiscounts(t, accountA)
	e.dropDiscounts(t, accountB)

	foreign := e.createPromotion(t, e.tokenFor(t, adminB), map[string]any{
		"name": "Todo a mitad", "condition_type": "ON_TOTAL", "action_type": "PERCENTAGE",
		"action_value": "50",
	})

	inForce, err := repository.NewPromotionRepository().ListInForce(context.Background(),
		e.db.CrossAccount(), accountA, branchA, time.Now())
	if err != nil {
		t.Fatalf("ListInForce() = %v", err)
	}
	if len(inForce) != 0 {
		t.Fatalf("account A reads %d promotions on the owner pool, want 0: %+v", len(inForce), inForce)
	}
	byID, err := repository.NewPromotionRepository().GetByID(context.Background(),
		e.db.CrossAccount(), accountA, foreign)
	if err == nil {
		t.Fatalf("account A read account B's promotion on the owner pool: %+v", byID)
	}

	// Account B's rule never reaches account A's quote: 10 × 1000 with nothing off.
	body := e.acceptMaterials(t, seeded.quoteID, e.tokenFor(t, sellerA), branchA.String())
	if body.Version.Total != "10000.00" {
		t.Fatalf("total = %q, want 10000.00", body.Version.Total)
	}

	tokenA := e.tokenFor(t, adminA)
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/promotions/" + foreign.String(),
		token: tokenA}); rec.Code != http.StatusNotFound {
		t.Errorf("get another account's promotion: status = %d, want 404", rec.Code)
	}
	if rec := e.do(t, request{method: http.MethodPut, path: "/v1/promotions/" + foreign.String(),
		token: tokenA, body: map[string]any{"name": "x", "condition_type": "ON_TOTAL",
			"action_type": "PERCENTAGE", "action_value": "1"}}); rec.Code != http.StatusNotFound {
		t.Errorf("replace another account's promotion: status = %d, want 404", rec.Code)
	}
	// A rule naming another account's product is refused, though the foreign key would take it.
	if rec := e.do(t, request{method: http.MethodPost, path: "/v1/promotions", token: e.tokenFor(t, adminB),
		body: map[string]any{"name": "Ajeno", "condition_type": "PER_ITEM", "action_type": "PERCENTAGE",
			"action_value": "10", "conditions": []map[string]any{{"product_id": cement}}}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("promotion on another account's product: status = %d, want 422: %s", rec.Code, rec.Body)
	}
	// Administering promotions is the admin's.
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/promotions",
		token: e.tokenFor(t, sellerA)}); rec.Code != http.StatusForbidden {
		t.Errorf("a seller lists promotions: status = %d, want 403", rec.Code)
	}
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/promotions", token: tokenA})
	var list struct {
		Items []struct{ ID uuid.UUID } `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, %v: %s", rec.Code, err, rec.Body)
	}
	if len(list.Items) != 0 {
		t.Errorf("account A lists %d promotions, want 0", len(list.Items))
	}
}

func TestDiscountEngine_ReplaceDeactivatesAndReactivates(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Discount engine replace")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento", "1000.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "cemento", quantity: "60", productID: &cement}})
	e.dropDiscounts(t, accountID)
	adminToken := e.tokenFor(t, admin)

	rule := map[string]any{
		"name": "Cemento por cantidad", "condition_type": "QUANTITY_TIERED",
		"action_type": "PERCENTAGE", "is_active": false,
		"conditions": []map[string]any{{"product_id": cement, "min_quantity": "10"}},
		"tiers": []map[string]any{{"from_quantity": "10", "to_quantity": "49", "value": "10"},
			{"from_quantity": "50", "value": "15"}},
	}
	id := e.createPromotion(t, adminToken, rule)
	body := e.acceptMaterials(t, seeded.quoteID, e.tokenFor(t, seller), branchID.String())
	if body.Version.Total != "60000.00" {
		t.Fatalf("total with the rule inactive = %q, want 60000.00", body.Version.Total)
	}

	rule["is_active"] = true
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/promotions/" + id.String(),
		token: adminToken, body: rule})
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivate: status = %d: %s", rec.Code, rec.Body)
	}
	var replaced struct {
		IsActive bool `json:"is_active"`
		Tiers    []struct {
			FromQuantity string  `json:"from_quantity"`
			ToQuantity   *string `json:"to_quantity"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &replaced); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if !replaced.IsActive || len(replaced.Tiers) != 2 || replaced.Tiers[1].ToQuantity != nil {
		t.Fatalf("replaced = %+v, want active with an open-ended second tier", replaced)
	}

	// The next item change sweeps the reactivated rule in: 60 bags fall in the 15% tier.
	line := e.lineID(t, seeded.versionID, "cemento")
	if rec := e.do(t, request{method: http.MethodPatch,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + line.String(),
		token: e.tokenFor(t, seller), branch: branchID.String(),
		body: map[string]any{"quantity": "60"}}); rec.Code != http.StatusOK {
		t.Fatalf("touch the line: status = %d: %s", rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "51000.00") // 60000 − 60000 × 15%
}

// A seller's line discount goes with the last line it covered; one still covering lines shrinks to
// what they carry, so the version total and the invoice's split of it agree.
func TestDiscountEngine_DeletingALineSettlesItsDiscounts(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Discount engine line delete")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento", "1000.00", nil)
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena", "500.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{
		{description: "cemento", quantity: "10", productID: &cement},
		{description: "arena", quantity: "4", productID: &sand},
	})
	e.dropDiscounts(t, accountID)
	token, branch := e.tokenFor(t, seller), branchID.String()
	e.acceptMaterials(t, seeded.quoteID, token, branch) // 10000 + 2000
	cementLine, sandLine := e.lineID(t, seeded.versionID, "cemento"), e.lineID(t, seeded.versionID, "arena")

	addDiscount := func(name, value, scope string, items ...uuid.UUID) {
		t.Helper()
		ids := make([]string, 0, len(items))
		for _, id := range items {
			ids = append(ids, id.String())
		}
		rec := e.do(t, request{method: http.MethodPost,
			path:  "/v1/quotes/" + seeded.quoteID.String() + "/discounts",
			token: token, branch: branch,
			body: map[string]any{"description": name, "action_type": "FIXED_AMOUNT", "value": value,
				"scope": scope, "item_ids": ids}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("add %s: status = %d: %s", name, rec.Code, rec.Body)
		}
	}
	deleteLine := func(id uuid.UUID) {
		t.Helper()
		if rec := e.do(t, request{method: http.MethodDelete,
			path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + id.String(),
			token: token, branch: branch}); rec.Code != http.StatusOK {
			t.Fatalf("delete line: status = %d: %s", rec.Code, rec.Body)
		}
	}

	addDiscount("Sobre el cemento", "1500", "ITEM", cementLine)
	addDiscount("Por el combo", "2400", "ITEM_SET", cementLine, sandLine)
	requireTotal(t, e, seeded.versionID, "8100.00") // 12000 − 1500 − 2400

	deleteLine(cementLine)
	// Only the sand line (2000) is left. The cement-only discount went with its line; the combo
	// discount stays on the sand line but cannot take more than its 2000: 2000 − 2000 = 0.
	requireTotal(t, e, seeded.versionID, "0.00")
	left := e.storedDiscounts(t, seeded.versionID)
	if _, ok := left["Sobre el cemento"]; ok {
		t.Errorf("a discount outlived the only line it covered: %+v", left)
	}
	combo, ok := left["Por el combo"]
	if !ok || combo.lines != 1 || !combo.amount.Equal(decimal.RequireFromString("2000.00")) {
		t.Fatalf("combo discount = %+v, want it on the sand line at 2000.00", combo)
	}

	deleteLine(sandLine)
	requireTotal(t, e, seeded.versionID, "0.00")
	if rest := e.storedDiscounts(t, seeded.versionID); len(rest) != 0 {
		t.Errorf("discounts left on a version with no lines: %+v", rest)
	}
}

// The exact report: a seller's 1500 off the cement, the cement deleted, the sand alone must total 2000.
func TestDiscountEngine_ASellersLineDiscountGoesWithItsLine(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Discount engine orphan")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	cement := e.seedPricedProduct(t, accountID, branchID, "Cemento", "1000.00", nil)
	sand := e.seedPricedProduct(t, accountID, branchID, "Arena", "500.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{
		{description: "cemento", quantity: "10", productID: &cement},
		{description: "arena", quantity: "4", productID: &sand},
	})
	e.dropDiscounts(t, accountID)
	token, branch := e.tokenFor(t, seller), branchID.String()
	e.acceptMaterials(t, seeded.quoteID, token, branch)
	cementLine := e.lineID(t, seeded.versionID, "cemento")
	if rec := e.do(t, request{method: http.MethodPost,
		path: "/v1/quotes/" + seeded.quoteID.String() + "/discounts", token: token, branch: branch,
		body: map[string]any{"description": "Sobre el cemento", "action_type": "FIXED_AMOUNT",
			"value": "1500", "scope": "ITEM", "item_ids": []string{cementLine.String()}}}); rec.Code != http.StatusCreated {
		t.Fatalf("add discount: status = %d: %s", rec.Code, rec.Body)
	}
	if rec := e.do(t, request{method: http.MethodDelete,
		path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + cementLine.String(),
		token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("delete line: status = %d: %s", rec.Code, rec.Body)
	}
	requireTotal(t, e, seeded.versionID, "2000.00")
	if rest := e.storedDiscounts(t, seeded.versionID); len(rest) != 0 {
		t.Errorf("orphan discount rows: %+v", rest)
	}
}
