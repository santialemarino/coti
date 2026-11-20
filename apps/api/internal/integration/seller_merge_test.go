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

// sellerMerge is an account with two branches and a seller assigned only to the first.
type sellerMerge struct {
	e       *env
	account uuid.UUID
	branch  uuid.UUID
	other   uuid.UUID
	channel uuid.UUID
	seller  domain.AppUser
	token   string
}

func (e *env) sellerMerge(t *testing.T) *sellerMerge {
	t.Helper()
	account, branch := e.seedAccount(t, "Seller merge")
	m := &sellerMerge{e: e, account: account, branch: branch, other: e.seedBranch(t, account, "Seller merge Sur")}
	m.channel = e.historyChannel(t, account, branch, "EMAIL")
	m.seller = e.seedUser(t, account, domain.UserRoleSeller)
	e.assignBranch(t, account, m.seller, branch)
	m.token = e.tokenFor(t, m.seller)
	return m
}

// profile seeds a live profile reached by one channel-verified email, with an optional CUIT.
func (m *sellerMerge) profile(t *testing.T, name, email string, taxID *string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client(account_id,name,tax_id) VALUES($1,$2,$3) RETURNING id`, m.account, name, taxID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
		contacts := repository.NewClientContactRepository()
		if err := contacts.Save(ctx, q, m.account, id, []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: email, Primary: true}}, "CHANNEL"); err != nil {
			return err
		}
		return contacts.ProjectPrimaries(ctx, q, m.account, id)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// registered seeds a profile whose CUIT has a customer account.
func (m *sellerMerge) registered(t *testing.T, name, email, taxID string) uuid.UUID {
	t.Helper()
	id := m.profile(t, name, email, &taxID)
	if _, err := m.e.db.CrossAccount().Exec(context.Background(), `INSERT INTO customer_identity(account_id,tax_id,password_hash,name) VALUES($1,$2,'unused',$3)`, m.account, taxID, name); err != nil {
		t.Fatal(err)
	}
	return id
}

// order seeds a request of the profile in a branch.
func (m *sellerMerge) order(t *testing.T, clientID, branch uuid.UUID) uuid.UUID {
	t.Helper()
	channel := m.channel
	if branch != m.branch {
		channel = m.e.historyChannel(t, m.account, branch, "EMAIL")
	}
	var id uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text,client_id) VALUES($1,$2,$3,'Seller merge order',$4) RETURNING id`, m.account, branch, channel, clientID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (m *sellerMerge) merge(t *testing.T, target, source uuid.UUID) (int, string) {
	t.Helper()
	rec := m.e.do(t, request{method: http.MethodPost, path: "/v1/clients/" + target.String() + "/merge", token: m.token, body: map[string]any{"source_client_id": source}})
	if rec.Code == http.StatusOK {
		return rec.Code, ""
	}
	return rec.Code, errorCode(t, rec)
}

func (m *sellerMerge) mergedInto(t *testing.T, id uuid.UUID) *uuid.UUID {
	t.Helper()
	var into *uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `SELECT merged_into FROM client WHERE id=$1`, id).Scan(&into); err != nil {
		t.Fatal(err)
	}
	return into
}

func (m *sellerMerge) audits(t *testing.T, source, target uuid.UUID) (count int, by *uuid.UUID) {
	t.Helper()
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*),max(merged_by::text)::uuid FROM client_merge_audit WHERE account_id=$1 AND source_client_id=$2 AND target_client_id=$3`, m.account, source, target).Scan(&count, &by); err != nil {
		t.Fatal(err)
	}
	return count, by
}

func TestSellerMerge_RenameOnlyAProfileWithoutCustomerAccount(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	cuit, cuit2 := "20123456786", "27111111110"
	dumb := m.profile(t, "🔨 Juanchi", "juanchi@example.test", nil)
	registered := m.registered(t, "Registered Buyer", "registered@example.test", cuit)
	fiscalOnly := m.profile(t, "CUIT sin cuenta", "fiscal-only@example.test", &cuit2)
	rename := func(id uuid.UUID, name string) *http.Response {
		rec := e.do(t, request{method: http.MethodPut, path: "/v1/clients/" + id.String() + "/name", token: m.token, body: map[string]any{"name": name}})
		return rec.Result()
	}

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/clients/" + dumb.String() + "/name", token: m.token, body: map[string]any{"name": "  Juan Pérez  "}})
	var body struct {
		Name string `json:"name"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Name != "Juan Pérez" {
		t.Fatalf("rename a provisional profile: status = %d, body = %s, want 200 with the trimmed name", rec.Code, rec.Body)
	}

	rec = e.do(t, request{method: http.MethodPut, path: "/v1/clients/" + registered.String() + "/name", token: m.token, body: map[string]any{"name": "Someone Else"}})
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != string(domain.CodeClientRegistered) {
		t.Fatalf("rename a registered profile: status = %d, body = %s, want 403 CLIENT_REGISTERED", rec.Code, rec.Body)
	}
	if got := e.profileNameOf(t, registered); got != "Registered Buyer" {
		t.Fatalf("a refused rename changed the registered name to %q", got)
	}
	// A CUIT a seller recorded without a customer account is still the seller's to correct.
	if res := rename(fiscalOnly, "CUIT corregido"); res.StatusCode != http.StatusOK {
		t.Fatalf("rename a profile whose CUIT has no account: status = %d, want 200", res.StatusCode)
	}

	foreignAccount, _ := e.seedAccount(t, "Seller merge vecino")
	foreign := (&sellerMerge{e: e, account: foreignAccount}).profile(t, "Foreign", "foreign@example.test", nil)
	if res := rename(foreign, "Mine now"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("rename another account's profile: status = %d, want 404", res.StatusCode)
	}
	if res := rename(dumb, " "); res.StatusCode != http.StatusUnprocessableEntity && res.StatusCode != http.StatusBadRequest {
		t.Fatalf("rename to a blank name: status = %d, want it refused", res.StatusCode)
	}
}

func (e *env) profileNameOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var name string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT name FROM client WHERE id=$1`, id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestSellerMerge_MergesTwoProvisionalProfiles(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	target := m.profile(t, "Counter buyer", "counter@example.test", nil)
	source := m.profile(t, "Counter buyer (second mail)", "counter-two@example.test", nil)
	moved := m.order(t, source, m.branch)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET dni='28123456',legal_name='Source SA',fiscal_address='Calle 1' WHERE id=$1`, source); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET legal_name='Target SRL' WHERE id=$1`, target); err != nil {
		t.Fatal(err)
	}

	if status, code := m.merge(t, target, source); status != http.StatusOK {
		t.Fatalf("merge: status = %d (%s), want 200", status, code)
	}
	if status, code := m.merge(t, target, source); status != http.StatusOK {
		t.Fatalf("a retried merge: status = %d (%s), want 200", status, code)
	}
	if into := m.mergedInto(t, source); into == nil || *into != target {
		t.Fatalf("source merged_into = %v, want %s", into, target)
	}
	var orderClient uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT client_id FROM rfq WHERE id=$1`, moved).Scan(&orderClient); err != nil || orderClient != target {
		t.Fatalf("order client = %s (%v), want the surviving profile %s", orderClient, err, target)
	}
	if count, by := m.audits(t, source, target); count != 1 || by == nil || *by != m.seller.ID {
		t.Fatalf("audit rows = %d by %v, want one by the seller %s", count, by, m.seller.ID)
	}
	// The DNI and fiscal data fill what the survivor lacked and never overwrite what it had.
	row := e.clientFiscal(t, target)
	if text(row.dni) != "28123456" || text(row.address) != "Calle 1" || text(row.legalName) != "Target SRL" {
		t.Fatalf("survivor fiscal = %s, want the source's DNI and address and its own legal name", row.String())
	}

	// The merged profile's contact keeps routing to the survivor instead of opening a new profile.
	route := domain.InboundChannelRoute{AccountID: m.account, BranchID: m.branch, ChannelID: m.channel, ChannelType: domain.ChannelTypeEmail}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	messenger := &captureContactMessenger{}
	service.WithMessaging(messenger)
	inbound := &mergedRouting{e: e, account: m.account, route: route, service: service, messenger: messenger}
	before := inbound.liveProfiles(t)
	if got := inbound.requestClient(t, inbound.deliver(t, "counter-two@example.test")); got != target {
		t.Fatalf("an order from the merged contact went to %s, want the survivor %s", got, target)
	}
	if after := inbound.liveProfiles(t); after != before {
		t.Fatalf("live profiles %d -> %d: the merged contact opened a new profile", before, after)
	}

	// An old link to the merged profile resolves to the survivor.
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/clients/" + source.String(), token: m.token})
	var profile struct {
		Client struct {
			ID uuid.UUID `json:"id"`
		} `json:"client"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &profile) != nil || profile.Client.ID != target {
		t.Fatalf("GET the merged id: status = %d, body = %s, want 200 with the survivor %s", rec.Code, rec.Body, target)
	}

	// A chain stays one hop: merging the survivor again repoints the profile merged into it.
	third := m.profile(t, "Counter buyer (third)", "counter-three@example.test", nil)
	if status, code := m.merge(t, third, target); status != http.StatusOK {
		t.Fatalf("second merge: status = %d (%s), want 200", status, code)
	}
	if into := m.mergedInto(t, source); into == nil || *into != third {
		t.Fatalf("the first source now merged_into = %v, want the latest survivor %s", into, third)
	}
	if got := inbound.requestClient(t, inbound.deliver(t, "counter-two@example.test")); got != third {
		t.Fatalf("after two merges the first contact routed to %s, want %s", got, third)
	}
}

func TestSellerMerge_RefusesWhatNoSellerMerges(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	cuit := "20123456786"
	target := m.profile(t, "Target", "target@example.test", nil)
	registered := m.registered(t, "Registered", "registered@example.test", cuit)
	elsewhere := m.profile(t, "Other branch buyer", "elsewhere@example.test", nil)
	m.order(t, elsewhere, m.other)
	merged := m.profile(t, "Already merged", "merged@example.test", nil)
	elsewhereSurvivor := m.profile(t, "Other survivor", "other-survivor@example.test", nil)
	if status, code := m.merge(t, elsewhereSurvivor, merged); status != http.StatusOK {
		t.Fatalf("seed merge: status = %d (%s)", status, code)
	}
	otherRegistered := m.registered(t, "Other registered", "other-registered@example.test", "27111111110")
	foreignAccount, _ := e.seedAccount(t, "Seller merge ajeno")
	foreign := (&sellerMerge{e: e, account: foreignAccount}).profile(t, "Foreign", "foreign@example.test", nil)

	cases := []struct {
		name           string
		target, source uuid.UUID
		status         int
		code           domain.ErrorCode
	}{
		{"two customer accounts", registered, otherRegistered, http.StatusForbidden, domain.CodeClientRegistered},
		{"order in another branch", target, elsewhere, http.StatusForbidden, domain.CodeClientOutOfReach},
		{"another account", target, foreign, http.StatusNotFound, domain.CodeNotFound},
		{"already merged", target, merged, http.StatusConflict, domain.CodeClientMerged},
	}
	for _, tc := range cases {
		if status, code := m.merge(t, tc.target, tc.source); status != tc.status || code != string(tc.code) {
			t.Errorf("%s: status = %d (%s), want %d (%s)", tc.name, status, code, tc.status, tc.code)
		}
	}
	for _, id := range []uuid.UUID{registered, otherRegistered, elsewhere, foreign} {
		if into := m.mergedInto(t, id); into != nil {
			t.Errorf("refused profile %s was merged into %s", id, *into)
		}
	}

	// The candidates offered are only the ones a merge would accept.
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/clients/" + target.String() + "/merge-candidates", token: m.token})
	var list struct {
		Items []struct {
			ID uuid.UUID `json:"id"`
		} `json:"items"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("merge candidates: status = %d, body = %s", rec.Code, rec.Body)
	}
	offersAccount := false
	for _, item := range list.Items {
		if item.ID == elsewhere || item.ID == merged || item.ID == foreign || item.ID == target {
			t.Fatalf("merge candidates offered %s, which no merge would accept", item.ID)
		}
		offersAccount = offersAccount || item.ID == registered
	}
	if !offersAccount {
		t.Fatal("a profile without a CUIT must be offered the customer accounts it can merge into")
	}
}

// A seller merges a profile into a customer's account: the seller sees the history together at once,
// the customer's portal shows the merged orders only after the customer proves a merged contact.
func TestSellerMerge_IntoCustomerAccountHoldsThePortal(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	cuit := "20123456786"
	account := m.registered(t, "Cliente Web", "web@example.test", cuit)
	provisional := m.profile(t, "Cliente WhatsApp", "wa@example.test", nil)
	order := m.order(t, provisional, m.branch)
	var customer uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, m.account, cuit).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history)
 SELECT account_id,$2,id,TRUE FROM client_contact WHERE account_id=$1 AND client_id=$3 AND retired_at IS NULL`, m.account, customer, account); err != nil {
		t.Fatal(err)
	}
	visible := func() bool {
		t.Helper()
		var found bool
		if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
			orders, err := repository.NewCustomerOrderRepository().List(ctx, q, m.account, customer, 50, 0)
			for _, o := range orders {
				found = found || o.ID == order
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return found
	}

	// The seller names the account as the one merged away; the account survives anyway.
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/clients/" + provisional.String() + "/merge", token: m.token, body: map[string]any{"source_client_id": account}})
	var body struct {
		Survivor uuid.UUID `json:"survivor_client_id"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Survivor != account {
		t.Fatalf("merge into the account: %d %s, want 200 naming %s", rec.Code, rec.Body, account)
	}
	var owner uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT client_id FROM rfq WHERE id=$1`, order).Scan(&owner); err != nil || owner != account {
		t.Fatalf("the order belongs to %s (%v), want the account %s", owner, err, account)
	}
	var verified, primary bool
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT verified_at IS NOT NULL,is_primary FROM client_contact WHERE client_id=$1 AND value='wa@example.test' AND retired_at IS NULL`, account).Scan(&verified, &primary); err != nil || verified || primary {
		t.Fatalf("the merged contact arrived verified=%v primary=%v (%v), want neither", verified, primary, err)
	}
	if visible() {
		t.Fatal("the merged order reached the customer's portal before any proof")
	}
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
		contacts := repository.NewClientContactRepository()
		if err := contacts.ConfirmCustomerProof(ctx, q, m.account, account, domain.ChannelTypeEmail, "other@example.test"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if visible() {
		t.Fatal("proving an unrelated contact released the merged orders")
	}
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
		return repository.NewClientContactRepository().ConfirmCustomerProof(ctx, q, m.account, account, domain.ChannelTypeEmail, "wa@example.test")
	}); err != nil {
		t.Fatal(err)
	}
	if !visible() {
		t.Fatal("the customer proved the merged contact but still cannot see the merged order")
	}
}

func TestSellerMerge_ResolvesPendingObservations(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	ctx := context.Background()
	observe := func(loser, owner uuid.UUID, value string) uuid.UUID {
		var id uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status) VALUES($1,$2,$3,$4,'EMAIL',$5::varchar,$5::text,'PENDING') RETURNING id`, m.account, m.branch, loser, owner, value).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	resolve := func(id uuid.UUID, body map[string]any) int {
		return e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + id.String(), token: m.token, body: body}).Code
	}
	status := func(id uuid.UUID) (string, *uuid.UUID) {
		var s string
		var by *uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT status::text,decided_by FROM client_contact_observation WHERE id=$1`, id).Scan(&s, &by); err != nil {
			t.Fatal(err)
		}
		return s, by
	}

	owner := m.profile(t, "Owner", "owner@example.test", nil)
	loser := m.profile(t, "Loser", "loser@example.test", nil)
	merge := observe(loser, owner, "owner@example.test")
	if code := resolve(merge, map[string]any{"action": "MERGE"}); code != http.StatusNoContent {
		t.Fatalf("resolve MERGE: status = %d, want 204", code)
	}
	if into := m.mergedInto(t, loser); into == nil || *into != owner {
		t.Fatalf("loser merged_into = %v, want the owner %s", into, owner)
	}
	if s, by := status(merge); s != "MERGED" || by == nil || *by != m.seller.ID {
		t.Fatalf("observation = %s by %v, want MERGED by the seller", s, by)
	}
	if code := resolve(merge, map[string]any{"action": "SEPARATE"}); code != http.StatusConflict {
		t.Fatalf("resolving a decided observation: status = %d, want 409", code)
	}

	kept := m.profile(t, "Kept", "kept@example.test", nil)
	folded := m.profile(t, "Folded", "folded@example.test", nil)
	pick := observe(kept, folded, "folded@example.test")
	if code := resolve(pick, map[string]any{"action": "MERGE", "keep_client_id": kept}); code != http.StatusNoContent {
		t.Fatalf("resolve MERGE keeping the source: status = %d, want 204", code)
	}
	if into := m.mergedInto(t, folded); into == nil || *into != kept {
		t.Fatalf("candidate merged_into = %v, want the source the seller kept %s", into, kept)
	}

	a := m.profile(t, "Separate A", "separate-a@example.test", nil)
	b := m.profile(t, "Separate B", "separate-b@example.test", nil)
	dismiss := observe(a, b, "separate-b@example.test")
	if code := resolve(dismiss, map[string]any{"action": "SEPARATE"}); code != http.StatusNoContent {
		t.Fatalf("resolve SEPARATE: status = %d, want 204", code)
	}
	if s, _ := status(dismiss); s != "SEPARATE" || m.mergedInto(t, a) != nil || m.mergedInto(t, b) != nil {
		t.Fatalf("dismiss: observation %s, want SEPARATE and both profiles live", s)
	}

	// Both sides already merged into one survivor: the case settles with no merge of its own.
	left := m.profile(t, "Shared left", "shared-left@example.test", nil)
	right := m.profile(t, "Shared right", "shared-right@example.test", nil)
	survivor := m.profile(t, "Shared survivor", "shared-survivor@example.test", nil)
	shared := observe(left, right, "shared-right@example.test")
	for _, source := range []uuid.UUID{left, right} {
		if code, errCode := m.merge(t, survivor, source); code != http.StatusOK {
			t.Fatalf("seed merge into the survivor: %d %s", code, errCode)
		}
	}
	if code := resolve(shared, map[string]any{"action": "MERGE"}); code != http.StatusNoContent {
		t.Fatalf("resolve MERGE on a shared survivor: status = %d, want 204", code)
	}
	if s, _ := status(shared); s != "MERGED" {
		t.Fatalf("shared-survivor observation = %s, want MERGED", s)
	}
	if into := m.mergedInto(t, survivor); into != nil {
		t.Fatalf("the shared survivor was merged into %s", *into)
	}
	if n, _ := m.audits(t, survivor, survivor); n != 0 {
		t.Fatalf("a survivor merged into itself %d times", n)
	}

	// A side whose survivor now has a customer account joins it only on a contact its customer proved.
	provisional := m.profile(t, "Still provisional", "still-provisional@example.test", nil)
	adopted := m.profile(t, "Adopted", "adopted@example.test", nil)
	registered := m.registered(t, "Registered", "registered@example.test", "20123456786")
	refused := observe(provisional, adopted, "adopted@example.test")
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client SET merged_into=$2 WHERE id=$1`, adopted, registered); err != nil {
		t.Fatal(err)
	}
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/client-contact-suggestions/" + refused.String(), token: m.token, body: map[string]any{"action": "MERGE"}})
	if rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeClientContactUnproved) {
		t.Fatalf("resolve MERGE into a registered survivor: %d %s, want 409 %s", rec.Code, rec.Body, domain.CodeClientContactUnproved)
	}
	if s, _ := status(refused); s != "PENDING" || m.mergedInto(t, provisional) != nil {
		t.Fatalf("refused case: observation %s, provisional merged into %v; want PENDING and live", s, m.mergedInto(t, provisional))
	}
}
