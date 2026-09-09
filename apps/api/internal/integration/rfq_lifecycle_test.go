//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedManualEntryChannel inserts the MANUAL_ENTRY channel a branch needs to host RFQs.
func seedManualEntryChannel(t *testing.T, e *env, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	var channelID uuid.UUID
	err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND type = 'MANUAL_ENTRY'`,
		accountID, branchID,
	).Scan(&channelID)
	if err == nil {
		return channelID
	}
	// Channel does not exist yet; create it.
	err = e.db.CrossAccount().QueryRow(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type)
		 VALUES ($1, $2, $3, 'MANUAL_ENTRY')
		 RETURNING id`,
		uuid.New(), accountID, branchID,
	).Scan(&channelID)
	if err != nil {
		t.Fatalf("seed manual entry channel: %v", err)
	}
	return channelID
}

func TestRfq_CreateAndList(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "RFQ Lifecycle")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	_ = seedManualEntryChannel(t, e, accountID, branchID)

	clientLabel := "Obra San Martín"
	rawText := "100 ladrillos huecos 8x18x33"
	workType := "obra nueva"

	createBody := map[string]any{
		"client_label": clientLabel,
		"raw_text":     rawText,
		"work_type":    workType,
		"items": []map[string]any{
			{
				"requested_description": "Ladrillo hueco 8x18x33",
				"quantity":              "100",
				"unit":                  "un",
			},
			{
				"requested_description": "Ladrillo semihueco 18x18x33",
				"quantity":              "50",
			},
		},
	}
	body, _ := json.Marshal(createBody)

	createRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs",
		token:  token,
		branch: branchID.String(),
		body:   json.RawMessage(body),
	})
	if createRes.Code != http.StatusCreated {
		t.Fatalf("POST /v1/rfqs = %d, want 201 (body %s)", createRes.Code, createRes.Body.String())
	}

	var created struct {
		Rfq struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"rfq"`
		Quote struct {
			ID            string `json:"id"`
			CurrentStatus string `json:"current_status"`
		} `json:"quote"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if created.Rfq.ID == "" {
		t.Fatal("rfq.id must be present")
	}
	if created.Rfq.Status != "GENERATED" {
		t.Errorf("rfq.status = %q, want GENERATED", created.Rfq.Status)
	}
	if created.Quote.ID == "" {
		t.Fatal("quote.id must be present")
	}
	if created.Quote.CurrentStatus != "DRAFT" {
		t.Errorf("quote.current_status = %q, want DRAFT", created.Quote.CurrentStatus)
	}

	// ── GET /v1/rfqs ──────────────────────────────────────────────────────────

	listRes := e.do(t, request{
		method: http.MethodGet,
		path:   "/v1/rfqs",
		token:  token,
		branch: branchID.String(),
	})
	if listRes.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs = %d, want 200 (body %s)", listRes.Code, listRes.Body.String())
	}

	var listBody []struct {
		ID        string  `json:"id"`
		Client    *string `json:"client"`
		Channel   string  `json:"channel"`
		Seller    string  `json:"seller"`
		Branch    string  `json:"branch"`
		ItemCount int     `json:"item_count"`
		Total     *string `json:"total"`
		Status    string  `json:"status"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}

	if len(listBody) != 1 {
		t.Fatalf("list items = %d, want 1", len(listBody))
	}

	item := listBody[0]
	if item.ID != created.Rfq.ID {
		t.Errorf("list item id = %s, want %s", item.ID, created.Rfq.ID)
	}
	if item.Client == nil || *item.Client != clientLabel {
		t.Errorf("client = %v, want %q", item.Client, clientLabel)
	}
	if item.Channel != "manual_entry" {
		t.Errorf("channel = %q, want manual_entry", item.Channel)
	}
	if item.ItemCount != 2 {
		t.Errorf("item_count = %d, want 2", item.ItemCount)
	}
	if item.Total != nil {
		t.Errorf("total = %v, want nil (no priced version yet)", item.Total)
	}
	// DRAFT is the quote's current_status, which becomes the merged display status.
	if item.Status != "DRAFT" {
		t.Errorf("status = %q, want DRAFT", item.Status)
	}
}

func TestRfq_ListShowsItemsFromMultipleAccounts(t *testing.T) {
	e := newEnv(t)

	// Account A
	accountA, branchA := e.seedAccount(t, "Corralón A")
	userA := e.seedUser(t, accountA, domain.UserRoleSeller)
	e.assignBranch(t, accountA, userA, branchA)
	tokenA := e.tokenFor(t, userA)
	_ = seedManualEntryChannel(t, e, accountA, branchA)

	// Account B
	accountB, branchB := e.seedAccount(t, "Corralón B")
	userB := e.seedUser(t, accountB, domain.UserRoleSeller)
	e.assignBranch(t, accountB, userB, branchB)
	tokenB := e.tokenFor(t, userB)
	_ = seedManualEntryChannel(t, e, accountB, branchB)

	// Create one RFQ in each account.
	for _, tc := range []struct {
		token  string
		branch uuid.UUID
		label  string
	}{
		{tokenA, branchA, "Cliente de A"},
		{tokenB, branchB, "Cliente de B"},
	} {
		raw := "50 metros de cable"
		body, _ := json.Marshal(map[string]any{
			"client_label": tc.label,
			"raw_text":     raw,
			"items": []map[string]any{
				{"requested_description": "Cable 2.5mm", "quantity": "50", "unit": "m"},
			},
		})
		res := e.do(t, request{
			method: http.MethodPost,
			path:   "/v1/rfqs",
			token:  tc.token,
			branch: tc.branch.String(),
			body:   json.RawMessage(body),
		})
		if res.Code != http.StatusCreated {
			t.Fatalf("create RFQ for %s = %d, want 201 (body %s)", tc.label, res.Code, res.Body.String())
		}
	}

	// Each account sees only its own RFQ.
	for _, tc := range []struct {
		name   string
		token  string
		branch uuid.UUID
		count  int
	}{
		{"account A", tokenA, branchA, 1},
		{"account B", tokenB, branchB, 1},
	} {
		res := e.do(t, request{
			method: http.MethodGet,
			path:   "/v1/rfqs",
			token:  tc.token,
			branch: tc.branch.String(),
		})
		if res.Code != http.StatusOK {
			t.Fatalf("list for %s = %d, want 200", tc.name, res.Code)
		}

		var body []struct {
			Client *string `json:"client"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode list for %s: %v", tc.name, err)
		}
		if len(body) != tc.count {
			t.Errorf("%s: items = %d, want %d", tc.name, len(body), tc.count)
		}
	}
}

// seedInboxRFQ drops an RFQ and its quote straight into the account with seller_id NULL — the
// shape a WhatsApp or email order arrives in, and the only shape the assign route answers.
func (e *env) seedInboxRFQ(
	t *testing.T, accountID, branchID, channelID uuid.UUID,
) (rfqID uuid.UUID) {
	t.Helper()
	rfqID = uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, client_label, status)
		 VALUES ($1, $2, $3, $4, 'Cliente del inbox', 'GENERATED')`,
		rfqID, accountID, branchID, channelID); err != nil {
		t.Fatalf("seed inbox rfq: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote (id, account_id, number, branch_id, rfq_id, seller_id, current_status)
		 VALUES ($1, $2, (('x'||substr(replace($4::uuid::text,'-',''),1,15))::bit(60)::bigint), $3, $4, NULL, 'DRAFT')`,
		uuid.New(), accountID, branchID, rfqID); err != nil {
		t.Fatalf("seed inbox quote: %v", err)
	}
	return rfqID
}

// The full self-assignment happy path and its refusals, end to end: two sellers in one branch,
// an admin, and an order that lives in a branch the first seller cannot see.
func TestRfq_SellerClaimsAnUnassignedOrder(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Seller claim")
	otherBranchID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, $3)`,
		otherBranchID, accountID, "Sucursal Lejos"); err != nil {
		t.Fatalf("seed second branch: %v", err)
	}

	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller1 := e.seedUser(t, accountID, domain.UserRoleSeller)
	seller2 := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller1, branchID)
	e.assignBranch(t, accountID, seller2, branchID)
	token1 := e.tokenFor(t, seller1)
	token2 := e.tokenFor(t, seller2)
	tokenAdmin := e.tokenFor(t, admin)

	channelID := seedManualEntryChannel(t, e, accountID, branchID)
	otherChannelID := seedManualEntryChannel(t, e, accountID, otherBranchID)
	rfqID := e.seedInboxRFQ(t, accountID, branchID, channelID)
	hiddenRFQ := e.seedInboxRFQ(t, accountID, otherBranchID, otherChannelID)

	// The unassigned order is readable and claimable for the seller, and its row already
	// carries no seller.
	listRes := e.do(t, request{
		method: http.MethodGet, path: "/v1/rfqs", token: token1, branch: branchID.String(),
	})
	if listRes.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs = %d, want 200 (body %s)", listRes.Code, listRes.Body.String())
	}
	var listBody []struct {
		ID     string `json:"id"`
		Seller string `json:"seller"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody) != 1 || listBody[0].ID != rfqID.String() {
		t.Fatalf("seller listed %v, want only %v", listBody, rfqID)
	}
	if listBody[0].Seller != "" {
		t.Errorf("unassigned order listed seller %q, want empty", listBody[0].Seller)
	}

	// The claim lands: the response carries the seller id and the draft status.
	claimRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs/" + rfqID.String() + "/assign",
		token:  token1,
		branch: branchID.String(),
	})
	if claimRes.Code != http.StatusOK {
		t.Fatalf("POST assign = %d, want 200 (body %s)", claimRes.Code, claimRes.Body.String())
	}
	var claimed struct {
		ID            string  `json:"id"`
		RFQID         string  `json:"rfq_id"`
		SellerID      *string `json:"seller_id"`
		CurrentStatus string  `json:"current_status"`
	}
	if err := json.Unmarshal(claimRes.Body.Bytes(), &claimed); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if claimed.ID == "" || claimed.RFQID != rfqID.String() {
		t.Fatalf("claim response %+v, want the claimed quote for %v", claimed, rfqID)
	}
	if claimed.SellerID == nil || *claimed.SellerID != seller1.ID.String() {
		t.Errorf("claim seller_id = %v, want %v", claimed.SellerID, seller1.ID)
	}
	if claimed.CurrentStatus != "DRAFT" {
		t.Errorf("claim current_status = %q, want DRAFT (claiming never moves the status)", claimed.CurrentStatus)
	}

	// Only one owner: the peer, the owner's own retry, and the admin all come back refused.
	secondRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs/" + rfqID.String() + "/assign",
		token:  token2,
		branch: branchID.String(),
	})
	if secondRes.Code != http.StatusConflict {
		t.Errorf("peer claim = %d, want 409 (body %s)", secondRes.Code, secondRes.Body.String())
	}
	reclaimRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs/" + rfqID.String() + "/assign",
		token:  token1,
		branch: branchID.String(),
	})
	if reclaimRes.Code != http.StatusConflict {
		t.Errorf("owner re-claim = %d, want 409 (body %s)", reclaimRes.Code, reclaimRes.Body.String())
	}
	adminRes := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs/" + rfqID.String() + "/assign",
		token:  tokenAdmin,
		branch: branchID.String(),
	})
	if adminRes.Code != http.StatusForbidden {
		t.Errorf("admin claim = %d, want 403 (body %s)", adminRes.Code, adminRes.Body.String())
	}

	// A claimed order disappears from a peer's list and its detail refuses them.
	list2 := e.do(t, request{
		method: http.MethodGet, path: "/v1/rfqs", token: token2, branch: branchID.String(),
	})
	if list2.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs (peer) = %d, want 200", list2.Code)
	}
	var list2Body []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(list2.Body.Bytes(), &list2Body); err != nil {
		t.Fatalf("decode peer list: %v", err)
	}
	if len(list2Body) != 0 {
		t.Errorf("peer still lists the claimed order: %v", list2Body)
	}
	peerDetail := e.do(t, request{
		method: http.MethodGet,
		path:   "/v1/rfqs/" + rfqID.String(),
		token:  token2,
		branch: branchID.String(),
	})
	if peerDetail.Code != http.StatusNotFound {
		t.Errorf("peer detail of a claimed order = %d, want 404", peerDetail.Code)
	}

	// The hidden order in another branch cannot be claimed or reached at all.
	hiddenClaim := e.do(t, request{
		method: http.MethodPost,
		path:   "/v1/rfqs/" + hiddenRFQ.String() + "/assign",
		token:  token1,
		branch: branchID.String(),
	})
	if hiddenClaim.Code != http.StatusNotFound {
		t.Errorf("claim of an out-of-branch order = %d, want 404 (body %s)",
			hiddenClaim.Code, hiddenClaim.Body.String())
	}
}

// Admin seller management end to end: hand an order to another seller of the branch, back to
// themselves, off again — with a seller refused before the route, a foreign seller refused at
// the service, and a seller of another branch refused because they would never list the order.
func TestRfq_AdminSteersTheSeller(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Admin seller steering")
	otherBranchID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, $3)`,
		otherBranchID, accountID, "Sucursal Lejos"); err != nil {
		t.Fatalf("seed second branch: %v", err)
	}

	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller1 := e.seedUser(t, accountID, domain.UserRoleSeller)
	seller2 := e.seedUser(t, accountID, domain.UserRoleSeller)
	farSeller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller1, branchID)
	e.assignBranch(t, accountID, seller2, branchID)
	e.assignBranch(t, accountID, farSeller, otherBranchID)
	tokenAdmin := e.tokenFor(t, admin)
	tokenSeller := e.tokenFor(t, seller1)

	channelID := seedManualEntryChannel(t, e, accountID, branchID)
	rfqID := e.seedInboxRFQ(t, accountID, branchID, channelID)

	set := func(token string, branch string, sellerID any) *httptest.ResponseRecorder {
		return e.do(t, request{
			method: http.MethodPut,
			path:   "/v1/rfqs/" + rfqID.String() + "/seller",
			token:  token,
			branch: branch,
			body:   map[string]any{"seller_id": sellerID},
		})
	}

	// Hand the order to seller2 of the same branch: the write lands and the quote carries it.
	reassigned := set(tokenAdmin, branchID.String(), seller2.ID.String())
	if reassigned.Code != http.StatusOK {
		t.Fatalf("admin reassign = %d, want 200 (body %s)", reassigned.Code, reassigned.Body.String())
	}
	var quoteBody struct {
		SellerID *string `json:"seller_id"`
	}
	if err := json.Unmarshal(reassigned.Body.Bytes(), &quoteBody); err != nil {
		t.Fatalf("decode reassign response: %v", err)
	}
	if quoteBody.SellerID == nil || *quoteBody.SellerID != seller2.ID.String() {
		t.Errorf("reassigned seller_id = %v, want %v", quoteBody.SellerID, seller2.ID)
	}

	// Visibility moved with the owner: seller1 no longer lists the order, seller2 does.
	list1 := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs", token: tokenSeller, branch: branchID.String()})
	list2 := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs", token: e.tokenFor(t, seller2), branch: branchID.String()})
	var list1Body, list2Body []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(list1.Body.Bytes(), &list1Body); err != nil {
		t.Fatalf("decode seller1 list: %v", err)
	}
	if err := json.Unmarshal(list2.Body.Bytes(), &list2Body); err != nil {
		t.Fatalf("decode seller2 list: %v", err)
	}
	if len(list1Body) != 0 {
		t.Errorf("seller1 still lists the reassigned order: %v", list1Body)
	}
	if len(list2Body) != 1 || list2Body[0].ID != rfqID.String() {
		t.Errorf("seller2 lists %v, want only the reassigned order", list2Body)
	}

	// A seller never reaches the route; the middleware answers 403 before the service.
	if refused := set(tokenSeller, branchID.String(), seller2.ID.String()); refused.Code != http.StatusForbidden {
		t.Errorf("seller PUT seller = %d, want 403 (body %s)", refused.Code, refused.Body.String())
	}

	// A seller who does not serve the order's branch is refused, exactly like a manual entry.
	if foreign := set(tokenAdmin, branchID.String(), farSeller.ID.String()); foreign.Code != http.StatusUnprocessableEntity {
		t.Errorf("far-branch seller PUT = %d, want 422 (body %s)", foreign.Code, foreign.Body.String())
	}

	// An unknown seller id fails the reach query the same way.
	if unknown := set(tokenAdmin, branchID.String(), uuid.New().String()); unknown.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown seller PUT = %d, want 422 (body %s)", unknown.Code, unknown.Body.String())
	}

	// The admin assigns the order to themselves: no seller-reach applies to a caller already
	// inside the account, and the write overwrites seller2.
	self := set(tokenAdmin, branchID.String(), admin.ID.String())
	if self.Code != http.StatusOK {
		t.Fatalf("admin self-assign = %d, want 200 (body %s)", self.Code, self.Body.String())
	}
	if err := json.Unmarshal(self.Body.Bytes(), &quoteBody); err != nil {
		t.Fatalf("decode self-assign response: %v", err)
	}
	if quoteBody.SellerID == nil || *quoteBody.SellerID != admin.ID.String() {
		t.Errorf("self-assigned seller_id = %v, want %v", quoteBody.SellerID, admin.ID)
	}

	// A null seller_id clears the order; the account-wide admin reaches it without a branch.
	cleared := set(tokenAdmin, "", nil)
	if cleared.Code != http.StatusOK {
		t.Fatalf("admin clear = %d, want 200 (body %s)", cleared.Code, cleared.Body.String())
	}
	if err := json.Unmarshal(cleared.Body.Bytes(), &quoteBody); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if quoteBody.SellerID != nil {
		t.Errorf("cleared seller_id = %v, want nil", quoteBody.SellerID)
	}

	// An order that does not exist is the one legitimate 404 on this route. It must arrive as
	// the JSON error DTO, not as Gin's plain "404 page not found" that a dropped route would
	// answer — the assertion pins the distinction so a routing regression cannot masquerade
	// as a missing resource.
	missing := e.do(t, request{
		method: http.MethodPut,
		path:   "/v1/rfqs/" + uuid.New().String() + "/seller",
		token:  tokenAdmin,
		branch: branchID.String(),
		body:   map[string]any{"seller_id": seller2.ID.String()},
	})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing RFQ PUT seller = %d, want 404 (body %s)", missing.Code, missing.Body.String())
	}
	if body := missing.Body.String(); len(body) == 0 || body[0] != '{' {
		t.Errorf("missing RFQ PUT seller body %q, want the JSON error DTO (routing 404 would answer text/plain)", body)
	}
}
