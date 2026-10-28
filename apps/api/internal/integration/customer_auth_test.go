//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func (e *env) registerCustomer(t *testing.T, accountID uuid.UUID, taxID string) (string, string) {
	t.Helper()
	hostname := "supplier-" + accountID.String() + ".coti.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET public_hostname=$2 WHERE id=$1`, accountID, hostname); err != nil {
		t.Fatal(err)
	}
	response := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + hostname + "/auth/register", body: map[string]any{"tax_id": taxID, "password": "Customer-pass123!", "name": "Juan Perez", "contacts": []map[string]any{{"kind": "EMAIL", "value": "customer" + taxID + "@example.test", "primary": true}}}})
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

func TestCustomerHistoryRequiresContactProofAndIsolatesCustomersAndSuppliers(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Customer history")
	otherAccount, _ := e.seedAccount(t, "Other supplier")
	channelID, rfqID := uuid.New(), uuid.New()
	_, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel(id,account_id,branch_id,type) VALUES ($1,$2,$3,'MANUAL_ENTRY')`, channelID, accountID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	clientID := uuid.New()
	if _, err = e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client(id,account_id,name,tax_id,email) VALUES($1,$2,'Previous client','20123456786','legacy@example.test')`, clientID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.CrossAccount().Exec(context.Background(), `INSERT INTO rfq(id,account_id,branch_id,channel_id,client_id,raw_text) VALUES($1,$2,$3,$4,$5,'Previous counter request')`, rfqID, accountID, branchID, channelID, clientID); err != nil {
		t.Fatal(err)
	}
	host, token := e.registerCustomer(t, accountID, "20123456786")
	base := "/v1/customer/sites/" + host
	rec := e.do(t, request{method: http.MethodGet, path: base + "/orders", token: token})
	var orders []struct {
		ID    uuid.UUID `json:"id"`
		Token *string   `json:"delivery_token"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(orders) != 0 {
		t.Fatalf("unproved CUIT exposed history: %d %s", rec.Code, rec.Body)
	}
	if denied := e.do(t, request{method: http.MethodGet, path: base + "/orders/" + rfqID.String(), token: token}); denied.Code != http.StatusNotFound {
		t.Fatalf("unproved detail status=%d", denied.Code)
	}

	e.verifyCustomerContact(t, accountID, "20123456786", "EMAIL", "customer20123456786@example.test", true)
	rec = e.do(t, request{method: http.MethodGet, path: base + "/orders", token: token})
	if err = json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 {
		t.Fatalf("new signup contact unlocked legacy history: %s", rec.Body)
	}
	declare := map[string]any{"name": "Juan Perez", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": "customer20123456786@example.test", "primary": true},
		{"kind": "EMAIL", "value": "legacy@example.test", "primary": false},
	}}
	if rec = e.do(t, request{method: http.MethodPut, path: base + "/profile", token: token, body: declare}); rec.Code != http.StatusNoContent {
		t.Fatalf("declare legacy contact: %d %s", rec.Code, rec.Body)
	}
	e.verifyCustomerContact(t, accountID, "20123456786", "EMAIL", "legacy@example.test", true)
	rec = e.do(t, request{method: http.MethodGet, path: base + "/orders", token: token})
	if err = json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(orders) != 1 || orders[0].ID != rfqID || orders[0].Token != nil {
		t.Fatalf("history: %d %s", rec.Code, rec.Body)
	}
	_, otherToken := e.registerCustomer(t, accountID, "30712345671")
	if rec = e.do(t, request{method: http.MethodGet, path: base + "/orders/" + rfqID.String(), token: otherToken}); rec.Code != http.StatusNotFound {
		t.Fatalf("other customer read request: %d %s", rec.Code, rec.Body)
	}
	otherHost, _ := e.registerCustomer(t, otherAccount, "20123456786")

	if err := e.db.InTenantTx(context.Background(), domain.Tenant{AccountID: otherAccount}, func(q repository.Querier) error {
		var count int
		if err := q.QueryRow(context.Background(), `SELECT count(*) FROM customer_contact_ownership WHERE account_id=$1`, accountID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("ownership rows crossed RLS")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history)
 SELECT $1,i.id,o.contact_id,false FROM customer_identity i CROSS JOIN customer_contact_ownership o WHERE i.account_id=$1 AND o.account_id=$2 LIMIT 1`, otherAccount, accountID); err == nil {
		t.Fatal("ownership accepted a foreign contact")
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO customer_contact_history(account_id,customer_id,rfq_id,kind,value)
 SELECT $1,id,$2,'EMAIL','foreign@example.test' FROM customer_identity WHERE account_id=$1`, otherAccount, rfqID); err == nil {
		t.Fatal("history accepted a foreign RFQ")
	}
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
