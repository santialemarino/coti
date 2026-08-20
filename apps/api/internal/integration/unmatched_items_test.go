//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The unmatched-items report, end to end: a flagged line keeps the candidates it was decided
// against, they come back attached to that line and no other, another account reaches none of
// them, and a frozen version keeps its marks after the next version resolves them.

// storedAlternative is one candidate read straight out of the table, so an assertion reads the
// column rather than the response that claimed to have written it.
type storedAlternative struct {
	itemID     uuid.UUID
	productID  *uuid.UUID
	rank       int
	confidence decimal.NullDecimal
	kind       string
	origin     string
	price      decimal.NullDecimal
}

func (e *env) storedAlternatives(t *testing.T, versionID uuid.UUID) []storedAlternative {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT a.quote_item_id, a.product_id, a.rank, a.confidence_score, a.type, a.origin,
		        a.price_snapshot
		 FROM quote_item_alternative a
		 JOIN quote_item i ON i.id = a.quote_item_id
		 WHERE i.version_id = $1
		 ORDER BY i.requested_description, a.rank`, versionID)
	if err != nil {
		t.Fatalf("read the candidates back: %v", err)
	}
	defer rows.Close()

	var stored []storedAlternative
	for rows.Next() {
		var row storedAlternative
		if err := rows.Scan(&row.itemID, &row.productID, &row.rank, &row.confidence, &row.kind,
			&row.origin, &row.price); err != nil {
			t.Fatalf("scan a candidate: %v", err)
		}
		stored = append(stored, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the candidates back: %v", err)
	}
	return stored
}

// freezeVersion is what sending a quote will do. There is no route for it yet, so the state AC5
// is about is reached directly.
func (e *env) freezeVersion(t *testing.T, versionID uuid.UUID) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_version SET is_immutable = TRUE WHERE id = $1`, versionID); err != nil {
		t.Fatalf("freeze the version: %v", err)
	}
}

// seedResolvedNextVersion writes the version a seller produces by resolving the flagged line: a
// second version of the same quote, with the line matched and no candidate left on it.
func (e *env) seedResolvedNextVersion(
	t *testing.T, accountID, quoteID, productID uuid.UUID, description string,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	versionID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_version (id, account_id, quote_id, version_number, total, is_immutable)
		 VALUES ($1, $2, $3, 2, 0, FALSE)`, versionID, accountID, quoteID); err != nil {
		t.Fatalf("seed the next version: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description,
		                         quantity, match_status)
		 VALUES ($1, $2, $3, $4, 1, 'MATCHED')`,
		accountID, versionID, productID, description); err != nil {
		t.Fatalf("seed the resolved line: %v", err)
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM quote_item WHERE version_id = $1`, versionID)
		e.mustCleanup(t, `DELETE FROM quote_version WHERE id = $1`, versionID)
	})
	return versionID
}

// ambiguousDraft runs the real pipeline over a catalog staged so one line is AMBIGUOUS between two
// near-identical products and another is NO_MATCH with a rejected near miss behind it.
type ambiguousDraft struct {
	draft      *domain.TextRFQDraft
	leader     uuid.UUID
	runnerUp   uuid.UUID
	nearMiss   uuid.UUID
	cementItem uuid.UUID
	strayItem  uuid.UUID
}

func (e *env) ambiguousDraft(
	t *testing.T, accountID, branchID uuid.UUID, seller domain.AppUser,
) ambiguousDraft {
	t.Helper()
	channelID := e.seedIntakeChannel(t, accountID, branchID)

	// Two cements a hair apart: the leader clears the floor, and the margin between them falls
	// under the ambiguity margin, which is what AMBIGUOUS means.
	leader := e.seedProduct(t, accountID, "Cemento Portland 50kg", "bolsa de cemento")
	runnerUp := e.seedProduct(t, accountID, "Cemento Portland 25kg", "bolsa de cemento")
	// On the second line's axis and below the confidence floor: a near miss, which is the one
	// thing that tells the seller how close the catalog came.
	nearMiss := e.seedProduct(t, accountID, "Membrana asfáltica 4mm", "membrana")
	for _, productID := range []uuid.UUID{leader, runnerUp, nearMiss} {
		e.stock(t, accountID, branchID, productID)
	}
	e.embedOn(t, leader, 0, 0.95)
	e.embedOn(t, runnerUp, 0, 0.94)
	e.embedOn(t, nearMiss, 2, 0.40)
	axes := map[string]int{"cemento portland": 0, "membrana rara": 2}

	rationale := "el cliente pidió 10 bolsas"
	strayRationale := "el cliente pidió 2 rollos"
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{
		{
			RequestedDescription: "cemento portland",
			Quantity:             decimal.RequireFromString("10"),
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    rationale,
		},
		{
			RequestedDescription: "membrana rara",
			Quantity:             decimal.RequireFromString("2"),
			Source:               domain.QuantitySourceExplicit,
			QuantityRationale:    strayRationale,
		},
	}}, axes).CreateTextDraft(context.Background(),
		domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
			Role: domain.UserRoleAdmin},
		domain.TextRFQDraftInput{
			ChannelID: channelID, RawText: "cemento portland y membrana rara",
		})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want no error", err)
	}
	e.dropDraft(t, draft.RFQ.ID)

	if draft.Version == nil || len(draft.Items) != 2 {
		t.Fatalf("draft = %+v, want a version and both lines", draft)
	}
	staged := ambiguousDraft{
		draft: draft, leader: leader, runnerUp: runnerUp, nearMiss: nearMiss,
	}
	for _, item := range draft.Items {
		switch item.RequestedDescription {
		case "cemento portland":
			staged.cementItem = item.ID
		case "membrana rara":
			staged.strayItem = item.ID
		}
	}
	if staged.cementItem == uuid.Nil || staged.strayItem == uuid.Nil {
		t.Fatalf("lines = %+v, want both descriptions back", draft.Items)
	}
	return staged
}

// The ticket's third criterion, against real SQL and the real search: an ambiguous line offers the
// products it might have been, and a line nothing matched offers what came closest.
func TestUnmatchedItems_OffersTheCandidatesOfEveryFlaggedLine(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Unmatched items report")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	staged := e.ambiguousDraft(t, accountID, branchID, seller)

	statuses := map[uuid.UUID]domain.ItemMatchStatus{}
	for _, item := range staged.draft.Items {
		statuses[item.ID] = item.MatchStatus
	}
	if statuses[staged.cementItem] != domain.ItemMatchStatusAmbiguous {
		t.Fatalf("cemento line = %q, want AMBIGUOUS: the fixture stages two products a hair apart",
			statuses[staged.cementItem])
	}
	if statuses[staged.strayItem] != domain.ItemMatchStatusNoMatch {
		t.Fatalf("membrana line = %q, want NO_MATCH", statuses[staged.strayItem])
	}

	// Read the rows back rather than trusting the returned structs: the criterion says persisted.
	stored := e.storedAlternatives(t, staged.draft.Version.ID)
	if len(stored) != 2 {
		t.Fatalf("stored %d candidates, want 2: the ambiguous runner-up and the near miss", len(stored))
	}
	byItem := map[uuid.UUID][]storedAlternative{}
	for _, row := range stored {
		byItem[row.itemID] = append(byItem[row.itemID], row)
	}

	// The AMBIGUOUS line keeps the leader on itself and offers the other cement. Rank is the
	// matcher's own ranking, so its offer is the runner-up at two rather than a renumbered one.
	cementOffers := byItem[staged.cementItem]
	if len(cementOffers) != 1 {
		t.Fatalf("cemento line offers %d, want the one it is not", len(cementOffers))
	}
	// Which cement the fusion ranked first is the search's business, so the expectation is derived
	// from the line's own product: the offer is the other one, at the alignment it was seeded with.
	onTheLine := lineProduct(t, staged.draft.Items, staged.cementItem)
	wantOffer, wantConfidence := staged.runnerUp, "0.9400"
	if onTheLine == staged.runnerUp {
		wantOffer, wantConfidence = staged.leader, "0.9500"
	}
	if cementOffers[0].productID == nil || *cementOffers[0].productID != wantOffer {
		t.Errorf("cemento offer = %v, want the other cement %v", cementOffers[0].productID,
			wantOffer)
	}
	if cementOffers[0].rank != 2 {
		t.Errorf("cemento offer rank = %d, want 2", cementOffers[0].rank)
	}
	// 1 − distance at the seeded alignment, rounded to the column's scale.
	if !cementOffers[0].confidence.Valid ||
		!cementOffers[0].confidence.Decimal.Equal(decimal.RequireFromString(wantConfidence)) {
		t.Errorf("cemento offer confidence = %v, want %s", cementOffers[0].confidence,
			wantConfidence)
	}
	if cementOffers[0].kind != string(domain.QuoteItemAlternativeTypeProduct) ||
		cementOffers[0].origin != string(domain.QuoteItemAlternativeOriginAI) {
		t.Errorf("cemento offer = (%q, %q), want (PRODUCT, AI)", cementOffers[0].kind,
			cementOffers[0].origin)
	}
	// Nothing is priced when matching runs, and a zero would read as free.
	if cementOffers[0].price.Valid {
		t.Errorf("cemento offer carries price %v, want none", cementOffers[0].price)
	}

	// The NO_MATCH line points at nothing, so its rejected near miss is on offer at rank one.
	strayOffers := byItem[staged.strayItem]
	if len(strayOffers) != 1 {
		t.Fatalf("membrana line offers %d, want the rejected near miss", len(strayOffers))
	}
	if strayOffers[0].productID == nil || *strayOffers[0].productID != staged.nearMiss {
		t.Errorf("membrana offer = %v, want %v", strayOffers[0].productID, staged.nearMiss)
	}
	if strayOffers[0].rank != 1 {
		t.Errorf("membrana offer rank = %d, want 1: the line kept no leader", strayOffers[0].rank)
	}

	// Each line's offers name that line and no other. A crossed pairing is a wrong product on a
	// seller's screen and nothing downstream could notice it.
	if len(staged.draft.Alternatives[staged.cementItem]) != 1 ||
		len(staged.draft.Alternatives[staged.strayItem]) != 1 {
		t.Fatalf("returned offers = %+v, want one per flagged line", staged.draft.Alternatives)
	}
	for itemID, offers := range staged.draft.Alternatives {
		for _, offer := range offers {
			if offer.QuoteItemID != itemID {
				t.Errorf("line %v was handed an offer belonging to %v", itemID, offer.QuoteItemID)
			}
		}
	}
	// The catalog identity is joined, which is what makes an offer readable at all.
	returned := staged.draft.Alternatives[staged.cementItem][0]
	wantName := "Cemento Portland 25kg"
	if onTheLine == staged.runnerUp {
		wantName = "Cemento Portland 50kg"
	}
	if returned.CanonicalName == nil || *returned.CanonicalName != wantName {
		t.Errorf("offer name = %v, want %q from the catalog", returned.CanonicalName, wantName)
	}
}

// lineProduct is the product a line ended up pointing at, which is what decides who is on offer.
func lineProduct(t *testing.T, items []domain.QuoteItem, itemID uuid.UUID) uuid.UUID {
	t.Helper()
	for _, item := range items {
		if item.ID != itemID {
			continue
		}
		if item.ProductID == nil {
			t.Fatalf("line %v points at no product", itemID)
		}
		return *item.ProductID
	}
	t.Fatalf("line %v is not among the draft's lines", itemID)
	return uuid.Nil
}

// The ticket's fifth criterion: version one keeps the marks it had, and the candidates behind
// them, after version two resolves the line.
func TestUnmatchedItems_AFrozenVersionKeepsItsMarks(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Unmatched items freeze")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	staged := e.ambiguousDraft(t, accountID, branchID, seller)
	firstVersion := staged.draft.Version.ID

	// The seller sends version one and then resolves the unmatched line into version two, which
	// is the sequence the criterion is about.
	e.freezeVersion(t, firstVersion)
	e.seedResolvedNextVersion(t, accountID, staged.draft.Quote.ID, staged.nearMiss,
		"membrana rara")

	repo := repository.NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
		Role: domain.UserRoleAdmin}
	var items []domain.QuoteItem
	var offers map[uuid.UUID][]domain.QuoteItemAlternative
	if err := e.db.InTenantTx(context.Background(), tenant, func(q repository.Querier) error {
		var err error
		if items, err = repo.ListItems(context.Background(), q, accountID, firstVersion); err != nil {
			return err
		}
		itemIDs := make([]uuid.UUID, 0, len(items))
		for _, item := range items {
			itemIDs = append(itemIDs, item.ID)
		}
		offers, err = repo.ListAlternativesByItemIDs(context.Background(), q, accountID, itemIDs)
		return err
	}); err != nil {
		t.Fatalf("read version one back: %v", err)
	}

	// Two lines, not three: version two's resolved line belongs to version two.
	if len(items) != 2 {
		t.Fatalf("version one holds %d lines, want the 2 it was written with", len(items))
	}
	marks := map[uuid.UUID]domain.ItemMatchStatus{}
	for _, item := range items {
		marks[item.ID] = item.MatchStatus
	}
	if marks[staged.strayItem] != domain.ItemMatchStatusNoMatch {
		t.Errorf("frozen membrana line = %q, want NO_MATCH: the next version resolving it does not "+
			"rewrite history", marks[staged.strayItem])
	}
	if marks[staged.cementItem] != domain.ItemMatchStatusAmbiguous {
		t.Errorf("frozen cemento line = %q, want AMBIGUOUS", marks[staged.cementItem])
	}
	// And the candidates behind the marks are still there: the report on a frozen version says
	// what was considered at the time, which is what makes it auditable.
	if len(offers[staged.strayItem]) != 1 ||
		offers[staged.strayItem][0].ProductID == nil ||
		*offers[staged.strayItem][0].ProductID != staged.nearMiss {
		t.Errorf("frozen membrana offers = %+v, want the near miss it was rejected against",
			offers[staged.strayItem])
	}
	if len(offers[staged.cementItem]) != 1 {
		t.Errorf("frozen cemento offers = %+v, want the runner-up", offers[staged.cementItem])
	}
}

func TestUnmatchedItems_AnotherAccountReachesNoCandidate(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Unmatched items owner")
	otherAccountID, _ := e.seedAccount(t, "Unmatched items intruder")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	intruder := e.seedUser(t, otherAccountID, domain.UserRoleAdmin)
	staged := e.ambiguousDraft(t, accountID, branchID, seller)

	repo := repository.NewQuoteRepository()
	itemIDs := []uuid.UUID{staged.cementItem, staged.strayItem}
	// Real line ids, the wrong caller. A tenant-scoped id arriving from anywhere proves nothing
	// about who owns the row it names.
	var offers map[uuid.UUID][]domain.QuoteItemAlternative
	if err := e.db.InTenantTx(context.Background(),
		domain.Tenant{AccountID: otherAccountID, UserID: intruder.ID, Role: domain.UserRoleAdmin},
		func(q repository.Querier) error {
			var err error
			offers, err = repo.ListAlternativesByItemIDs(context.Background(), q, otherAccountID,
				itemIDs)
			return err
		}); err != nil {
		t.Fatalf("ListAlternativesByItemIDs() = %v, want no error", err)
	}
	if len(offers) != 0 {
		t.Errorf("the intruder read %+v, want nothing", offers)
	}
}

// Valuation does not change which line is flagged, so the candidates ride back with the prices and
// the seller reviews both on one screen.
func TestUnmatchedItems_SurviveValuation(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Unmatched items valuation")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	staged := e.ambiguousDraft(t, accountID, branchID, admin)
	// The ambiguous line's leader has a price; the unmatched one has nothing to price.
	e.openPricePeriod(t, accountID, branchID, staged.leader, "1200.00", nil)

	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + staged.draft.Quote.ID.String() + "/accept-materials",
		token:  e.tokenFor(t, seller), branch: branchID.String(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	body := decodeReportBody(t, rec.Body.Bytes())

	if len(body.Items) != 2 {
		t.Fatalf("returned %d lines, want both", len(body.Items))
	}
	for _, item := range body.Items {
		if len(item.Alternatives) != 1 {
			t.Errorf("line %q offers %d candidates, want the one it was decided against",
				item.RequestedDescription, len(item.Alternatives))
			continue
		}
		offer := item.Alternatives[0]
		if offer.CanonicalName == nil || *offer.CanonicalName == "" {
			t.Errorf("line %q offers a candidate with no name", item.RequestedDescription)
		}
		if offer.ConfidenceScore == nil {
			t.Errorf("line %q offers a candidate with no score", item.RequestedDescription)
		}
		// Valuation has run, so every line answers the pricing question one way or the other.
		if item.PricingUnavailable == nil {
			t.Errorf("line %q leaves pricing_unavailable null after valuation",
				item.RequestedDescription)
		}
	}
}

// reportBody is the part of the priced response this ticket added: the candidates on each line and
// the answer to whether the branch could price it.
type reportBody struct {
	Items []struct {
		RequestedDescription string  `json:"requested_description"`
		MatchStatus          string  `json:"match_status"`
		Subtotal             *string `json:"subtotal"`
		PricingUnavailable   *bool   `json:"pricing_unavailable"`
		Alternatives         []struct {
			ProductID       *uuid.UUID `json:"product_id"`
			Rank            int        `json:"rank"`
			ConfidenceScore *string    `json:"confidence_score"`
			CanonicalName   *string    `json:"canonical_name"`
			Origin          string     `json:"origin"`
		} `json:"alternatives"`
	} `json:"items"`
}

func decodeReportBody(t *testing.T, raw []byte) reportBody {
	t.Helper()
	var body reportBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return body
}

// The gap PR #83 left unreported: a line the catalog decided, whose product the branch cannot
// price. It is neither NO_MATCH nor AMBIGUOUS, and the seller still has to act on it.
func TestUnmatchedItems_NamesTheLineTheBranchCannotPrice(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Unmatched items pricing gap")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, branchID)
	priced := e.seedPricedProduct(t, accountID, branchID, "Ladrillo hueco 12", "300.00", nil)
	// Carried at the branch and in the catalog, but no price period was ever opened for it.
	unpriceable := e.seedProduct(t, accountID, "Membrana asfáltica 4mm", "membrana")
	e.stock(t, accountID, branchID, unpriceable)
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, []draftLine{
		{description: "500 ladrillos", quantity: "500", productID: &priced},
		{description: "2 rollos de membrana", quantity: "2", productID: &unpriceable},
		{description: "algo que no está en el catálogo", quantity: "1"},
	})

	rec := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/quotes/" + seeded.quoteID.String() + "/accept-materials",
		token:  e.tokenFor(t, seller), branch: branchID.String(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	body := decodeReportBody(t, rec.Body.Bytes())
	if len(body.Items) != 3 {
		t.Fatalf("returned %d lines, want all 3", len(body.Items))
	}

	gaps := map[string]bool{}
	statuses := map[string]string{}
	for _, item := range body.Items {
		if item.PricingUnavailable == nil {
			t.Fatalf("line %q leaves pricing_unavailable null after valuation",
				item.RequestedDescription)
		}
		gaps[item.RequestedDescription] = *item.PricingUnavailable
		statuses[item.RequestedDescription] = item.MatchStatus
	}

	if gaps["500 ladrillos"] {
		t.Error("the priced line is reported as a pricing gap")
	}
	if !gaps["2 rollos de membrana"] {
		t.Error("the line the branch cannot price is not reported; the seller has no way to see it")
	}
	// It stays MATCHED: match_status answers what the catalog decided, and conflating the two
	// would lose the difference between a product nobody could find and one nobody priced.
	if statuses["2 rollos de membrana"] != string(domain.ItemMatchStatusMatched) {
		t.Errorf("unpriceable line status = %q, want MATCHED",
			statuses["2 rollos de membrana"])
	}
	// A line with no product is already flagged NO_MATCH; saying it again would be noise.
	if gaps["algo que no está en el catálogo"] {
		t.Error("a line with no product is reported as a pricing gap as well as NO_MATCH")
	}

	// 500 × 300.00, by hand: the unpriceable line contributes nothing and does not block the rest.
	if got := e.storedVersionTotal(t, seeded.versionID); !got.Equal(
		decimal.RequireFromString("150000.00")) {
		t.Errorf("total = %s, want 150000.00", got)
	}
}
