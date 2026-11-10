//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestQuoteRecommendationEdit_PriceAndReplaceAtTheQuoteBranch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Recommendation editing")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	base := e.seedPricedProduct(t, accountID, branchID, "Base", "100.00", nil)
	economy := e.seedPricedProduct(t, accountID, branchID, "Economy", "80.00", nil)
	replacement := e.seedProduct(t, accountID, "Replacement", "")
	e.stock(t, accountID, branchID, replacement)
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_price WHERE account_id=$1 AND product_id=$2`, accountID, replacement)
	})
	var familyID, subgroupID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT family_id, id FROM product_subgroup
		ORDER BY family_id, sort_order LIMIT 1`).Scan(&familyID, &subgroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE product SET family_id=$2, subgroup_id=$3, unit='bolsa'
		WHERE account_id=$1 AND id=ANY($4::uuid[])`, accountID, familyID, subgroupID,
		[]uuid.UUID{base, economy, replacement}); err != nil {
		t.Fatal(err)
	}
	seeded := e.seedDraftQuote(t, accountID, branchID, admin,
		[]draftLine{{description: "Base", quantity: "1", productID: &base}})
	e.acceptMaterials(t, seeded.quoteID, token, branchID.String())
	var itemID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM quote_item
		WHERE account_id=$1 AND version_id=$2`, accountID, seeded.versionID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	path := "/v1/quotes/" + seeded.quoteID.String() + "/items/" + itemID.String() + "/recommendations/"
	unpriced := e.do(t, request{method: http.MethodPut, path: path + "PREMIUM", token: token,
		branch: branchID.String(), body: map[string]any{"product_id": replacement}})
	var refusal dto.ErrorResponse
	if err := json.Unmarshal(unpriced.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if unpriced.Code != http.StatusUnprocessableEntity || refusal.Code != string(domain.CodeRecommendationPriceUnavailable) {
		t.Fatalf("unpriced = %d %s", unpriced.Code, unpriced.Body)
	}
	set := func(kind string, productID uuid.UUID, wantPrice string) {
		t.Helper()
		rec := e.do(t, request{method: http.MethodPut, path: path + kind, token: token,
			branch: branchID.String(), body: map[string]any{"product_id": productID}})
		var result dto.QuoteItemAlternativeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || result.ProductID == nil || *result.ProductID != productID ||
			result.PriceSnapshot == nil || *result.PriceSnapshot != wantPrice || result.ApprovedBySeller {
			t.Fatalf("set %s = %d %s", kind, rec.Code, rec.Body)
		}
	}
	set("ECONOMY", economy, "80.00")
	for _, step := range []struct{ price, kind string }{{"120.00", "PREMIUM"}, {"70.00", "ECONOMY"}} {
		rec := e.do(t, request{method: http.MethodPost, path: "/v1/products/" + replacement.String() + "/prices",
			token: token, branch: branchID.String(), body: map[string]any{"price": step.price, "currency": "ARS"}})
		if rec.Code != http.StatusCreated {
			t.Fatalf("set branch price = %d %s", rec.Code, rec.Body)
		}
		set(step.kind, replacement, step.price)
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM quote_item_alternative
		WHERE account_id=$1 AND quote_item_id=$2 AND recommendation_type='ECONOMY'`, accountID, itemID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("economy slots = %d, want one replacement", count)
	}
	var baseSnapshot string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT unit_price_snapshot::text FROM quote_item
		WHERE account_id=$1 AND id=$2`, accountID, itemID).Scan(&baseSnapshot); err != nil {
		t.Fatal(err)
	}
	if baseSnapshot != "100.00" {
		t.Fatalf("base snapshot = %s, want 100.00", baseSnapshot)
	}
}

func TestQuoteRecommendationEdit_RefreshesOnlyMutableOptionsAtThePricedBranch(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Live recommendation prices")
	otherBranch := e.seedBranch(t, accountID, "Other price branch")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	base := e.seedPricedProduct(t, accountID, branchID, "Base", "100.00", nil)
	alternative := e.seedPricedProduct(t, accountID, branchID, "Alternative", "80.00", nil)
	for _, productID := range []uuid.UUID{base, alternative} {
		e.stock(t, accountID, otherBranch, productID)
	}
	e.openPricePeriod(t, accountID, otherBranch, base, "100.00", nil)
	e.openPricePeriod(t, accountID, otherBranch, alternative, "80.00", nil)
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_price WHERE account_id=$1 AND product_id=$2`, accountID, alternative)
	})
	var familyID, subgroupID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT family_id,id FROM product_subgroup
		ORDER BY family_id,sort_order LIMIT 1`).Scan(&familyID, &subgroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE product SET family_id=$2,subgroup_id=$3,unit='bolsa'
		WHERE account_id=$1 AND id=ANY($4::uuid[])`, accountID, familyID, subgroupID, []uuid.UUID{base, alternative}); err != nil {
		t.Fatal(err)
	}
	type option struct {
		id     uuid.UUID
		branch uuid.UUID
		frozen bool
	}
	var options []option
	for _, state := range []struct {
		branch uuid.UUID
		frozen bool
	}{
		{branchID, false}, {branchID, true}, {otherBranch, false},
	} {
		seeded := e.seedDraftQuote(t, accountID, state.branch, admin,
			[]draftLine{{description: "Base", quantity: "1", productID: &base}})
		e.acceptMaterials(t, seeded.quoteID, token, state.branch.String())
		var itemID uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM quote_item WHERE account_id=$1 AND version_id=$2`,
			accountID, seeded.versionID).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		rec := e.do(t, request{method: http.MethodPut,
			path:  "/v1/quotes/" + seeded.quoteID.String() + "/items/" + itemID.String() + "/recommendations/ECONOMY",
			token: token, branch: state.branch.String(), body: map[string]any{"product_id": alternative}})
		if rec.Code != http.StatusOK {
			t.Fatalf("set option = %d %s", rec.Code, rec.Body)
		}
		var result dto.QuoteItemAlternativeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_item_alternative SET approved_by_seller=TRUE
			WHERE account_id=$1 AND id=$2`, accountID, result.ID); err != nil {
			t.Fatal(err)
		}
		if state.frozen {
			if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_version SET is_immutable=TRUE,frozen_at=now()
				WHERE account_id=$1 AND id=$2`, accountID, seeded.versionID); err != nil {
				t.Fatal(err)
			}
		}
		options = append(options, option{result.ID, state.branch, state.frozen})
	}
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/products/" + alternative.String() + "/prices",
		token: token, branch: branchID.String(), body: map[string]any{"price": "60.00", "min_price": "50.00", "currency": "ARS"}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("reprice = %d %s", rec.Code, rec.Body)
	}
	for _, option := range options {
		var price string
		var approved bool
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT price_snapshot::text,approved_by_seller
			FROM quote_item_alternative WHERE account_id=$1 AND id=$2`, accountID, option.id).Scan(&price, &approved); err != nil {
			t.Fatal(err)
		}
		changed := option.branch == branchID && !option.frozen
		wantPrice := "80.00"
		if changed {
			wantPrice = "60.00"
		}
		if price != wantPrice || approved == changed {
			t.Errorf("option %+v = %s approved=%v, want %s approved=%v", option, price, approved, wantPrice, !changed)
		}
	}
}
