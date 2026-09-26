//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedClosedBranch adds a branch that is already closed; the account teardown removes it.
func (e *env) seedClosedBranch(t *testing.T, accountID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := e.seedBranch(t, accountID, name)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE branch SET is_active = FALSE WHERE id = $1`, id); err != nil {
		t.Fatalf("close branch: %v", err)
	}
	return id
}

func (e *env) anyFamily(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM product_family ORDER BY sort_order LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("read a product family: %v", err)
	}
	return id
}

// availableAt lists the branches that carry the product, active rows only, in id order.
func (e *env) availableAt(t *testing.T, productID uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT branch_id FROM branch_product WHERE product_id = $1 AND is_active = TRUE
		 ORDER BY branch_id`, productID)
	if err != nil {
		t.Fatalf("read availability: %v", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan availability: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func sortedIDs(ids ...uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

func (e *env) createProductThroughAPI(
	t *testing.T, token string, body map[string]any,
) uuid.UUID {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/products", token: token, body: body})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product = %d, want 201: %s", rec.Code, rec.Body)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode product %s: %v", rec.Body, err)
	}
	return created.ID
}

/*
 * A product typed into the backoffice form is quotable at once: the create makes it available at
 * every active branch and prices it there, the pipeline matches it, and accepting the materials
 * values the line at that price. A closed branch gets neither.
 */
func TestManualProduct_CreatedThroughTheAPIIsMatchedAndPriced(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Manual product")
	secondBranch := e.seedBranch(t, accountID, "Manual product Norte")
	closedBranch := e.seedClosedBranch(t, accountID, "Manual product cerrada")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	productID := e.createProductThroughAPI(t, e.tokenFor(t, admin), map[string]any{
		"canonical_name": "Cemento Portland 50kg", "family_id": e.anyFamily(t),
		"price": "12500.50", "min_price": "11000",
	})

	if got, want := e.availableAt(t, productID), sortedIDs(branchID, secondBranch); !slices.Equal(got, want) {
		t.Fatalf("available at %v, want the two active branches %v (not the closed %v)", got, want,
			closedBranch)
	}
	var priced int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM product_price
		 WHERE product_id = $1 AND valid_to IS NULL AND price = 12500.50 AND min_price = 11000
		   AND currency = 'ARS' AND user_id = $2 AND branch_id = ANY($3)`,
		productID, admin.ID, []uuid.UUID{branchID, secondBranch}).Scan(&priced); err != nil {
		t.Fatalf("count prices: %v", err)
	}
	if priced != 2 {
		t.Fatalf("open price periods = %d, want one at each active branch", priced)
	}

	// The catalog-embedding job's work, done by hand: the product lands on the line's axis.
	e.embedOn(t, productID, 0, 0.98)
	channelID := e.seedIntakeChannel(t, accountID, secondBranch)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, secondBranch)
	unit := "bolsa"
	tenant := domain.Tenant{AccountID: accountID, BranchID: secondBranch, UserID: seller.ID,
		Role: domain.UserRoleSeller}
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "10 bolsas de cemento portland",
		Quantity:             decimal.RequireFromString("10"),
		Unit:                 &unit, Source: domain.QuantitySourceExplicit,
		QuantityRationale: "el cliente pidió 10 bolsas",
	}}}, map[string]int{"10 bolsas de cemento portland": 0}).CreateTextDraft(
		context.Background(), tenant,
		domain.TextRFQDraftInput{ChannelID: channelID, RawText: "10 bolsas de cemento portland"})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want the order drafted", err)
	}
	e.dropDraft(t, draft.RFQ.ID)
	if len(draft.Items) != 1 || draft.Items[0].ProductID == nil ||
		*draft.Items[0].ProductID != productID {
		t.Fatalf("drafted %+v, want the one line matched to the manual product %v", draft.Items,
			productID)
	}

	valued := e.acceptMaterials(t, draft.Quote.ID, e.tokenFor(t, seller), secondBranch.String())
	if len(valued.Items) != 1 || valued.Items[0].UnitPriceSnapshot == nil ||
		*valued.Items[0].UnitPriceSnapshot != "12500.50" {
		t.Fatalf("valued lines = %+v, want the line at 12500.50", valued.Items)
	}
	if floor := valued.Items[0].MinPriceSnapshot; floor == nil || *floor != "11000.00" {
		t.Errorf("min price snapshot = %v, want 11000.00", floor)
	}
}

func TestManualProduct_WithoutAPriceIsAvailableButUnpriced(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Manual product unpriced")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	productID := e.createProductThroughAPI(t, e.tokenFor(t, admin), map[string]any{
		"canonical_name": "Arena gruesa", "family_id": e.anyFamily(t),
	})

	if got := e.availableAt(t, productID); !slices.Equal(got, []uuid.UUID{branchID}) {
		t.Errorf("available at %v, want the account's branch %v", got, branchID)
	}
	var prices int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM product_price WHERE product_id = $1`, productID).Scan(&prices); err != nil {
		t.Fatalf("count prices: %v", err)
	}
	if prices != 0 {
		t.Errorf("price periods = %d, want none without a price", prices)
	}
}

func TestManualProduct_RefusesAFloorWithoutAPrice(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Manual product floor only")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/products",
		token: e.tokenFor(t, admin), body: map[string]any{
			"canonical_name": "Cal hidratada", "family_id": e.anyFamily(t), "min_price": "100",
		}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create with only min_price = %d, want 400: %s", rec.Code, rec.Body)
	}
	var products int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM product WHERE account_id = $1`, accountID).Scan(&products); err != nil {
		t.Fatalf("count products: %v", err)
	}
	if products != 0 {
		t.Errorf("products = %d, want none after a refused create", products)
	}
}

// A new branch starts with the account's active catalog available and nothing priced: prices are
// per branch and arrive with its price list.
func TestBranches_NewBranchStartsWithTheActiveCatalog(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Branch catalog")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	active := e.seedProduct(t, accountID, "Ladrillo hueco 12", "unidad")
	withdrawn := e.seedProduct(t, accountID, "Ladrillo discontinuado", "unidad")
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE product SET is_active = FALSE WHERE id = $1`, withdrawn); err != nil {
		t.Fatalf("withdraw product: %v", err)
	}

	res := e.do(t, request{method: http.MethodPost, path: "/v1/branches",
		token: e.tokenFor(t, admin), body: map[string]any{"name": "Sucursal Oeste"}})
	if res.Code != http.StatusCreated {
		t.Fatalf("create branch = %d, want 201: %s", res.Code, res.Body)
	}
	var branch struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &branch); err != nil {
		t.Fatalf("decode branch: %v", err)
	}

	if got := e.availableAt(t, active); !slices.Equal(got, []uuid.UUID{branch.ID}) {
		t.Errorf("active product available at %v, want the new branch %v", got, branch.ID)
	}
	if got := e.availableAt(t, withdrawn); len(got) != 0 {
		t.Errorf("withdrawn product available at %v, want nowhere", got)
	}
	var prices int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM product_price WHERE branch_id = $1`, branch.ID).Scan(&prices); err != nil {
		t.Fatalf("count prices: %v", err)
	}
	if prices != 0 {
		t.Errorf("price periods at the new branch = %d, want none", prices)
	}
}

// migrationUp reads the Up half of a goose migration, so a data repair can be exercised against
// rows the test controls.
func migrationUp(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	up, _, found := strings.Cut(strings.SplitN(string(raw), "-- +goose Up", 2)[1], "-- +goose Down")
	if !found {
		t.Fatalf("migration %s has no Down section", name)
	}
	return up
}

/*
 * The repair runs inside a transaction that is rolled back, so it never touches rows another test
 * owns. Run twice: the second pass must add nothing.
 */
func TestRepairUnavailableProducts_AddsOnlyProductsNoBranchCarries(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Repair availability")
	secondBranch := e.seedBranch(t, accountID, "Repair availability Norte")
	closedBranch := e.seedClosedBranch(t, accountID, "Repair availability cerrada")
	orphan := e.seedProduct(t, accountID, "Hierro 8mm", "barra")
	decided := e.seedProduct(t, accountID, "Hierro 10mm", "barra")
	e.stock(t, accountID, branchID, decided)
	withdrawn := e.seedProduct(t, accountID, "Hierro 6mm", "barra")
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE product SET is_active = FALSE WHERE id = $1`, withdrawn); err != nil {
		t.Fatalf("withdraw product: %v", err)
	}
	up := migrationUp(t, "00028_repair_unavailable_products.sql")

	tx, err := e.db.AdminTx(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("first run: %v", err)
	}
	tag, err := tx.Exec(ctx, up)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Errorf("second run added %d rows, want none", tag.RowsAffected())
	}

	carried := func(productID uuid.UUID) []uuid.UUID {
		rows, err := tx.Query(ctx,
			`SELECT branch_id FROM branch_product WHERE product_id = $1 ORDER BY branch_id`,
			productID)
		if err != nil {
			t.Fatalf("read availability: %v", err)
		}
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan availability: %v", err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	if got, want := carried(orphan), sortedIDs(branchID, secondBranch); !slices.Equal(got, want) {
		t.Errorf("orphan available at %v, want the active branches %v (not the closed %v)", got,
			want, closedBranch)
	}
	if got := carried(decided); !slices.Equal(got, []uuid.UUID{branchID}) {
		t.Errorf("decided product available at %v, want only its own %v", got, branchID)
	}
	if got := carried(withdrawn); len(got) != 0 {
		t.Errorf("withdrawn product available at %v, want nowhere", got)
	}
}
