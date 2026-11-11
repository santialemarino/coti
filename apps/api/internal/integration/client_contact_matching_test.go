//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// typeNewClient confirms a new profile with phone, and email unless empty, on the seeded quote, as a
// seller does at send time.
func (e *env) typeNewClient(t *testing.T, seed sendableQuote, name, phone, email string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	clients := services.NewClientService(e.db, repository.NewClientRepository(), repository.NewTagRepository(), repository.NewQuoteRepository(), repository.NewRFQRepository(), repository.NewQuoteSendRepository())
	var clientID uuid.UUID
	if err := e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
		quote, err := repository.NewQuoteRepository().GetByID(ctx, q, seed.tenant.AccountID, seed.tenant.BranchID, seed.draft.Quote.ID)
		if err != nil {
			return err
		}
		typed := &domain.NewClient{Name: &name, Phone: &phone}
		if email != "" {
			typed.Email = &email
		}
		if err := clients.ConfirmForSend(ctx, q, seed.tenant, *quote, &domain.AssociateQuoteClientInput{New: typed}); err != nil {
			return err
		}
		return q.QueryRow(ctx, `SELECT client_id FROM quote WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, seed.draft.Quote.ID).Scan(&clientID)
	}); err != nil {
		t.Fatal(err)
	}
	return clientID
}

// declareCustomerContact adds value, unverified, beside the email registerCustomer signed up with.
func (e *env) declareCustomerContact(t *testing.T, account uuid.UUID, tax string, kind domain.ChannelType, value string) {
	t.Helper()
	ctx := context.Background()
	p := domain.CustomerPrincipal{AccountID: account, TaxID: tax}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	contacts := []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "customer" + tax + "@example.test", Primary: true}, {Kind: kind, Value: value, Primary: true}}
	if err := service.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: contacts}); err != nil {
		t.Fatal(err)
	}
}

type contactState struct {
	value    string
	source   string
	verified bool
}

func (e *env) contactsOf(t *testing.T, clientID uuid.UUID) []contactState {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(), `SELECT value,source::text,verified_at IS NOT NULL FROM client_contact WHERE client_id=$1 AND retired_at IS NULL ORDER BY kind,value`, clientID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []contactState
	for rows.Next() {
		var c contactState
		if err := rows.Scan(&c.value, &c.source, &c.verified); err != nil {
			t.Fatal(err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

// pendingMatches lists the open «Coincidencias» on value as source>candidate pairs.
func (e *env) pendingMatches(t *testing.T, account uuid.UUID, value string) [][2]uuid.UUID {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(), `SELECT source_client_id,candidate_client_id FROM client_contact_observation WHERE account_id=$1 AND value=$2 AND status='PENDING' ORDER BY created_at,id`, account, value)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result [][2]uuid.UUID
	for rows.Next() {
		var pair [2]uuid.UUID
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			t.Fatal(err)
		}
		result = append(result, pair)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

// whatsAppOrder records one inbound WhatsApp message from wa the way ingress does and returns its request.
func (e *env) whatsAppOrder(t *testing.T, tenant domain.Tenant, channel uuid.UUID, wa string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	route := domain.InboundChannelRoute{AccountID: tenant.AccountID, BranchID: tenant.BranchID, ChannelID: channel, ChannelType: domain.ChannelTypeWhatsApp}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	var rfq, message uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,'Pedido por WhatsApp') RETURNING id`, tenant.AccountID, tenant.BranchID, channel).Scan(&rfq); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,$6,'Pedido por WhatsApp','{}',$7) RETURNING id`,
		tenant.AccountID, tenant.BranchID, channel, rfq, uuid.NewString(), wa, time.Now()).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if err := e.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
		return service.RecordInbound(ctx, q, route, message, domain.ChannelTypeWhatsApp, "+"+wa, "Pedido por WhatsApp", domain.ClientContactHints{})
	}); err != nil {
		t.Fatal(err)
	}
	var client uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT o.source_client_id FROM client_contact_observation o WHERE o.account_id=$1 AND o.message_id=$2`, tenant.AccountID, message).Scan(&client); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientContactMatching_InboundSenderOnlyTypedBySellerRaisesACoincidence(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Typed contact match", false)
	account := seed.tenant.AccountID

	typed := e.typeNewClient(t, seed, "Obra Belgrano", "+54 11 5555-0123", "")
	if got := e.contactsOf(t, typed); len(got) != 1 || got[0] != (contactState{"+5491155550123", "SELLER", false}) {
		t.Fatalf("seller-typed contact = %+v, want +5491155550123 from the seller, unverified", got)
	}

	// Meta reports the number with the 9; it is the same contact, but the seller's typing proves
	// nothing, so the sender gets its own profile and the seller is asked about the pair.
	channel := e.historyChannel(t, account, seed.tenant.BranchID, "WHATSAPP")
	sender := e.whatsAppOrder(t, seed.tenant, channel, "5491155550123")
	if sender == typed {
		t.Fatal("an inbound message was routed to the profile whose contact a seller typed")
	}
	if got := e.contactsOf(t, sender); len(got) != 1 || got[0] != (contactState{"+5491155550123", "CHANNEL", true}) {
		t.Fatalf("sender contact = %+v, want the channel-verified +5491155550123", got)
	}
	if got := e.pendingMatches(t, account, "+5491155550123"); len(got) != 1 || got[0] != [2]uuid.UUID{sender, typed} {
		t.Fatalf("pending coincidences = %v, want one from the sender %s to the typed profile %s", got, sender, typed)
	}

	// The sender is known now: a second message routes to it and raises nothing new.
	if again := e.whatsAppOrder(t, seed.tenant, channel, "5491155550123"); again != sender {
		t.Fatalf("a second message went to %s, want the sender's profile %s", again, sender)
	}
	if got := e.pendingMatches(t, account, "+5491155550123"); len(got) != 1 {
		t.Fatalf("pending coincidences after a second message = %v, want still one", got)
	}
}

func TestClientContactMatching_SellerEntryClashingWithARegisteredProfileRaisesACoincidence(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Registered contact match", false)
	account := seed.tenant.AccountID
	const tax = "30712345671"
	e.registerCustomer(t, account, tax)
	e.declareCustomerContact(t, account, tax, domain.ChannelTypeWhatsApp, "+5492215550303")
	var registered uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM client WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&registered); err != nil {
		t.Fatal(err)
	}

	// The seller types both contacts the registered profile holds: one case for the pair, not one each.
	typed := e.typeNewClient(t, seed, "Mostrador La Plata", "+54 221 555 0303", "customer"+tax+"@example.test")
	if got := e.pendingMatches(t, account, "+5492215550303"); len(got) != 1 || got[0] != [2]uuid.UUID{typed, registered} {
		t.Fatalf("pending coincidences = %v, want one from the typed profile %s to the registered %s", got, typed, registered)
	}
	if got := e.pendingMatches(t, account, "customer"+tax+"@example.test"); len(got) != 0 {
		t.Fatalf("the shared email raised %v beside the phone's case, want the pair reviewed once", got)
	}

	seller := e.tokenFor(t, domain.AppUser{ID: seed.tenant.UserID, AccountID: account, Role: domain.UserRoleAdmin, SessionEpoch: 1})
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/client-contact-suggestions", token: seller, branch: seed.tenant.BranchID.String()})
	var list []struct {
		CandidateClientID   uuid.UUID `json:"candidate_client_id"`
		SourceRegistered    bool      `json:"source_registered"`
		CandidateRegistered bool      `json:"candidate_registered"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("suggestions: %d %s", rec.Code, rec.Body)
	}
	found := false
	for _, item := range list {
		if item.CandidateClientID == registered {
			found = item.CandidateRegistered && !item.SourceRegistered
		}
	}
	if !found {
		t.Fatalf("suggestions = %s, want the registered profile as a registered candidate", rec.Body)
	}

	// The customer proved another contact but not the case's phone: the seller cannot join yet.
	e.verifyCustomerContact(t, account, tax, domain.ChannelTypeEmail, "customer"+tax+"@example.test", true)
	caseID := e.pendingCaseID(t, account, typed, registered)
	resolve := func() *httptest.ResponseRecorder {
		return e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + caseID.String(), token: seller, branch: seed.tenant.BranchID.String(), body: map[string]any{"action": "MERGE"}})
	}
	if rec := resolve(); rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeClientContactUnproved) {
		t.Fatalf("join before the proof: %d %s, want 409 %s", rec.Code, rec.Body, domain.CodeClientContactUnproved)
	}
	if e.joinReady(t, seller, seed.tenant.BranchID, caseID) {
		t.Fatal("the case reads ready to join before the customer proved the contact")
	}
	e.verifyCustomerContact(t, account, tax, domain.ChannelTypeWhatsApp, "+5492215550303", true)
	if got := e.pendingMatches(t, account, "+5492215550303"); len(got) != 1 {
		t.Fatalf("the proof raised %v beside the pending case, want still one", got)
	}
	if !e.joinReady(t, seller, seed.tenant.BranchID, caseID) {
		t.Fatal("the case does not read ready to join after the customer proved the contact")
	}
	if rec := resolve(); rec.Code != http.StatusNoContent {
		t.Fatalf("join after the proof: %d %s, want 204", rec.Code, rec.Body)
	}
	var into *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT merged_into FROM client WHERE id=$1`, typed).Scan(&into); err != nil || into == nil || *into != registered {
		t.Fatalf("typed profile merged into %v (%v), want the account %s", into, err, registered)
	}
}

// pendingCaseID returns the pending case from source to candidate.
func (e *env) pendingCaseID(t *testing.T, account, source, candidate uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM client_contact_observation WHERE account_id=$1 AND source_client_id=$2 AND candidate_client_id=$3 AND status='PENDING'`, account, source, candidate).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// joinReady reads whether the suggestion list offers joining the case into the customer's account.
func (e *env) joinReady(t *testing.T, token string, branch, caseID uuid.UUID) bool {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/client-contact-suggestions", token: token, branch: branch.String()})
	var list []struct {
		ID    uuid.UUID `json:"id"`
		Ready bool      `json:"account_join_ready"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("suggestions: %d %s", rec.Code, rec.Body)
	}
	for _, item := range list {
		if item.ID == caseID {
			return item.Ready
		}
	}
	t.Fatalf("suggestions = %s, want case %s", rec.Body, caseID)
	return false
}

func TestClientContactMatching_AQuoteReplyProvesTheTypedContact(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Reply proves contact")
	channel := e.seedWhatsAppInbox(t, account, branch)
	name, phone := "Obra Palermo", domain.CanonicalPhone("+54 11 5555-0707")
	var typed uuid.UUID
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
		c, err := repository.NewClientRepository().Create(ctx, q, account, domain.NewClient{Name: &name, Phone: &phone})
		if err == nil {
			typed = c.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	quote := e.seedSentQuote(t, sentQuote{accountID: account, branchID: branch, channel: channel, destination: "+541155550707",
		status: domain.QuoteStatusSent, sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET client_id=$2 WHERE id=$1`, quote, typed); err != nil {
		t.Fatal(err)
	}
	profiles := func() int {
		var n int
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM client WHERE account_id=$1`, account).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := profiles()

	if err := e.inboundService(t, &recordingInboundRFQs{}).Ingest(ctx, []domain.InboundTextMessage{
		whatsAppReply(channel, "wamid."+uuid.NewString(), "5491155550707", "sumale 5 bolsas")}); err != nil {
		t.Fatal(err)
	}
	if messages, _ := e.conversationOf(t, quote); messages != 1 {
		t.Fatalf("quote messages = %d, want the reply in the conversation", messages)
	}
	if got := e.contactsOf(t, typed); len(got) != 1 || got[0] != (contactState{"+5491155550707", "SELLER", true}) {
		t.Fatalf("typed contact after the reply = %+v, want it verified by the channel", got)
	}
	if after := profiles(); after != before {
		t.Fatalf("profiles %d -> %d: the reply opened a provisional profile", before, after)
	}
	if got := e.pendingMatches(t, account, "+5491155550707"); len(got) != 0 {
		t.Fatalf("the reply raised cases %v, want none", got)
	}
}

func TestClientContactMatching_AccountJoinKeepsToTheSellersReach(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	const tax, shared = "20123456786", "shared-join@example.test"
	account := m.registered(t, "Cuenta", "cuenta@example.test", tax)
	var customer uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, m.account, tax).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `WITH proved AS (INSERT INTO client_contact(account_id,client_id,kind,value,source,verified_at) VALUES($1,$2,'EMAIL',$3,'CUSTOMER',now()) RETURNING id)
 INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history) SELECT $1,$4,id,TRUE FROM proved`, m.account, account, shared, customer); err != nil {
		t.Fatal(err)
	}
	observe := func(provisional, branch uuid.UUID) uuid.UUID {
		var id uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,'EMAIL',$5::varchar,$5::text,'PENDING') RETURNING id`, m.account, branch, provisional, account, shared).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	resolve := func(id uuid.UUID) (int, string) {
		rec := e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + id.String(), token: m.token, body: map[string]any{"action": "MERGE"}})
		if rec.Code == http.StatusNoContent {
			return rec.Code, ""
		}
		return rec.Code, errorCode(t, rec)
	}

	elsewhere := m.profile(t, "Pedidos en otra sucursal", "elsewhere-join@example.test", nil)
	m.order(t, elsewhere, m.other)
	if status, code := resolve(observe(elsewhere, m.branch)); status != http.StatusForbidden || code != string(domain.CodeClientOutOfReach) {
		t.Fatalf("join a profile with orders in another branch: %d %s, want 403 %s", status, code, domain.CodeClientOutOfReach)
	}
	hidden := m.profile(t, "Caso de otra sucursal", "hidden-join@example.test", nil)
	if status, _ := resolve(observe(hidden, m.other)); status != http.StatusNotFound {
		t.Fatalf("resolve another branch's case: %d, want 404", status)
	}
	reachable := m.profile(t, "Mostrador", "reachable-join@example.test", nil)
	m.order(t, reachable, m.branch)
	reachableCase := observe(reachable, m.branch)
	// The account always survives: keeping the provisional side is refused.
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + reachableCase.String(), token: m.token, body: map[string]any{"action": "MERGE", "keep_client_id": reachable}})
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != string(domain.CodeClientRegistered) {
		t.Fatalf("keep the provisional side: %d %s, want 403 %s", rec.Code, rec.Body, domain.CodeClientRegistered)
	}
	if status, code := resolve(reachableCase); status != http.StatusNoContent {
		t.Fatalf("join a reachable profile: %d %s, want 204", status, code)
	}
	if into := m.mergedInto(t, reachable); into == nil || *into != account {
		t.Fatalf("reachable merged into %v, want the account %s", into, account)
	}
	if count, by := m.audits(t, reachable, account); count != 1 || by == nil || *by != m.seller.ID {
		t.Fatalf("audit = %d by %v, want one by the seller", count, by)
	}
	for _, id := range []uuid.UUID{elsewhere, hidden} {
		if into := m.mergedInto(t, id); into != nil {
			t.Fatalf("refused profile %s merged into %s", id, *into)
		}
	}
}

func TestSellerMerge_ACUITWithoutAccountMergesAndSurvives(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	cuit := "20123456786"
	fiscal := m.profile(t, "Con CUIT", "fiscal@example.test", &cuit)
	dumb := m.profile(t, "Mostrador", "mostrador@example.test", nil)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET notes=CASE id WHEN $1 THEN 'Paga a 30 días' ELSE 'Retira por la tarde' END WHERE id IN ($1,$2)`, fiscal, dumb); err != nil {
		t.Fatal(err)
	}

	rec := e.do(t, request{method: http.MethodGet, path: "/v1/clients/" + dumb.String() + "/merge-candidates", token: m.token})
	if rec.Code != http.StatusOK || !json.Valid(rec.Body.Bytes()) || !containsID(t, rec.Body.Bytes(), fiscal) {
		t.Fatalf("merge candidates: %d %s, want the CUIT profile without an account offered", rec.Code, rec.Body)
	}
	// The seller merges the CUIT profile into the provisional one; the CUIT side survives instead.
	if status, code := m.merge(t, dumb, fiscal); status != http.StatusNoContent {
		t.Fatalf("merge: status = %d (%s), want 204", status, code)
	}
	if into := m.mergedInto(t, dumb); into == nil || *into != fiscal {
		t.Fatalf("provisional merged_into = %v, want the CUIT profile %s", into, fiscal)
	}
	var notes string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT notes FROM client WHERE id=$1`, fiscal).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if notes != "Paga a 30 días\n\nRetira por la tarde" {
		t.Fatalf("survivor notes = %q, want both profiles' notes", notes)
	}

	other := "27111111110"
	second := m.profile(t, "Otro CUIT", "otro-cuit@example.test", &other)
	if status, code := m.merge(t, fiscal, second); status != http.StatusConflict || code != string(domain.CodeClientTaxIDConflict) {
		t.Fatalf("merge two CUITs: status = %d (%s), want 409 %s", status, code, domain.CodeClientTaxIDConflict)
	}
}

func containsID(t *testing.T, body []byte, id uuid.UUID) bool {
	t.Helper()
	var list struct {
		Items []struct {
			ID uuid.UUID `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestClientContacts_RegistrationLeavesUnprovedContactsOffTheProfile(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Registration projection")
	const tax = "30712345671"
	e.registerCustomer(t, account, tax)
	email := func() *string {
		var value *string
		if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT email FROM client WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if got := email(); got != nil {
		t.Fatalf("registration projected the unverified %q into client.email", *got)
	}
	e.verifyCustomerContact(t, account, tax, domain.ChannelTypeEmail, "customer"+tax+"@example.test", true)
	if got := email(); got == nil || *got != "customer"+tax+"@example.test" {
		t.Fatalf("client.email after proof = %v, want the proved address", got)
	}
}
