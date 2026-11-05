//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
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
	SignupToken       string `json:"signup_token"`
	SignupEmail       string `json:"signup_email"`
	SignupEmailProven bool   `json:"signup_email_proven"`
}

func newGoogleEnv(t *testing.T) (*env, *googleStub) {
	t.Helper()
	stub := &googleStub{}
	return newEnvWithRFQProviders(t, rfqTestProviders{google: stub}), stub
}

// googleSignup stages a Google Workspace identity for email, whose hosted domain proves the mailbox,
// exchanges a code for it and returns the signup capability.
func (e *env) googleSignup(t *testing.T, stub *googleStub, host, email string, verified bool) googleExchange {
	t.Helper()
	return e.googleSignupAs(t, stub, host, email, verified, email[strings.LastIndex(email, "@")+1:])
}

// googleSignupAs stages a Google identity with the given hosted domain claim, empty for none.
func (e *env) googleSignupAs(t *testing.T, stub *googleStub, host, email string, verified bool, hostedDomain string) googleExchange {
	t.Helper()
	stub.identity = domain.CustomerGoogleIdentity{Subject: "google-" + uuid.NewString(), Email: email, EmailVerified: verified, HostedDomain: hostedDomain, Name: "Google Name"}
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
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,email,dni,legal_name) VALUES($1,$2,'Mostrador','dumb@example.test','28123456','Mostrador SA')`, dumb, account); err != nil {
		t.Fatal(err)
	}
	// A profile a seller had already merged into the dumb one follows it into the registered profile.
	earlier := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,merged_into) VALUES($1,$2,'Mostrador viejo',$3)`, earlier, account, dumb); err != nil {
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
	var earlierInto *uuid.UUID
	var dni, legalName *string
	var audits, bySeller int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT e.merged_into,r.dni,r.legal_name,
 (SELECT count(*) FROM client_merge_audit WHERE account_id=$1 AND source_client_id=$3 AND target_client_id=$4),
 (SELECT count(*) FROM client_merge_audit WHERE account_id=$1 AND merged_by IS NOT NULL)
 FROM client e, client r WHERE e.id=$2 AND r.id=$4`, account, earlier, dumb, registered).Scan(&earlierInto, &dni, &legalName, &audits, &bySeller); err != nil {
		t.Fatal(err)
	}
	if earlierInto == nil || *earlierInto != registered {
		t.Fatalf("the profile merged into the dumb one now points at %v, want the registered %v", earlierInto, registered)
	}
	if text(dni) != "28123456" || text(legalName) != "Mostrador SA" {
		t.Fatalf("registered dni = %s, legal name = %s, want the dumb profile's carried over", text(dni), text(legalName))
	}
	if audits != 1 || bySeller != 0 {
		t.Fatalf("audit rows = %d (%d by staff), want one with no staff user", audits, bySeller)
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

func TestCustomerGoogle_EmailVerifiedOnAnotherCUITIsRefusedWithItsOwnCode(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google in use")
	const personal, company = "20123456786", "30712345671"
	host, _ := e.registerCustomer(t, account, personal)
	shared := "customer" + personal + "@example.test"
	e.verifyCustomerContact(t, account, personal, domain.ChannelTypeEmail, shared, true)
	signup := e.googleSignup(t, stub, host, shared, true)
	response := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/google/complete", body: map[string]any{"token": signup.SignupToken, "tax_id": company, "name": "Empresa SA", "use_google_email": true}})
	if response.Code != http.StatusConflict || errorCode(t, response) != string(domain.CodeContactInUse) {
		t.Fatalf("address verified on another CUIT: %d %s", response.Code, response.Body)
	}
	var identities int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, company).Scan(&identities); err != nil || identities != 0 {
		t.Fatalf("refused registration left %d identities: %v", identities, err)
	}
	// The refusal rolled everything back, the spent signup token included, so the same token still
	// registers once the Google address is left out.
	status, _ := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": company, "name": "Empresa SA", "use_google_email": false,
		"contacts": []map[string]any{{"kind": "WHATSAPP", "value": "+5491155550199", "primary": true}}})
	if status != http.StatusCreated {
		t.Fatalf("retry with the same token and the address left out: %d", status)
	}
	if exists, _, _ := e.contactState(t, account, company, shared); exists {
		t.Fatal("the retry without the Google address still copied it")
	}
}

func TestCustomerGoogle_ChosenEmailIsThePrimaryOverATypedOne(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, _ := e.seedAccount(t, "Google primary")
	host := e.publicHost(t, account)
	const tax = "20123456786"
	signup := e.googleSignup(t, stub, host, "verified@example.test", true)
	status, _ := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true,
		"contacts": []map[string]any{{"kind": "EMAIL", "value": "typed@example.test", "primary": true}}})
	if status != http.StatusCreated {
		t.Fatalf("registration with both emails: %d", status)
	}
	var projected, primary string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT COALESCE(p.email,''),(SELECT value FROM client_contact WHERE account_id=p.account_id AND client_id=p.id AND kind='EMAIL' AND is_primary) FROM client p WHERE p.account_id=$1 AND p.tax_id=$2`, account, tax).Scan(&projected, &primary); err != nil {
		t.Fatal(err)
	}
	if projected != "verified@example.test" || primary != "verified@example.test" {
		t.Fatalf("primary email %q, projected %q; want the verified Google address", primary, projected)
	}
	if exists, verified, _ := e.contactState(t, account, tax, "typed@example.test"); !exists || verified {
		t.Fatalf("typed email: exists=%v verified=%v, want pending verification", exists, verified)
	}
}

func TestCustomerGoogle_ChosenEmailBringsTheDumbHistoryIntoAnUnprovedLegacyProfile(t *testing.T) {
	e, stub := newGoogleEnv(t)
	account, branch := e.seedAccount(t, "Google legacy merge")
	host := e.publicHost(t, account)
	ctx := context.Background()
	const tax = "20123456786"
	legacy, dumb, channel, rfq := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,tax_id,email) VALUES($1,$2,'Dueño SA',$3,'owner@example.test')`, legacy, account, tax); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,email) VALUES($1,$2,'Mostrador','walkin@example.test')`, dumb, account); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(id,account_id,branch_id,type) VALUES ($1,$2,$3,'MANUAL_ENTRY')`, channel, account, branch); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO rfq(id,account_id,branch_id,channel_id,client_id,raw_text) VALUES($1,$2,$3,$4,$5,'Pedido de mostrador')`, rfq, account, branch, channel, dumb); err != nil {
		t.Fatal(err)
	}
	signup := e.googleSignup(t, stub, host, "walkin@example.test", true)
	status, token := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true})
	if status != http.StatusCreated {
		t.Fatalf("registration over the legacy profile: %d", status)
	}
	var mergedInto *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT merged_into FROM client WHERE account_id=$1 AND id=$2`, account, dumb).Scan(&mergedInto); err != nil {
		t.Fatal(err)
	}
	if mergedInto == nil || *mergedInto != legacy {
		t.Fatalf("dumb profile merged into %v, want the legacy profile %v", mergedInto, legacy)
	}
	if response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/orders/" + rfq.String(), token: token}); response.Code != http.StatusOK {
		t.Fatalf("the proved address's orders are not visible on the unproved legacy profile: %d %s", response.Code, response.Body)
	}
	response := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/profile", token: token})
	var profile adoptedProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil || !profile.OwnershipUnproved {
		t.Fatalf("a merged address proved the legacy profile: %s", response.Body)
	}
	var email string
	var primaries []string
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT COALESCE(p.email,''),COALESCE((SELECT array_agg(c.value ORDER BY c.value) FROM client_contact c WHERE c.account_id=p.account_id AND c.client_id=p.id AND c.is_primary AND c.retired_at IS NULL),'{}')
 FROM client p WHERE p.account_id=$1 AND p.id=$2`, account, legacy).Scan(&email, &primaries); err != nil {
		t.Fatal(err)
	}
	if email != "owner@example.test" || len(primaries) != 1 || primaries[0] != "owner@example.test" {
		t.Fatalf("unproved legacy profile: email %q, primaries %v; want its own address alone", email, primaries)
	}
}

// Only a mailbox Google controls today joins verified: Gmail, or a Workspace domain the hd claim
// names. Any other address Google once verified joins pending, and a code verifies it.
func TestCustomerGoogle_OnlyAMailboxGoogleProvesJoinsVerified(t *testing.T) {
	e, stub := newGoogleEnv(t)
	cases := []struct {
		name, email, hosted string
		proven              bool
	}{
		{"gmail", "juan.%s@gmail.com", "", true},
		{"workspace", "juan@%s.example.test", "%s.example.test", true},
		{"other domain without hd", "juan@%s.example.test", "", false},
		{"hd of another domain", "juan@%s.example.test", "otro.example.test", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account, _ := e.seedAccount(t, "Google proof "+tc.name)
			host := e.publicHost(t, account)
			const tax = "20123456786"
			unique := strings.ReplaceAll(uuid.NewString(), "-", "")
			email := strings.ReplaceAll(tc.email, "%s", unique)
			hosted := strings.ReplaceAll(tc.hosted, "%s", unique)
			signup := e.googleSignupAs(t, stub, host, email, true, hosted)
			if signup.SignupEmail != email || signup.SignupEmailProven != tc.proven {
				t.Fatalf("exchange offered %q proven=%v, want %q proven=%v", signup.SignupEmail, signup.SignupEmailProven, email, tc.proven)
			}
			if status, _ := e.completeGoogle(t, host, map[string]any{"token": signup.SignupToken, "tax_id": tax, "name": "Juan Perez", "use_google_email": true}); status != http.StatusCreated {
				t.Fatalf("registration with the Google address: %d", status)
			}
			exists, verified, owned := e.contactState(t, account, tax, email)
			if !exists || verified != tc.proven || owned != tc.proven {
				t.Fatalf("chosen address: exists=%v verified=%v owned=%v, want verified and owned = %v", exists, verified, owned, tc.proven)
			}
			if tc.proven {
				return
			}
			e.verifyCustomerContact(t, account, tax, domain.ChannelTypeEmail, email, true)
			if _, verified, owned := e.contactState(t, account, tax, email); !verified || !owned {
				t.Fatalf("after the code: verified=%v owned=%v", verified, owned)
			}
		})
	}
}
