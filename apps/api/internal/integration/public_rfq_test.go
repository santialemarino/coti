//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
)

func TestPublicRFQRoutePreservesScopeAndIdempotencyWithoutClientAssociation(t *testing.T) {
	e := newEnvWithRFQProviders(t, rfqTestProviders{extractor: stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "10 bolsas de cemento", Quantity: decimal.NewFromInt(10),
		Source: domain.QuantitySourceExplicit, QuantityRationale: "el cliente pidió 10 bolsas",
	}}}})
	accountID, branchID := e.seedAccount(t, "public intake supplier")
	otherAccountID, otherBranchID := e.seedAccount(t, "other public intake supplier")
	channelID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel
		(id, account_id, branch_id, type) VALUES ($1, $2, $3, 'WEBAPP')`, channelID, accountID, branchID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM inbound_channel_message WHERE account_id = $1`, accountID) })
	path := "/v1/public/rfq-channels/" + channelID.String()
	contextResponse := e.do(t, request{method: http.MethodGet, path: path})
	if contextResponse.Code != http.StatusOK {
		t.Fatalf("public destination: %d %s", contextResponse.Code, contextResponse.Body.String())
	}
	body := map[string]any{"request_id": uuid.NewString(), "tax_id": "20123456786", "name": "Public customer", "email": "customer@example.com", "text": "10 bolsas de cemento",
		"account_id": otherAccountID, "branch_id": otherBranchID, "client_id": uuid.New()}
	for range 2 {
		response := e.do(t, request{method: http.MethodPost, path: path + "/requests", body: body})
		if response.Code != http.StatusCreated || response.Body.String() != `{"received":true}` {
			t.Fatalf("receipt: %d %s", response.Code, response.Body.String())
		}
	}
	var rfqID, actualBranch uuid.UUID
	var clientID, sellerID *uuid.UUID
	var rfqCount, quoteCount, clientCount int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT r.id, r.branch_id, r.client_id, q.seller_id
		FROM rfq r JOIN quote q ON q.rfq_id = r.id WHERE r.account_id = $1 AND r.channel_id = $2`, accountID, channelID).
		Scan(&rfqID, &actualBranch, &clientID, &sellerID); err != nil {
		t.Fatal(err)
	}
	e.dropDraft(t, rfqID)
	if actualBranch != branchID || clientID != nil || sellerID != nil {
		t.Fatalf("unexpected public routing or association: %v %v %v", actualBranch, clientID, sellerID)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM rfq WHERE account_id = $1),
		(SELECT count(*) FROM quote WHERE account_id = $1),
		(SELECT count(*) FROM client WHERE account_id = $1)`, accountID).Scan(&rfqCount, &quoteCount, &clientCount); err != nil {
		t.Fatal(err)
	}
	if rfqCount != 1 || quoteCount != 1 || clientCount != 0 {
		t.Fatalf("RFQs=%d quotes=%d clients=%d", rfqCount, quoteCount, clientCount)
	}
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	detail := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + rfqID.String(), token: e.tokenFor(t, admin), branch: branchID.String()})
	var payload struct {
		Contact *struct {
			Email string `json:"email"`
		} `json:"public_contact"`
		Text string `json:"public_original_text"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if detail.Code != http.StatusOK || payload.Contact == nil || payload.Contact.Email != "customer@example.com" || payload.Text != "10 bolsas de cemento" {
		t.Fatalf("seller source: %d %s", detail.Code, detail.Body.String())
	}
	otherAdmin := e.seedUser(t, otherAccountID, domain.UserRoleAdmin)
	denied := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + rfqID.String(), token: e.tokenFor(t, otherAdmin), branch: otherBranchID.String()})
	if denied.Code != http.StatusNotFound {
		t.Fatalf("cross-account detail status: %d", denied.Code)
	}
}
