//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// googleStub answers every exchange with the identity a test staged for it.
type googleStub struct{ identity domain.CustomerGoogleIdentity }

func (g *googleStub) Exchange(context.Context, string, string, string, string) (*domain.CustomerGoogleIdentity, error) {
	identity := g.identity
	return &identity, nil
}

type googleExchange struct {
	SignupToken string `json:"signup_token"`
	SignupEmail string `json:"signup_email"`
}

func newGoogleEnv(t *testing.T) (*env, *googleStub) {
	t.Helper()
	stub := &googleStub{}
	return newEnvWithRFQProviders(t, rfqTestProviders{google: stub}), stub
}

// googleSignup stages one Google identity, exchanges a code for it and returns the signup capability.
func (e *env) googleSignup(t *testing.T, stub *googleStub, host, email string, verified bool) googleExchange {
	t.Helper()
	stub.identity = domain.CustomerGoogleIdentity{Subject: "google-" + uuid.NewString(), Email: email, EmailVerified: verified, Name: "Google Name"}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	response := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/google/exchange", body: map[string]any{"code": "one-use-code", "verifier": secret, "nonce": secret}})
	var result googleExchange
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK || result.SignupToken == "" {
		t.Fatalf("google exchange: %d %s", response.Code, response.Body)
	}
	return result
}

func (e *env) completeGoogle(t *testing.T, host string, body map[string]any) (int, string) {
	t.Helper()
	response := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/google/complete", body: body})
	var login struct {
		Token string `json:"session_token"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &login)
	return response.Code, login.Token
}

func (e *env) publicHost(t *testing.T, account uuid.UUID) string {
	t.Helper()
	host := "google-" + account.String() + ".coti.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET public_hostname=$2 WHERE id=$1`, account, host); err != nil {
		t.Fatal(err)
	}
	return host
}

// contactState reads one value's contact on the CUIT's profile: whether it exists, is verified, and is
// owned by the customer that registered the CUIT.
func (e *env) contactState(t *testing.T, account uuid.UUID, tax, value string) (exists, verified, owned bool) {
	t.Helper()
	err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(c.id)>0,COALESCE(bool_or(c.verified_at IS NOT NULL),FALSE),COALESCE(bool_or(o.contact_id IS NOT NULL),FALSE)
 FROM client p JOIN client_contact c ON c.account_id=p.account_id AND c.client_id=p.id AND c.retired_at IS NULL
 LEFT JOIN customer_identity i ON i.account_id=p.account_id AND i.tax_id=p.tax_id
 LEFT JOIN customer_contact_ownership o ON o.account_id=c.account_id AND o.contact_id=c.id AND o.customer_id=i.id
 WHERE p.account_id=$1 AND p.tax_id=$2 AND p.merged_into IS NULL AND lower(c.value)=lower($3)`, account, tax, value).Scan(&exists, &verified, &owned)
	if err != nil {
		t.Fatal(err)
	}
	return exists, verified, owned
}

func TestCustomerGoogle_EmailIsNeverCopiedUnlessChosen(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google not chosen")
	host := e.publicHost(t, account)
	const tax = "20123456786"
	signup := e.googleSignup(t, stub, host, "chooser@example.test", true)
	if signup.SignupEmail != "chooser@example.test" {
		t.Fatalf("verified Google address not offered: %+v", signup)
	}
	if status, _ := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez"}); status != http.StatusUnprocessableEntity {
		t.Fatalf("registration without a contact: %d", status)
	}
	signup = e.googleSignup(t, stub, host, "chooser@example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez",
		"contacts": []map[string]any{{"kind": "WHATSAPP", "value": "+5491155550101", "primary": true}}})
	if status != http.StatusCreated || token == "" {
		t.Fatalf("registration with its own contact: %d", status)
	}
	if exists, _, _ := e.contactState(t, account, tax, "chooser@example.test"); exists {
		t.Fatal("the Google address was copied to the profile without being chosen")
	}
	if exists, verified, _ := e.contactState(t, account, tax, "+5491155550101"); !exists || verified {
		t.Fatalf("own contact: exists=%v verified=%v, want pending verification", exists, verified)
	}
}

func TestCustomerGoogle_ChosenEmailIsAVerifiedMainContact(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google chosen")
	host := e.publicHost(t, account)
	const tax = "20123456786"
	signup := e.googleSignup(t, stub, host, "Chosen@Example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true})
	if status != http.StatusCreated {
		t.Fatalf("registration with the Google address: %d", status)
	}
	if exists, verified, owned := e.contactState(t, account, tax, "chosen@example.test"); !exists || !verified || !owned {
		t.Fatalf("chosen Google address: exists=%v verified=%v owned=%v", exists, verified, owned)
	}
	response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/profile", token: token})
	var profile struct {
		Contacts []struct {
			Value    string `json:"value"`
			Verified bool   `json:"verified"`
			Main     bool   `json:"main"`
		} `json:"contacts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil || response.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", response.Code, response.Body)
	}
	if len(profile.Contacts) != 1 || profile.Contacts[0].Value != "chosen@example.test" || !profile.Contacts[0].Verified || !profile.Contacts[0].Main {
		t.Fatalf("profile contacts: %s", response.Body)
	}
}

func TestCustomerGoogle_UnverifiedEmailIsNeverAutoVerified(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google unverified")
	host := e.publicHost(t, account)
	const tax = "20123456786"
	signup := e.googleSignup(t, stub, host, "unverified@example.test", false)
	if signup.SignupEmail != "" {
		t.Fatalf("unverified Google address offered: %+v", signup)
	}
	if status, _ := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true}); status != http.StatusUnprocessableEntity {
		t.Fatalf("unverified Google address accepted: %d", status)
	}
	if exists, _, _ := e.contactState(t, account, tax, "unverified@example.test"); exists {
		t.Fatal("unverified Google address reached the profile")
	}
}

func TestCustomerGoogle_ChosenEmailMergesTheDumbProfileThatOwnsIt(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, branch := e.seedAccount(t, "Google merge")
	host := e.publicHost(t, account)
	ctx := context.Background()
	const tax = "20123456786"
	dumb, channel, rfq := uuid.New(), uuid.New(), uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,email) VALUES($1,$2,'Mostrador','dumb@example.test')`, dumb, account); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(id,account_id,branch_id,type) VALUES ($1,$2,$3,'MANUAL_ENTRY')`, channel, account, branch); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO rfq(id,account_id,branch_id,channel_id,client_id,raw_text) VALUES($1,$2,$3,$4,$5,'Pedido previo')`, rfq, account, branch, channel, dumb); err != nil {
		t.Fatal(err)
	}
	signup := e.googleSignup(t, stub, host, "dumb@example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true})
	if status != http.StatusCreated {
		t.Fatalf("registration with the dumb profile's address: %d", status)
	}
	var registered uuid.UUID
	var mergedInto *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT p.id,d.merged_into FROM client p, client d WHERE p.account_id=$1 AND p.tax_id=$2 AND d.account_id=$1 AND d.id=$3`, account, tax, dumb).Scan(&registered, &mergedInto); err != nil {
		t.Fatal(err)
	}
	if mergedInto == nil || *mergedInto != registered {
		t.Fatalf("dumb profile merged into %v, want %v", mergedInto, registered)
	}
	if exists, verified, owned := e.contactState(t, account, tax, "dumb@example.test"); !exists || !verified || !owned {
		t.Fatalf("merged address: exists=%v verified=%v owned=%v", exists, verified, owned)
	}
	if response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + rfq.String(), token: token}); response.Code != http.StatusOK {
		t.Fatalf("merged order history: %d %s", response.Code, response.Body)
	}
}

func TestCustomerGoogle_ChosenEmailProvesOwnershipOfTheLegacyProfile(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google legacy")
	host := e.publicHost(t, account)
	ctx := context.Background()
	const tax = "20123456786"
	legacy := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,tax_id,legal_name,phone,email) VALUES($1,$2,'Dueño SA',$3,'Dueño SA','+5491155557777','owner@example.test')`, legacy, account, tax); err != nil {
		t.Fatal(err)
	}
	signup := e.googleSignup(t, stub, host, "owner@example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true})
	if status != http.StatusCreated {
		t.Fatalf("registration over the legacy profile: %d", status)
	}
	response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/profile", token: token})
	var profile adoptedProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil || response.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", response.Code, response.Body)
	}
	if profile.OwnershipUnproved || profile.LegalName == nil || *profile.LegalName != "Dueño SA" {
		t.Fatalf("legacy profile not adopted by the Google proof: %s", response.Body)
	}
}

func TestCustomerGoogle_NewEmailDoesNotProveTheLegacyProfile(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google legacy other")
	host := e.publicHost(t, account)
	const tax = "20123456786"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO client(id,account_id,name,tax_id,email) VALUES($1,$2,'Dueño SA',$3,'owner@example.test')`, uuid.New(), account, tax); err != nil {
		t.Fatal(err)
	}
	signup := e.googleSignup(t, stub, host, "someone-else@example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true})
	if status != http.StatusCreated {
		t.Fatalf("registration over the legacy profile: %d", status)
	}
	response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/profile", token: token})
	var profile adoptedProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil || response.Code != http.StatusOK {
		t.Fatalf("profile: %d %s", response.Code, response.Body)
	}
	if !profile.OwnershipUnproved {
		t.Fatalf("an address the legacy profile never had proved ownership: %s", response.Body)
	}
}
