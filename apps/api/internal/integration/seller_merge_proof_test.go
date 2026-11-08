//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// reply records one inbound email from value that declares a name, the way a name reply arrives.
func (m *sellerMerge) reply(t *testing.T, value, name string) {
	t.Helper()
	ctx := context.Background()
	route := domain.InboundChannelRoute{AccountID: m.account, BranchID: m.branch, ChannelID: m.channel, ChannelType: domain.ChannelTypeEmail}
	service := services.NewClientContactService(m.e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	service.WithMessaging(&captureContactMessenger{})
	var rfq, message uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,$4) RETURNING id`, m.account, m.branch, m.channel, "Soy "+name).Scan(&rfq); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,$6,$7,'{}',now()) RETURNING id`,
		m.account, m.branch, m.channel, rfq, uuid.NewString(), value, "Soy "+name).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return service.RecordInbound(ctx, q, route, message, domain.ChannelTypeEmail, value, "Soy "+name, domain.ClientContactHints{Name: &name})
	}); err != nil {
		t.Fatal(err)
	}
}

// askName leaves a delivered name question open for value, as onboarding does for a new sender.
func (m *sellerMerge) askName(t *testing.T, value string) {
	t.Helper()
	if _, err := m.e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose,status) VALUES($1,$2,$3,'EMAIL',$4,'REQUEST_NAME','SENT')`, m.account, m.branch, m.channel, value); err != nil {
		t.Fatal(err)
	}
}

func TestSellerMerge_LaterCustomerProofBringsTheMergedHistory(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	const tax = "20123456786"
	survivor := m.profile(t, "Counter buyer", "counter-e@example.test", nil)
	source := m.profile(t, "Counter buyer (second mail)", "counter-s@example.test", nil)
	order := m.order(t, source, m.branch)
	if status, code := m.merge(t, survivor, source); status != http.StatusNoContent {
		t.Fatalf("merge: status = %d (%s)", status, code)
	}

	host, token := e.registerCustomer(t, m.account, tax)
	e.verifyCustomerContact(t, m.account, tax, domain.ChannelTypeEmail, "counter-s@example.test")
	var registered uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM client WHERE account_id=$1 AND tax_id=$2`, m.account, tax).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if into := m.mergedInto(t, survivor); into == nil || *into != registered {
		t.Fatalf("the seller's survivor merged_into = %v, want the registered profile %s", into, registered)
	}
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + order.String(), token: token}); rec.Code != http.StatusOK {
		t.Fatalf("the merged source's order: status = %d, body = %s, want it in the customer's history", rec.Code, rec.Body)
	}
}

func TestSellerMerge_SettlesOpenNameQuestions(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	survivor := m.profile(t, "Ferreteria Lopez", "lopez@example.test", nil)
	source := m.profile(t, "pedro@example.test", "pedro@example.test", nil)
	m.askName(t, "pedro@example.test")
	if status, code := m.merge(t, survivor, source); status != http.StatusNoContent {
		t.Fatalf("merge: status = %d (%s)", status, code)
	}
	m.reply(t, "pedro@example.test", "Pedro Gomez")
	if got := e.profileNameOf(t, survivor); got != "Ferreteria Lopez" {
		t.Fatalf("a name reply to the merged profile renamed the survivor to %q", got)
	}

	// A seller's correction is final in the same way.
	asked := m.profile(t, "🔨 Juanchi", "juanchi@example.test", nil)
	m.askName(t, "juanchi@example.test")
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/clients/" + asked.String() + "/name", token: m.token, body: map[string]any{"name": "Juan Pérez"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d, body = %s", rec.Code, rec.Body)
	}
	m.reply(t, "juanchi@example.test", "Juanchi")
	if got := e.profileNameOf(t, asked); got != "Juan Pérez" {
		t.Fatalf("a late name reply overwrote the seller's correction with %q", got)
	}
}

func TestSellerMerge_ResolvesObservationsOnSurvivorsAndRefusesAStrangerToKeep(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	resolve := func(id uuid.UUID, body map[string]any) int {
		return e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + id.String(), token: m.token, body: body}).Code
	}
	owner := m.profile(t, "Owner", "owner-s@example.test", nil)
	loser := m.profile(t, "Loser", "loser-s@example.test", nil)
	stranger := m.profile(t, "Stranger", "stranger-s@example.test", nil)
	var pick uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,'EMAIL','owner-s@example.test','owner-s@example.test','PENDING') RETURNING id`, m.account, m.branch, loser, owner).Scan(&pick); err != nil {
		t.Fatal(err)
	}
	if code := resolve(pick, map[string]any{"action": "MERGE", "keep_client_id": stranger}); code != http.StatusUnprocessableEntity {
		t.Fatalf("keep_client_id naming a third profile: status = %d, want 422", code)
	}
	for _, id := range []uuid.UUID{owner, loser, stranger} {
		if into := m.mergedInto(t, id); into != nil {
			t.Fatalf("a refused keep merged %s into %s", id, *into)
		}
	}

	// The loser was merged elsewhere since the case was raised: the case merges its survivor.
	elsewhere := m.profile(t, "Elsewhere", "elsewhere-s@example.test", nil)
	if status, code := m.merge(t, elsewhere, loser); status != http.StatusNoContent {
		t.Fatalf("merge: status = %d (%s)", status, code)
	}
	if code := resolve(pick, map[string]any{"action": "MERGE"}); code != http.StatusNoContent {
		t.Fatalf("resolve after a side was merged: status = %d, want 204", code)
	}
	if into := m.mergedInto(t, elsewhere); into == nil || *into != owner {
		t.Fatalf("the loser's survivor merged_into = %v, want the owner %s", into, owner)
	}
	if into := m.mergedInto(t, loser); into == nil || *into != owner {
		t.Fatalf("the loser merged_into = %v, want it repointed to the owner %s", into, owner)
	}
}
