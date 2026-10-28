//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCustomerHistory_RecycledContactOpensOnlyOrdersSinceItsProof(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Recycled contact history")
	ctx := context.Background()
	phone := "+5491155550142"
	whatsapp := e.historyChannel(t, account, branch, "WHATSAPP")

	host, tokenA := e.registerCustomer(t, account, "20123456786", "")
	e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeWhatsApp, phone)
	provedByA := e.contactOwnershipTime(t, account, "20123456786", phone)
	earlier := e.observedOrder(t, account, branch, whatsapp, phone, provedByA, nil)
	if !e.customerOrderIDs(t, host, tokenA)[earlier] {
		t.Fatal("the owner did not see an order its proved contact sent")
	}
	leave := map[string]any{"name": "Juan Perez", "contacts": []map[string]any{{"kind": "EMAIL", "value": "juan-new@example.test", "primary": true}}}
	if rec := e.do(t, request{method: http.MethodPut, path: "/v1/customer/sites/" + host + "/profile", token: tokenA, body: leave}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove contact: %d %s", rec.Code, rec.Body)
	}

	_, tokenB := e.registerCustomer(t, account, "30712345671", "")
	var customerB uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id='30712345671'`, account).Scan(&customerB); err != nil {
		t.Fatal(err)
	}
	portal := e.historyChannel(t, account, branch, "WEBAPP")
	authored := e.observedOrder(t, account, branch, portal, "", time.Now().Add(-2*time.Hour), map[string]any{"customer_id": customerB.String()})
	e.verifyCustomerContact(t, account, "30712345671", domain.ChannelTypeWhatsApp, phone)
	provedByB := e.contactOwnershipTime(t, account, "30712345671", phone)
	later := e.observedOrder(t, account, branch, whatsapp, phone, provedByB.Add(time.Second), nil)

	seen := e.customerOrderIDs(t, host, tokenB)
	if seen[earlier] {
		t.Fatal("a recycled contact opened the previous owner's order")
	}
	if !seen[later] || !seen[authored] || len(seen) != 2 {
		t.Fatalf("history after proof=%v", seen)
	}
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + earlier.String(), token: tokenB}); rec.Code != http.StatusNotFound {
		t.Fatalf("previous owner's order detail=%d", rec.Code)
	}

	e.verifyCustomerContact(t, account, "30712345671", domain.ChannelTypeWhatsApp, phone, true)
	if again := e.contactOwnershipTime(t, account, "30712345671", phone); !again.Equal(provedByB) {
		t.Fatalf("a repeated proof moved ownership from %s to %s", provedByB, again)
	}
}

func TestCustomerHistory_MergedProfileKeepsItsEarlierOrders(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Merged profile history")
	ctx := context.Background()
	email := "provisional-history@example.test"
	var source uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client(account_id,name,email) VALUES($1,'Provisional','`+email+`') RETURNING id`, account).Scan(&source); err != nil {
		t.Fatal(err)
	}
	channel := e.historyChannel(t, account, branch, "EMAIL")
	order := e.observedOrder(t, account, branch, channel, email, time.Now().Add(-24*time.Hour), nil)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE rfq SET client_id=$3 WHERE account_id=$1 AND id=$2`, account, order, source); err != nil {
		t.Fatal(err)
	}
	host, token := e.registerCustomer(t, account, "20123456786", "")
	e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeEmail, email)
	if !e.customerOrderIDs(t, host, token)[order] {
		t.Fatal("a merged profile's earlier order left the history")
	}
}

func TestCustomerHistory_LegacyProofOpensTheProfilePast(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Legacy profile history")
	ctx := context.Background()
	legacy := "legacy-history@example.test"
	var profile uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client(account_id,name,tax_id,email) VALUES($1,'Legacy','20123456786','`+legacy+`') RETURNING id`, account).Scan(&profile); err != nil {
		t.Fatal(err)
	}
	channel := e.historyChannel(t, account, branch, "EMAIL")
	order := e.observedOrder(t, account, branch, channel, legacy, time.Now().Add(-24*time.Hour), nil)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE rfq SET client_id=$3 WHERE account_id=$1 AND id=$2`, account, order, profile); err != nil {
		t.Fatal(err)
	}
	host, token := e.registerCustomer(t, account, "20123456786", "")
	declare := map[string]any{"name": "Juan Perez", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": "customer20123456786@example.test", "primary": true},
		{"kind": "EMAIL", "value": legacy, "primary": false},
	}}
	if rec := e.do(t, request{method: http.MethodPut, path: "/v1/customer/sites/" + host + "/profile", token: token, body: declare}); rec.Code != http.StatusNoContent {
		t.Fatalf("declare legacy contact: %d %s", rec.Code, rec.Body)
	}
	e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeEmail, legacy, true)
	if !e.customerOrderIDs(t, host, token)[order] {
		t.Fatal("proving a legacy contact did not open the profile's earlier order")
	}
}

func (e *env) historyChannel(t *testing.T, account, branch uuid.UUID, kind string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `INSERT INTO channel(account_id,branch_id,type) VALUES($1,$2,$3) RETURNING id`, account, branch, kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// observedOrder seeds a request with no profile, reached only through its inbound message: an
// observation of value when one is given, otherwise the payload.
func (e *env) observedOrder(t *testing.T, account, branch, channel uuid.UUID, value string, receivedAt time.Time, payload map[string]any) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var rfq, message uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text,received_at) VALUES($1,$2,$3,'Observed order',$4) RETURNING id`, account, branch, channel, receivedAt).Scan(&rfq); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,$6,'Observed order',$7,$8) RETURNING id`,
		account, branch, channel, rfq, uuid.NewString(), value, body, receivedAt).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if value == "" {
		return rfq
	}
	kind := domain.ChannelTypeEmail
	if value[0] == '+' {
		kind = domain.ChannelTypeWhatsApp
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,message_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,$5,'Observed order','RECORDED')`, account, branch, message, kind, value); err != nil {
		t.Fatal(err)
	}
	return rfq
}

func (e *env) contactOwnershipTime(t *testing.T, account uuid.UUID, taxID, value string) time.Time {
	t.Helper()
	var at time.Time
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT o.verified_at FROM customer_contact_ownership o
 JOIN customer_identity i ON i.account_id=o.account_id AND i.id=o.customer_id
 JOIN client_contact c ON c.account_id=o.account_id AND c.id=o.contact_id
 WHERE o.account_id=$1 AND i.tax_id=$2 AND c.value=$3`, account, taxID, value).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func (e *env) customerOrderIDs(t *testing.T, host, token string) map[uuid.UUID]bool {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders", token: token})
	if rec.Code != http.StatusOK {
		t.Fatalf("orders: %d %s", rec.Code, rec.Body)
	}
	var orders []struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatal(err)
	}
	ids := make(map[uuid.UUID]bool, len(orders))
	for _, order := range orders {
		ids[order.ID] = true
	}
	return ids
}
