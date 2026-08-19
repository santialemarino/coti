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
)

// draftLine is one line to seed onto a draft version: a description, a quantity, and the product
// it matched, if any.
type draftLine struct {
	description string
	quantity    string
	productID   *uuid.UUID
}

// draft is a seeded DRAFT quote — the state the RFQ pipeline leaves behind, with no prices on it.
type draft struct {
	rfqID     uuid.UUID
	quoteID   uuid.UUID
	versionID uuid.UUID
}

// assignBranch lets a seller select the branch. Only an admin reaches a branch they are not
// assigned to, and accepting materials is the seller's action. seedAccount takes the row away.
func (e *env) assignBranch(
	t *testing.T, accountID uuid.UUID, user domain.AppUser, branchID uuid.UUID,
) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`,
		accountID, user.ID, branchID); err != nil {
		t.Fatalf("assign the seller to the branch: %v", err)
	}
}

// seedPricedProduct puts a product in the catalog, stocks it at the branch, and opens a price
// period for it. A nil minPrice is the common case: most accounts set no discount floor.
func (e *env) seedPricedProduct(
	t *testing.T, accountID, branchID uuid.UUID, name, price string, minPrice *string,
) uuid.UUID {
	t.Helper()
	productID := e.seedProduct(t, accountID, name, name)
	e.stock(t, accountID, branchID, productID)
	e.openPricePeriod(t, accountID, branchID, productID, price, minPrice)
	return productID
}

// openPricePeriod closes whatever period is open for the product at the branch and opens a new
// one, the way setting a price does. Returns the moment the switch happened.
func (e *env) openPricePeriod(
	t *testing.T, accountID, branchID, productID uuid.UUID, price string, minPrice *string,
) time.Time {
	t.Helper()
	ctx := context.Background()
	var at time.Time
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT now()`).Scan(&at); err != nil {
		t.Fatalf("read the database clock: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE product_price SET valid_to = $4
		 WHERE account_id = $1 AND branch_id = $2 AND product_id = $3 AND valid_to IS NULL`,
		accountID, branchID, productID, at); err != nil {
		t.Fatalf("close the open price period: %v", err)
	}
	priceID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO product_price (id, account_id, branch_id, product_id, price, min_price,
		                            valid_from)
		 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7)`,
		priceID, accountID, branchID, productID, price, minPrice, at); err != nil {
		t.Fatalf("open a price period: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM product_price WHERE id = $1`, priceID) })
	return at
}

// seedDraftQuote writes what the RFQ pipeline leaves behind: an order, a quote at DRAFT, its
// unfrozen version one at a zero total, and one line per material with no prices on any of them.
func (e *env) seedDraftQuote(
	t *testing.T, accountID, branchID uuid.UUID, seller domain.AppUser, lines []draftLine,
) draft {
	t.Helper()
	ctx := context.Background()
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seeded := draft{rfqID: uuid.New(), quoteID: uuid.New(), versionID: uuid.New()}

	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, raw_text, status)
		 VALUES ($1, $2, $3, $4, $5, 'GENERATED')`,
		seeded.rfqID, accountID, branchID, channelID, "pedido de prueba"); err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote (id, account_id, branch_id, rfq_id, seller_id, current_status)
		 VALUES ($1, $2, $3, $4, $5, 'DRAFT')`,
		seeded.quoteID, accountID, branchID, seeded.rfqID, seller.ID); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_version (id, account_id, quote_id, author_id, version_number, total,
		                            is_immutable)
		 VALUES ($1, $2, $3, $4, 1, 0, FALSE)`,
		seeded.versionID, accountID, seeded.quoteID, seller.ID); err != nil {
		t.Fatalf("seed quote_version: %v", err)
	}
	for _, line := range lines {
		matchStatus := domain.ItemMatchStatusNoMatch
		if line.productID != nil {
			matchStatus = domain.ItemMatchStatusMatched
		}
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO quote_item (account_id, version_id, product_id, requested_description,
			                         quantity, match_status)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6)`,
			accountID, seeded.versionID, line.productID, line.description, line.quantity,
			matchStatus); err != nil {
			t.Fatalf("seed quote_item: %v", err)
		}
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote SET current_version_id = $2 WHERE id = $1`,
		seeded.quoteID, seeded.versionID); err != nil {
		t.Fatalf("point the quote at its version: %v", err)
	}
	e.dropDraft(t, seeded.rfqID)
	return seeded
}

// storedLine is one quote_item read straight back out, so an assertion reads the column rather
// than the response that claimed to have written it.
type storedLine struct {
	description string
	quantity    decimal.Decimal
	unitPrice   decimal.NullDecimal
	minPrice    decimal.NullDecimal
	subtotal    decimal.NullDecimal
}

func (e *env) storedLines(t *testing.T, versionID uuid.UUID) []storedLine {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT requested_description, quantity, unit_price_snapshot, min_price_snapshot, subtotal
		 FROM quote_item WHERE version_id = $1 ORDER BY requested_description`, versionID)
	if err != nil {
		t.Fatalf("read the lines back: %v", err)
	}
	defer rows.Close()

	var lines []storedLine
	for rows.Next() {
		var line storedLine
		if err := rows.Scan(&line.description, &line.quantity, &line.unitPrice, &line.minPrice,
			&line.subtotal); err != nil {
			t.Fatalf("scan a line: %v", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the lines back: %v", err)
	}
	return lines
}

func (e *env) storedVersionTotal(t *testing.T, versionID uuid.UUID) decimal.Decimal {
	t.Helper()
	var total decimal.Decimal
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT total FROM quote_version WHERE id = $1`, versionID).Scan(&total); err != nil {
		t.Fatalf("read the version total back: %v", err)
	}
	return total
}

func (e *env) acceptMaterials(t *testing.T, quoteID uuid.UUID, token, branch string) *pricedBody {
	t.Helper()
	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + quoteID.String() + "/accept-materials",
		token:  token, branch: branch,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var body pricedBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return &body
}

type pricedBody struct {
	Quote struct {
		CurrentStatus string `json:"current_status"`
	} `json:"quote"`
	Version struct {
		Total       string `json:"total"`
		IsImmutable bool   `json:"is_immutable"`
	} `json:"version"`
	Items []struct {
		RequestedDescription string  `json:"requested_description"`
		UnitPriceSnapshot    *string `json:"unit_price_snapshot"`
		MinPriceSnapshot     *string `json:"min_price_snapshot"`
		Subtotal             *string `json:"subtotal"`
	} `json:"items"`
}

// The criterion no fake can prove: the version is valued from frozen snapshots, so the account
// changing its price list afterwards leaves that version's total exactly where it was.
func TestQuotePricing_APriceChangeAfterValuationLeavesTheVersionAlone(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Quote pricing freeze")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	floor := "900.00"
	product := e.seedPricedProduct(t, accountID, branchID, "Cemento Portland 50kg", "1200.50",
		&floor)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "10 bolsas de cemento", quantity: "10", productID: &product}})

	body := e.acceptMaterials(t, seeded.quoteID, e.tokenFor(t, seller), branchID.String())

	// 10 × 1200.50, by hand.
	if body.Version.Total != "12005.00" {
		t.Fatalf("total = %q, want 12005.00", body.Version.Total)
	}
	if body.Quote.CurrentStatus != string(domain.QuoteStatusQuoted) {
		t.Errorf("status = %q, want QUOTED", body.Quote.CurrentStatus)
	}
	// QUOTED and a frozen version are different things: the seller still edits this draft.
	if body.Version.IsImmutable {
		t.Error("version is immutable, want the draft still editable")
	}

	// Now the corralón raises the price and the floor. This is the whole reason both values are
	// snapshotted onto the line instead of read through to product_price.
	e.openPricePeriod(t, accountID, branchID, product, "1500.00", nil)
	var live decimal.Decimal
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT price FROM product_price
		 WHERE product_id = $1 AND valid_to IS NULL`, product).Scan(&live); err != nil {
		t.Fatalf("read the new price: %v", err)
	}
	if !live.Equal(decimal.RequireFromString("1500.00")) {
		t.Fatalf("the new price did not take: price in force = %s, want 1500.00", live)
	}

	if got := e.storedVersionTotal(t, seeded.versionID); !got.Equal(
		decimal.RequireFromString("12005.00")) {
		t.Errorf("total after the price change = %s, want 12005.00 — the version is not frozen",
			got)
	}
	lines := e.storedLines(t, seeded.versionID)
	if len(lines) != 1 {
		t.Fatalf("stored %d lines, want 1", len(lines))
	}
	assertStored(t, "unit price after the price change", lines[0].unitPrice, "1200.50")
	// The floor is snapshotted with the price, so re-evaluating discounts on this version reads
	// the pair as it was — the new period having no floor at all cannot reach it.
	assertStored(t, "floor after the price change", lines[0].minPrice, "900.00")
	assertStored(t, "subtotal after the price change", lines[0].subtotal, "12005.00")
}

func TestQuotePricing_KeepsUnvaluedLinesNullAndRoundTripsTheScale(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Quote pricing nulls")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	// No floor: the column is nullable and most accounts never load one.
	priced := e.seedPricedProduct(t, accountID, branchID, "Arena fina", "1234.56", nil)
	// In the catalog and stocked, but the branch has never priced it.
	unpriced := e.seedProduct(t, accountID, "Hierro del 8", "hierro")
	e.stock(t, accountID, branchID, unpriced)

	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{
		{description: "a arena", quantity: "2.50", productID: &priced},
		{description: "b hierro", quantity: "12", productID: &unpriced},
		{description: "c algo que el catálogo no tiene", quantity: "7"},
	})

	body := e.acceptMaterials(t, seeded.quoteID, e.tokenFor(t, seller), branchID.String())

	// 2.50 × 1234.56 = 3086.40, by hand. Only that line contributes.
	if body.Version.Total != "3086.40" {
		t.Fatalf("total = %q, want 3086.40", body.Version.Total)
	}
	if len(body.Items) != 3 {
		t.Fatalf("returned %d lines, want all 3: an unvalued line is never dropped",
			len(body.Items))
	}

	lines := e.storedLines(t, seeded.versionID)
	if len(lines) != 3 {
		t.Fatalf("stored %d lines, want 3", len(lines))
	}
	// NUMERIC(14,2) holds the product exactly, and it comes back out at the stored scale.
	assertStored(t, "arena subtotal", lines[0].subtotal, "3086.40")
	assertStored(t, "arena unit price", lines[0].unitPrice, "1234.56")
	// Null, not 0.00. Read as zero, a later discount could take the line to nothing.
	if lines[0].minPrice.Valid {
		t.Errorf("arena floor = %s, want null: no floor is not a floor of zero",
			lines[0].minPrice.Decimal)
	}
	for _, line := range lines[1:] {
		if line.unitPrice.Valid || line.minPrice.Valid || line.subtotal.Valid {
			t.Errorf("%s = (%v, %v, %v), want all three null", line.description, line.unitPrice,
				line.minPrice, line.subtotal)
		}
	}

	if got := e.storedVersionTotal(t, seeded.versionID); !got.Equal(
		decimal.RequireFromString("3086.40")) {
		t.Errorf("stored total = %s, want 3086.40", got)
	}
	// The transition was recorded once, DRAFT to QUOTED, by the seller who accepted.
	var previous, next string
	var userID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT previous_status, new_status, user_id FROM quote_status_change WHERE quote_id = $1`,
		seeded.quoteID).Scan(&previous, &next, &userID); err != nil {
		t.Fatalf("read the status history: %v", err)
	}
	if previous != string(domain.QuoteStatusDraft) || next != string(domain.QuoteStatusQuoted) {
		t.Errorf("history = %q to %q, want DRAFT to QUOTED", previous, next)
	}
	if userID != seller.ID {
		t.Errorf("history user = %v, want the seller %v", userID, seller.ID)
	}
}

func TestQuotePricing_RefusesAQuoteOfAnotherAccount(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Quote pricing owner")
	otherAccountID, otherBranchID := e.seedAccount(t, "Quote pricing intruder")
	owner := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, owner, branchID)
	intruder := e.seedUser(t, otherAccountID, domain.UserRoleAdmin)

	product := e.seedPricedProduct(t, accountID, branchID, "Ladrillo hueco", "300.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, owner,
		[]draftLine{{description: "500 ladrillos", quantity: "500", productID: &product}})

	// The quote id is a real one; only the caller is wrong. A tenant-scoped id arriving in a
	// request proves nothing about who owns the row it names.
	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + seeded.quoteID.String() + "/accept-materials",
		token:  e.tokenFor(t, intruder), branch: otherBranchID.String(),
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}

	// And nothing of the owner's moved.
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT current_status FROM quote WHERE id = $1`, seeded.quoteID).Scan(&status); err != nil {
		t.Fatalf("read the quote back: %v", err)
	}
	if status != string(domain.QuoteStatusDraft) {
		t.Errorf("status = %q, want DRAFT", status)
	}
	if got := e.storedVersionTotal(t, seeded.versionID); !got.IsZero() {
		t.Errorf("total = %s, want 0", got)
	}
	if lines := e.storedLines(t, seeded.versionID); lines[0].subtotal.Valid {
		t.Errorf("subtotal = %s, want null", lines[0].subtotal.Decimal)
	}
	if changes := e.countStatusChanges(t, seeded.quoteID); changes != 0 {
		t.Errorf("status changes = %d, want none", changes)
	}
}

func TestQuotePricing_RefusesTheSecondAcceptance(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Quote pricing repeat")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	product := e.seedPricedProduct(t, accountID, branchID, "Cal hidratada", "500.00", nil)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller,
		[]draftLine{{description: "3 bolsas de cal", quantity: "3", productID: &product}})
	token := e.tokenFor(t, seller)

	e.acceptMaterials(t, seeded.quoteID, token, branchID.String())

	// The price list moves, and a second acceptance must not quietly re-value the version at the
	// new price: re-pricing is an explicit act of the seller's, not a repeated request.
	e.openPricePeriod(t, accountID, branchID, product, "700.00", nil)
	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + seeded.quoteID.String() + "/accept-materials",
		token:  token, branch: branchID.String(),
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if code := errorCode(t, rec); code != string(domain.CodeQuoteNotDraft) {
		t.Errorf("code = %q, want %q", code, domain.CodeQuoteNotDraft)
	}

	// 3 × 500.00, by hand — the first acceptance's price, not the second's.
	if got := e.storedVersionTotal(t, seeded.versionID); !got.Equal(
		decimal.RequireFromString("1500.00")) {
		t.Errorf("total = %s, want 1500.00", got)
	}
	// One transition, not two.
	if changes := e.countStatusChanges(t, seeded.quoteID); changes != 1 {
		t.Errorf("status changes = %d, want 1", changes)
	}
}

func (e *env) countStatusChanges(t *testing.T, quoteID uuid.UUID) int {
	t.Helper()
	var changes int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM quote_status_change WHERE quote_id = $1`,
		quoteID).Scan(&changes); err != nil {
		t.Fatalf("count status changes: %v", err)
	}
	return changes
}

func assertStored(t *testing.T, what string, got decimal.NullDecimal, want string) {
	t.Helper()
	if !got.Valid {
		t.Errorf("%s = null, want %s", what, want)
		return
	}
	if got.Decimal.StringFixed(domain.MoneyScale) != want {
		t.Errorf("%s = %s, want %s", what, got.Decimal.StringFixed(domain.MoneyScale), want)
	}
}
