//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/shopspring/decimal"
)

func TestPublicRFQFileRouteQueuesOneStoredAttachmentOnRetry(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "public file intake")
	channelID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel
		(id, account_id, branch_id, type) VALUES ($1, $2, $3, 'WEBAPP')`, channelID, accountID, branchID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM inbound_channel_message WHERE account_id = $1`, accountID) })
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"request_id": uuid.NewString(), "tax_id": "20123456786", "name": "Public file customer", "email": "customer@example.com"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="pedido.txt"`},
		"Content-Type":        {"text/plain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("10 bolsas de cemento")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/v1/public/rfq-channels/"+channelID.String()+"/requests", bytes.NewReader(body.Bytes()))
		req.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		e.router.ServeHTTP(response, req)
		if response.Code != http.StatusCreated || response.Body.String() != `{"received":true}` {
			t.Fatalf("file receipt: %d %s", response.Code, response.Body.String())
		}
	}
	var rfqID uuid.UUID
	var attachments, quotes int
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT r.id, a.processing_status,
		(SELECT count(*) FROM rfq_attachment WHERE account_id = $1),
		(SELECT count(*) FROM quote WHERE account_id = $1)
		FROM rfq r JOIN rfq_attachment a ON a.rfq_id = r.id AND a.account_id = r.account_id
		WHERE r.account_id = $1 AND r.branch_id = $2 AND r.channel_id = $3`, accountID, branchID, channelID).
		Scan(&rfqID, &status, &attachments, &quotes); err != nil {
		t.Fatal(err)
	}
	if attachments != 1 || quotes != 0 || status != "PENDING" {
		t.Fatalf("files=%d quotes=%d status=%s", attachments, quotes, status)
	}
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	detail := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + rfqID.String(), token: e.tokenFor(t, admin), branch: branchID.String()})
	var payload struct {
		Attachments []struct {
			URL string `json:"url"`
		} `json:"public_attachments"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if detail.Code != http.StatusOK || len(payload.Attachments) != 1 || payload.Attachments[0].URL == "" {
		t.Fatalf("seller file source: %d %s", detail.Code, detail.Body.String())
	}
}

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
