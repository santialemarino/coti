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
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO client(account_id,name,email,tax_id) VALUES($1,$2,$3,$4) RETURNING id`, m.account, name, email, taxID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.InTenantTx(ctx, domain.Tenant{AccountID: m.account}, func(q repository.Querier) error {
		return repository.NewClientContactRepository().Save(ctx, q, m.account, id, []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: email, Primary: true}}, "CHANNEL")
	}); err != nil {
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
	if rec.Code == http.StatusNoContent {
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

func TestSellerMerge_RenameOnlyAProfileWithoutCUIT(t *testing.T) {
	e := newEnv(t)
	m := e.sellerMerge(t)
	cuit := "20123456786"
	dumb := m.profile(t, "🔨 Juanchi", "juanchi@example.test", nil)
	registered := m.profile(t, "Registered Buyer", "registered@example.test", &cuit)
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

	if status, code := m.merge(t, target, source); status != http.StatusNoContent {
		t.Fatalf("merge: status = %d (%s), want 204", status, code)
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
	if status, code := m.merge(t, third, target); status != http.StatusNoContent {
		t.Fatalf("second merge: status = %d (%s), want 204", status, code)
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
	registered := m.profile(t, "Registered", "registered@example.test", &cuit)
	elsewhere := m.profile(t, "Other branch buyer", "elsewhere@example.test", nil)
	m.order(t, elsewhere, m.other)
	merged := m.profile(t, "Already merged", "merged@example.test", nil)
	if status, code := m.merge(t, target, merged); status != http.StatusNoContent {
		t.Fatalf("seed merge: status = %d (%s)", status, code)
	}
	foreignAccount, _ := e.seedAccount(t, "Seller merge ajeno")
	foreign := (&sellerMerge{e: e, account: foreignAccount}).profile(t, "Foreign", "foreign@example.test", nil)

	cases := []struct {
		name           string
		target, source uuid.UUID
		status         int
		code           domain.ErrorCode
	}{
		{"registered source", target, registered, http.StatusForbidden, domain.CodeClientRegistered},
		{"registered target", registered, target, http.StatusForbidden, domain.CodeClientRegistered},
		{"order in another branch", target, elsewhere, http.StatusForbidden, domain.CodeClientOutOfReach},
		{"another account", target, foreign, http.StatusNotFound, domain.CodeNotFound},
		{"already merged", target, merged, http.StatusConflict, domain.CodeClientMerged},
	}
	for _, tc := range cases {
		if status, code := m.merge(t, tc.target, tc.source); status != tc.status || code != string(tc.code) {
			t.Errorf("%s: status = %d (%s), want %d (%s)", tc.name, status, code, tc.status, tc.code)
		}
	}
	for _, id := range []uuid.UUID{registered, elsewhere, foreign} {
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
	for _, item := range list.Items {
		if item.ID == registered || item.ID == elsewhere || item.ID == merged || item.ID == foreign || item.ID == target {
			t.Fatalf("merge candidates offered %s, which no merge would accept", item.ID)
		}
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
}
