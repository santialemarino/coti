//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestClientContacts_LocksAllowIndependentContacts(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Contact lock concurrency")
	other, _ := e.seedAccount(t, "Contact lock isolation")
	store := repository.NewClientContactRepository()
	ctx := context.Background()
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- e.db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
			if err := store.Lock(ctx, q, account, "contact:EMAIL:first@example.test"); err != nil {
				close(held)
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	t.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	for _, scope := range []struct {
		account uuid.UUID
		key     string
	}{{account, "contact:EMAIL:second@example.test"}, {other, "contact:EMAIL:first@example.test"}} {
		deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := e.db.InTenantTx(deadline, domain.Tenant{AccountID: scope.account}, func(q repository.Querier) error {
			return store.Lock(deadline, q, scope.account, scope.key)
		})
		cancel()
		if err != nil {
			t.Fatalf("unrelated contact or supplier was blocked: %v", err)
		}
	}
	deadline, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	err := e.db.InTenantTx(deadline, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
		return store.Lock(deadline, q, account, "contact:EMAIL:first@example.test")
	})
	if err == nil {
		t.Fatal("concurrent writes for the same contact were not serialized")
	}
}

func TestCustomerProfile_SavesOwnContactsAndKeepsRecoverySeparate(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Customer contacts")
	other, _ := e.seedAccount(t, "Other customer contacts")
	host, token := e.registerCustomer(t, account, "20123456786", "recovery@example.test")
	otherHost, otherToken := e.registerCustomer(t, other, "20123456786", "")
	base := "/v1/customer/sites/" + host + "/profile"
	input := map[string]any{"name": "Customer", "client_id": uuid.New(), "tax_id": "30712345671", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": " COMMERCIAL@EXAMPLE.TEST ", "primary": true},
		{"kind": "EMAIL", "value": "other@example.test", "primary": false},
		{"kind": "WHATSAPP", "value": "+54 9 (11) 5555-0101", "primary": true},
	}}
	for range 2 {
		response := e.do(t, request{method: http.MethodPut, path: base, token: token, body: input})
		if response.Code != 204 {
			t.Fatalf("save: %d %s", response.Code, response.Body)
		}
	}
	var count int
	var recovery string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("retry contacts=%d", count)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT recovery_email FROM customer_identity WHERE account_id=$1`, account).Scan(&recovery); err != nil {
		t.Fatal(err)
	}
	if recovery != "recovery@example.test" {
		t.Fatal("commercial update changed recovery")
	}
	response := e.do(t, request{method: http.MethodGet, path: base, token: token})
	var result struct {
		TaxID    string `json:"tax_id"`
		Contacts []struct {
			Kind    string `json:"kind"`
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"contacts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.TaxID != "20123456786" || len(result.Contacts) != 3 {
		t.Fatalf("own profile: %d %s", response.Code, response.Body)
	}
	response = e.do(t, request{method: http.MethodGet, path: base, token: otherToken})
	if response.Code != 401 {
		t.Fatalf("foreign supplier token: %d", response.Code)
	}
	response = e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + otherHost + "/profile", token: otherToken})
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	var foreign struct {
		Contacts []any `json:"contacts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &foreign); err != nil {
		t.Fatal(err)
	}
	if len(foreign.Contacts) != 1 {
		t.Fatal("contacts crossed suppliers with the same CUIT")
	}
	invalid := map[string]any{"contacts": []map[string]any{{"kind": "EMAIL", "value": "one@example.test", "primary": false}}}
	response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: invalid})
	if response.Code != 422 {
		t.Fatalf("missing primary: %d %s", response.Code, response.Body)
	}
	response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: map[string]any{"name": "Customer", "contacts": []any{}}})
	if response.Code != 422 {
		t.Fatal(response.Body)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client_contact WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatal("an invalid save removed the last required contact")
	}
}

func TestClientContacts_UnknownSendersStartWithDisplayNameAndNeverClaimCUIT(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Contact onboarding")
	ctx := context.Background()
	store := repository.NewClientContactRepository()
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), store)
	messenger := &captureContactMessenger{}
	service.WithMessaging(messenger)
	var channel uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL',$3) RETURNING id`, account, branch, uuid.NewString()+"@example.test").Scan(&channel); err != nil {
		t.Fatal(err)
	}
	route := domain.InboundChannelRoute{AccountID: account, BranchID: branch, ChannelID: channel, ChannelType: domain.ChannelTypeEmail}
	value := "unknown@example.test"
	taxID := "20123456786"
	message := uuid.New()
	record := func(id uuid.UUID, hints domain.ClientContactHints) {
		t.Helper()
		if err := e.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			return service.RecordInbound(ctx, q, route, id, domain.ChannelTypeEmail, value, "Retained original", hints)
		}); err != nil {
			t.Fatal(err)
		}
	}
	displayName := "WhatsApp Profile"
	record(message, domain.ClientContactHints{TaxID: &taxID, DisplayName: &displayName})
	record(message, domain.ClientContactHints{TaxID: &taxID})
	for range 2 {
		if err := service.CompleteInbound(ctx, route, domain.ChannelTypeEmail, value); err != nil {
			t.Fatal(err)
		}
	}
	if len(messenger.messages) != 1 {
		t.Fatalf("name prompts=%d", len(messenger.messages))
	}
	var count int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM client WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("an unknown sender did not receive one provisional profile")
	}
	var initialName string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT name FROM client WHERE account_id=$1`, account).Scan(&initialName); err != nil {
		t.Fatal(err)
	}
	if initialName != displayName {
		t.Fatalf("provisional name=%q", initialName)
	}
	name := "Juan Perez"
	record(uuid.New(), domain.ClientContactHints{Name: &name, TaxID: &taxID})
	if err := service.CompleteInbound(ctx, route, domain.ChannelTypeEmail, value); err != nil {
		t.Fatal(err)
	}
	var storedName string
	var storedTax *string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT name,tax_id FROM client WHERE account_id=$1 AND merged_into IS NULL`, account).Scan(&storedName, &storedTax); err != nil {
		t.Fatal(err)
	}
	if storedName != name || storedTax != nil || len(messenger.messages) != 1 {
		t.Fatal("onboarding did not retain a name-only provisional identity")
	}
	changed := "Unrequested Rename"
	record(uuid.New(), domain.ClientContactHints{Name: &changed})
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT name FROM client WHERE account_id=$1`, account).Scan(&storedName); err != nil {
		t.Fatal(err)
	}
	if storedName != name {
		t.Fatalf("later message overwrote confirmed name: %q", storedName)
	}
	other, _ := e.seedAccount(t, "Contact isolation")
	err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: other}, func(q repository.Querier) error {
		rows, err := store.FindProfiles(ctx, q, other, "", domain.ChannelTypeEmail, value)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatal("contact crossed suppliers")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientContacts_VerifiedMergePreservesHistoryAndRegisteredName(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Verified merge", false)
	ctx := context.Background()
	account := seed.tenant.AccountID
	profiles := repository.NewClientRepository()
	store := repository.NewClientContactRepository()
	service := services.NewClientContactService(e.db, profiles, store)
	name, email := "Provisional Name", "provisional@example.test"
	var source uuid.UUID
	err := e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
		c, err := profiles.Create(ctx, q, account, domain.NewClient{Name: &name, Email: &email})
		if err != nil {
			return err
		}
		source = c.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET client_id=$2 WHERE account_id=$3 AND id=$1`, seed.draft.Quote.ID, source, account); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE rfq SET client_id=$2 WHERE account_id=$3 AND id=$1`, seed.draft.RFQ.ID, source, account); err != nil {
		t.Fatal(err)
	}
	host, token := e.registerCustomer(t, account, "20123456786", "")
	_ = host
	p := domain.CustomerPrincipal{AccountID: account, TaxID: "20123456786"}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, p.TaxID).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: email, Primary: true}}}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/customer/sites/" + host + "/orders/" + seed.draft.RFQ.ID.String()
	if response := e.do(t, request{method: http.MethodGet, path: path, token: token}); response.Code != 404 {
		t.Fatalf("history before verification=%d", response.Code)
	}
	messenger := &captureContactMessenger{}
	verifier := services.NewClientContactVerificationService(service, store, messenger, config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 2})
	proof, err := verifier.Request(ctx, p, domain.ChannelTypeEmail, email)
	if err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 1 {
		t.Fatal("verification was not dispatched")
	}
	fields := strings.Fields(messenger.messages[0].Body)
	link, err := url.Parse(fields[len(fields)-1])
	if err != nil {
		t.Fatal(err)
	}
	secret := link.Query().Get("token")

	if _, err := verifier.Request(ctx, p, domain.ChannelTypeEmail, email); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("proof cooldown: %v", err)
	}
	foreign := p
	foreign.AccountID = uuid.New()
	if err := verifier.Confirm(ctx, foreign, proof.ID, secret); err == nil {
		t.Fatal("foreign supplier redeemed proof")
	}
	foreign = p
	foreign.CustomerID = uuid.New()
	if err := verifier.Confirm(ctx, foreign, proof.ID, secret); err == nil {
		t.Fatal("another customer redeemed proof")
	}
	if err := verifier.Confirm(ctx, p, proof.ID, "wrong"); err == nil {
		t.Fatal("invalid proof transferred history")
	}
	for range 2 {
		if err := verifier.Confirm(ctx, p, proof.ID, secret); err != nil {
			t.Fatal(err)
		}
	}
	profile, err := service.GetProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "Registered Name" || len(profile.Contacts) != 1 || !profile.Contacts[0].Verified {
		t.Fatalf("canonical profile=%+v", profile)
	}
	var quoteClient, rfqClient, mergedInto uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT q.client_id,r.client_id,c.merged_into FROM quote q JOIN rfq r ON r.account_id=q.account_id AND r.id=q.rfq_id JOIN client c ON c.account_id=q.account_id AND c.id=$2 WHERE q.account_id=$3 AND q.id=$1`, seed.draft.Quote.ID, source, account).Scan(&quoteClient, &rfqClient, &mergedInto); err != nil {
		t.Fatal(err)
	}
	if quoteClient != *profile.ClientID || rfqClient != *profile.ClientID || mergedInto != *profile.ClientID {
		t.Fatal("verified history was not transferred")
	}
	if response := e.do(t, request{method: http.MethodGet, path: path, token: token}); response.Code != 200 {
		t.Fatalf("history after verification=%d %s", response.Code, response.Body)
	}
	var retainedName string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT name FROM client WHERE account_id=$1 AND id=$2`, account, source).Scan(&retainedName); err != nil {
		t.Fatal(err)
	}
	if retainedName != name {
		t.Fatal("source provenance was overwritten")
	}
}

type captureContactMessenger struct{ messages []domain.ContactMessage }

func (m *captureContactMessenger) SendContact(_ context.Context, message domain.ContactMessage) (string, error) {
	m.messages = append(m.messages, message)
	return "test-reference", nil
}

func (e *env) verifyCustomerContact(t *testing.T, account uuid.UUID, tax string, kind domain.ChannelType, value string, preserve ...bool) {
	t.Helper()
	ctx := context.Background()
	p := domain.CustomerPrincipal{AccountID: account, TaxID: tax}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	store := repository.NewClientContactRepository()
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), store)
	if len(preserve) == 0 {
		if err := service.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}}); err != nil {
			t.Fatal(err)
		}
	}
	messenger := &captureContactMessenger{}
	verifier := services.NewClientContactVerificationService(service, store, messenger, config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 3})
	proof, err := verifier.Request(ctx, p, kind, value)
	if err != nil {
		t.Fatal(err)
	}
	secret := messenger.messages[0].Code
	if kind == domain.ChannelTypeEmail {
		fields := strings.Fields(messenger.messages[0].Body)
		link, _ := url.Parse(fields[len(fields)-1])
		secret = link.Query().Get("token")
	}
	if err := verifier.Confirm(ctx, p, proof.ID, secret); err != nil {
		t.Fatal(err)
	}
}

func TestClientContacts_RegistrationLinkUsesOwnedVerifiedContact(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Profile link")
	ctx := context.Background()
	name, email := "Juan Perez", "profile-link@example.test"
	var clientID uuid.UUID
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
		c, err := repository.NewClientRepository().Create(ctx, q, account, domain.NewClient{Name: &name, Email: &email})
		if err == nil {
			clientID = c.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE account SET public_hostname=$2 WHERE id=$1`, account, "links-"+account.String()+".coti.test"); err != nil {
		t.Fatal(err)
	}
	messenger := &captureContactMessenger{}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(messenger)
	tenant := domain.Tenant{AccountID: account, BranchID: branch, Role: domain.UserRoleAdmin}
	if err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeEmail); err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 1 || messenger.messages[0].To != email || messenger.messages[0].AccountID != account || messenger.messages[0].BranchID != branch || !strings.Contains(messenger.messages[0].Body, "/login?next=%2Fprofile") {
		t.Fatalf("profile link: %+v", messenger.messages)
	}
	foreign, _ := e.seedAccount(t, "Foreign profile link")
	tenant.AccountID = foreign
	if err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeEmail); err == nil {
		t.Fatal("foreign account sent profile link")
	}
	tenant.AccountID = account
	tenant.BranchID = uuid.Nil
	if err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeEmail); err == nil {
		t.Fatal("missing branch sent profile link")
	}
	if len(messenger.messages) != 1 {
		t.Fatal("invalid scope reached provider")
	}
}

func TestClientContacts_MessagingConfigurationRequiresActiveOwnedBranch(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Contact channel scope")
	ctx := context.Background()
	channel := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(id,account_id,branch_id,type,config) VALUES($1,$2,$3,'WHATSAPP','{"phone_number_id":"test","access_token":"encrypted"}')`, channel, account, branch); err != nil {
		t.Fatal(err)
	}
	store := repository.NewClientContactRepository()
	get := func(scope, selectedBranch uuid.UUID) error {
		return e.db.InTenantTx(ctx, domain.Tenant{AccountID: scope}, func(q repository.Querier) error {
			_, err := store.GetMessagingConfiguration(ctx, q, scope, selectedBranch, channel, domain.ChannelTypeWhatsApp)
			return err
		})
	}
	if err := get(account, branch); err != nil {
		t.Fatal(err)
	}
	foreign, _ := e.seedAccount(t, "Foreign contact channel")
	if err := get(foreign, uuid.Nil); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("foreign configuration: %v", err)
	}
	if err := get(account, uuid.New()); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("other branch configuration: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE branch SET is_active=FALSE WHERE account_id=$1 AND id=$2`, account, branch); err != nil {
		t.Fatal(err)
	}
	if err := get(account, uuid.Nil); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("inactive branch configuration: %v", err)
	}
}
