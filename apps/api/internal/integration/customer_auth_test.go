//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func (e *env) registerCustomer(t *testing.T, accountID uuid.UUID, taxID, email string) (string, string) {
	t.Helper()
	hostname := "supplier-" + accountID.String() + ".coti.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET public_hostname=$2 WHERE id=$1`, accountID, hostname); err != nil {
		t.Fatal(err)
	}
	response := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + hostname + "/auth/register", body: map[string]any{"tax_id": taxID, "password": "Customer-pass123!", "recovery_email": email}})
	var login struct {
		Token string `json:"session_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusCreated || login.Token == "" {
		t.Fatalf("register customer: %d %s", response.Code, response.Body)
	}
	return hostname, login.Token
}

func TestCustomerAuthRecoveryRequiresVerifiedMatchingEmailAndRevokesSessions(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Customer recovery")
	hostname, token := e.registerCustomer(t, accountID, "20123456786", "recovery@example.test")
	base := "/v1/customer/sites/" + hostname
	if e.mail.count() != 1 {
		t.Fatalf("verification mail count: %d", e.mail.count())
	}
	verification := e.customerMailedToken(t, "/verify-email")
	forgot := func(email string) {
		t.Helper()
		rec := e.do(t, request{method: http.MethodPost, path: base + "/auth/forgot-password", body: map[string]any{"tax_id": "20123456786", "email": email}})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("forgot: %d %s", rec.Code, rec.Body)
		}
	}
	forgot("recovery@example.test")
	if e.mail.count() != 1 {
		t.Fatal("unverified recovery address received a reset")
	}
	verified := e.do(t, request{method: http.MethodPost, path: base + "/auth/verify-email", body: map[string]any{"token": verification}})
	if verified.Code != http.StatusNoContent {
		t.Fatalf("verify: %d %s", verified.Code, verified.Body)
	}
	forgot("attacker@example.test")
	if e.mail.count() != 1 {
		t.Fatal("a caller-selected recovery address received a reset")
	}
	forgot("recovery@example.test")
	if e.mail.count() != 2 {
		t.Fatal("verified address did not receive recovery")
	}
	reset := e.customerMailedToken(t, "/reset-password")
	response := e.do(t, request{method: http.MethodPost, path: base + "/auth/reset-password", body: map[string]any{"token": reset, "password": "Replacement-pass123!"}})
	if response.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", response.Code, response.Body)
	}
	if rec := e.do(t, request{method: http.MethodGet, path: base + "/me", token: token}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session survived reset: %d", rec.Code)
	}
	if rec := e.do(t, request{method: http.MethodPost, path: base + "/auth/reset-password", body: map[string]any{"token": reset, "password": "Replacement-again123!"}}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("reset reused: %d", rec.Code)
	}
	if rec := e.do(t, request{method: http.MethodPost, path: base + "/auth/login", body: map[string]any{"tax_id": "20123456786", "password": "Replacement-pass123!"}}); rec.Code != http.StatusOK {
		t.Fatalf("replacement login: %d %s", rec.Code, rec.Body)
	}
}

func TestCustomerHistoryIncludesEarlierCUITRequestsButIsolatesCustomersAndSuppliers(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Customer history")
	otherAccount, _ := e.seedAccount(t, "Other supplier")
	channelID, rfqID := uuid.New(), uuid.New()
	_, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel(id,account_id,branch_id,type) VALUES ($1,$2,$3,'MANUAL_ENTRY')`, channelID, accountID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	clientID := uuid.New()
	if _, err = e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client(id,account_id,name,tax_id) VALUES($1,$2,'Previous client','20123456786')`, clientID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.CrossAccount().Exec(context.Background(), `INSERT INTO rfq(id,account_id,branch_id,channel_id,client_id,raw_text) VALUES($1,$2,$3,$4,$5,'Previous counter request')`, rfqID, accountID, branchID, channelID, clientID); err != nil {
		t.Fatal(err)
	}
	host, token := e.registerCustomer(t, accountID, "20123456786", "")
	base := "/v1/customer/sites/" + host
	rec := e.do(t, request{method: http.MethodGet, path: base + "/orders", token: token})
	var orders []struct {
		ID    uuid.UUID `json:"id"`
		Token *string   `json:"delivery_token"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(orders) != 1 || orders[0].ID != rfqID || orders[0].Token != nil {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	_, otherToken := e.registerCustomer(t, accountID, "30712345671", "")
	if rec = e.do(t, request{method: http.MethodGet, path: base + "/orders/" + rfqID.String(), token: otherToken}); rec.Code != http.StatusNotFound {
		t.Fatalf("other customer read request: %d %s", rec.Code, rec.Body)
	}
	otherHost, _ := e.registerCustomer(t, otherAccount, "20123456786", "")
	if rec = e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + otherHost + "/orders", token: token}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-supplier session accepted: %d", rec.Code)
	}
	if rec = e.do(t, request{method: http.MethodGet, path: base + "/orders"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous history: %d", rec.Code)
	}
	if rec = e.do(t, request{method: http.MethodPost, path: base + "/auth/logout", token: token}); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec = e.do(t, request{method: http.MethodGet, path: base + "/orders", token: token}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session accepted: %d", rec.Code)
	}
}

func (e *env) customerMailedToken(t *testing.T, path string) string {
	t.Helper()
	msg, ok := e.mail.last()
	if !ok {
		t.Fatal("no customer mail")
	}
	_, after, ok := strings.Cut(msg.TextBody, path+"?token=")
	if !ok {
		t.Fatal("missing customer mail capability")
	}
	token, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(token)
}
