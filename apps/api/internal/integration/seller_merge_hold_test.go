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
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// A seller merge into a customer's account holds back, until the customer's proof, every order the
// merged contacts bring — later ones included — and the merged contacts themselves.
func TestSellerMerge_HoldCoversLaterOrdersAndContacts(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	cuit := "20123456786"
	account := m.registered(t, "Cliente Web", "web@example.test", cuit)
	provisional := m.profile(t, "Cliente WhatsApp", "wa@example.test", nil)
	var customer uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, m.account, cuit).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history)
 SELECT account_id,$2,id,TRUE FROM client_contact WHERE account_id=$1 AND client_id=$3 AND retired_at IS NULL`, m.account, customer, account); err != nil {
		t.Fatal(err)
	}
	if status, code := m.merge(t, account, provisional); status != http.StatusOK {
		t.Fatalf("merge into the account: %d %s", status, code)
	}

	// A new order from the merged address routes to the account.
	route := domain.InboundChannelRoute{AccountID: m.account, BranchID: m.branch, ChannelID: m.channel, ChannelType: domain.ChannelTypeEmail}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	var later, message uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,'10 bolsas') RETURNING id`, m.account, m.branch, m.channel).Scan(&later); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,'wa@example.test','10 bolsas','{}',now()) RETURNING id`,
		m.account, m.branch, m.channel, later, uuid.NewString()).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if err := e.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return service.RecordInbound(ctx, q, route, message, domain.ChannelTypeEmail, "wa@example.test", "10 bolsas", domain.ClientContactHints{})
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteInbound(ctx, route, domain.ChannelTypeEmail, "wa@example.test", domain.ContactReplyContext{Automated: true}); err != nil {
		t.Fatal(err)
	}
	var owner uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT client_id FROM rfq WHERE id=$1`, later).Scan(&owner); err != nil || owner != account {
		t.Fatalf("the later order belongs to %s (%v), want the account %s", owner, err, account)
	}

	principal := domain.CustomerPrincipal{AccountID: m.account, CustomerID: customer, TaxID: cuit}
	contacts := repository.NewClientContactRepository()
	portal := func() (visible bool, listed bool) {
		t.Helper()
		if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
			orders, err := repository.NewCustomerOrderRepository().List(ctx, q, m.account, customer, 50, 0, domain.ARCAEnvironmentHomologation)
			if err != nil {
				return err
			}
			for _, o := range orders {
				visible = visible || o.ID == later
			}
			profile, err := contacts.ListForCustomer(ctx, q, principal, account, true)
			for _, c := range profile {
				listed = listed || c.Value == "wa@example.test"
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return visible, listed
	}
	if visible, listed := portal(); visible || listed {
		t.Fatalf("before any proof: later order visible=%v, merged contact listed=%v; want neither", visible, listed)
	}
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
		return contacts.ConfirmCustomerProof(ctx, q, m.account, account, domain.ChannelTypeEmail, "wa@example.test")
	}); err != nil {
		t.Fatal(err)
	}
	if visible, listed := portal(); !visible || !listed {
		t.Fatalf("after the proof: later order visible=%v, merged contact listed=%v; want both", visible, listed)
	}
}

// «Coincidencias» shows one case per pair of surviving profiles, never a profile paired with itself,
// and dismissing a case dismisses its duplicates.
func TestSellerMerge_SuggestionsOnePerPair(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	observe := func(source, candidate uuid.UUID, value string) uuid.UUID {
		var id uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,'EMAIL',$5::varchar,$5::text,'PENDING') RETURNING id`, m.account, m.branch, source, candidate, value).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	listed := func() map[uuid.UUID]bool {
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/client-contact-suggestions", token: m.token})
		var items []struct {
			ID uuid.UUID `json:"id"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &items) != nil {
			t.Fatalf("suggestions: %d %s", rec.Code, rec.Body)
		}
		ids := map[uuid.UUID]bool{}
		for _, item := range items {
			ids[item.ID] = true
		}
		return ids
	}
	status := func(id uuid.UUID) string {
		var s string
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT status::text FROM client_contact_observation WHERE id=$1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	a := m.profile(t, "A", "a@example.test", nil)
	b := m.profile(t, "B", "b@example.test", nil)
	c := m.profile(t, "C", "c@example.test", nil)
	first := observe(a, c, "shared-a@example.test")
	second := observe(b, c, "shared-b@example.test")
	if status, code := m.merge(t, b, a); status != http.StatusOK {
		t.Fatalf("merge A into B: %d %s", status, code)
	}
	ids := listed()
	if ids[first] == ids[second] {
		t.Fatalf("cases A–C and B–C now both pair B with C; want exactly one listed, got %v", ids)
	}
	shown := first
	if ids[second] {
		shown = second
	}
	if code := e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + shown.String(), token: m.token, body: map[string]any{"action": "SEPARATE"}}).Code; code != http.StatusNoContent {
		t.Fatalf("dismiss: %d", code)
	}
	if status(first) != "SEPARATE" || status(second) != "SEPARATE" {
		t.Fatalf("dismissing one case left its duplicate: %s / %s", status(first), status(second))
	}

	d := m.profile(t, "D", "d@example.test", nil)
	f := m.profile(t, "F", "f@example.test", nil)
	self := observe(d, f, "shared-d@example.test")
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET merged_into=$2 WHERE id=$1`, d, f); err != nil {
		t.Fatal(err)
	}
	if listed()[self] {
		t.Fatal("a case whose two sides already share a survivor is offered as a merge of a profile with itself")
	}
}
