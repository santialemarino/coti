//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type adoptedProfile struct {
	OwnershipUnproved bool    `json:"ownership_unproved"`
	Name              string  `json:"name"`
	LegalName         *string `json:"legal_name"`
	IVACondition      *string `json:"iva_condition"`
	Address           *string `json:"fiscal_address"`
	Contacts          []struct {
		Kind    string `json:"kind"`
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"contacts"`
}

// legacyState is everything the seller sees of the earlier profile, read past row level security.
type legacyState struct {
	name, legalName, address, iva, phone, email string
	contacts                                    string
}

func (e *env) legacyProfileState(t *testing.T, account, clientID uuid.UUID) legacyState {
	t.Helper()
	var s legacyState
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT COALESCE(c.name,''),COALESCE(c.legal_name,''),COALESCE(c.fiscal_address,''),COALESCE(c.iva_condition::text,''),COALESCE(c.phone,''),COALESCE(c.email,''),
 COALESCE((SELECT string_agg(kind::text||':'||value||':'||is_primary::text,',' ORDER BY kind,value) FROM client_contact WHERE account_id=c.account_id AND client_id=c.id AND source='SELLER'),'')
 FROM client c WHERE c.account_id=$1 AND c.id=$2`, account, clientID).Scan(&s.name, &s.legalName, &s.address, &s.iva, &s.phone, &s.email, &s.contacts); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCustomerProfile_AdoptedLegacyProfileStaysHiddenUntilOwnershipProof(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Legacy adoption")
	ctx := context.Background()
	const tax = "20123456786"
	legacy := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client(id,account_id,name,tax_id,legal_name,fiscal_address,iva_condition,phone,email) VALUES($1,$2,'Dueño SA',$3,'Dueño SA','Calle Falsa 123','REGISTERED','+5491155557777','owner@example.test')`, legacy, account, tax); err != nil {
		t.Fatal(err)
	}
	before := e.legacyProfileState(t, account, legacy)
	host, token := e.registerCustomer(t, account, tax, "")
	base := "/v1/customer/sites/" + host + "/profile"
	if after := e.legacyProfileState(t, account, legacy); after != before {
		t.Fatalf("registration changed the earlier profile: %+v -> %+v", before, after)
	}

	get := func() adoptedProfile {
		t.Helper()
		response := e.do(t, request{method: http.MethodGet, path: base, token: token})
		if response.Code != http.StatusOK {
			t.Fatalf("profile: %d %s", response.Code, response.Body)
		}
		for _, leaked := range []string{"5491155557777", "owner@example.test", "Dueño", "Calle Falsa"} {
			if strings.Contains(response.Body.String(), leaked) {
				t.Fatalf("unproved customer read %q: %s", leaked, response.Body)
			}
		}
		var profile adoptedProfile
		if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
			t.Fatal(err)
		}
		return profile
	}
	profile := get()
	if !profile.OwnershipUnproved || profile.Name != "Juan Perez" || profile.LegalName != nil || profile.IVACondition != nil || profile.Address != nil ||
		len(profile.Contacts) != 1 || profile.Contacts[0].Value != "customer"+tax+"@example.test" || !profile.Contacts[0].Primary {
		t.Fatalf("unproved profile: %+v", profile)
	}

	fiscal := map[string]any{"name": "Juan Perez", "iva_condition": "FINAL_CONSUMER", "contacts": []map[string]any{{"kind": "EMAIL", "value": "customer" + tax + "@example.test", "primary": true}}}
	response := e.do(t, request{method: http.MethodPut, path: base, token: token, body: fiscal})
	if response.Code != http.StatusForbidden || errorCode(t, response) != string(domain.CodeProfileOwnershipUnproved) {
		t.Fatalf("unproved fiscal write: %d %s", response.Code, response.Body)
	}
	replace := map[string]any{"name": "Squatter Name", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": "owner@example.test", "primary": false},
		{"kind": "EMAIL", "value": "new@example.test", "primary": true},
		{"kind": "WHATSAPP", "value": "+5491155550000", "primary": true},
	}}
	if response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: replace}); response.Code != http.StatusNoContent {
		t.Fatalf("replace: %d %s", response.Code, response.Body)
	}
	drop := map[string]any{"name": "Squatter Name", "contacts": []map[string]any{{"kind": "EMAIL", "value": "new@example.test", "primary": true}}}
	if response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: drop}); response.Code != http.StatusNoContent {
		t.Fatalf("drop: %d %s", response.Code, response.Body)
	}
	if after := e.legacyProfileState(t, account, legacy); after != before {
		t.Fatalf("unproved customer changed the earlier profile: %+v -> %+v", before, after)
	}
	profile = get()
	if profile.Name != "Squatter Name" || len(profile.Contacts) != 1 || profile.Contacts[0].Value != "new@example.test" {
		t.Fatalf("own entries: %+v", profile)
	}

	p := domain.CustomerPrincipal{AccountID: account, TaxID: tax}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	store := repository.NewClientContactRepository()
	messenger := &captureContactMessenger{}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), store)
	verifier := services.NewClientContactVerificationService(service, store, messenger, config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 3})
	if _, err := verifier.Request(ctx, p, domain.ChannelTypeWhatsApp, "+5491155557777"); !errors.Is(err, domain.ErrNotFound) || len(messenger.messages) != 0 {
		t.Fatalf("code sent to a contact the customer never entered: %v", err)
	}

	claim := map[string]any{"name": "Squatter Name", "contacts": []map[string]any{
		{"kind": "EMAIL", "value": "new@example.test", "primary": true},
		{"kind": "WHATSAPP", "value": "+5491155557777", "primary": true},
	}}
	if response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: claim}); response.Code != http.StatusNoContent {
		t.Fatalf("claim: %d %s", response.Code, response.Body)
	}
	e.verifyCustomerContact(t, account, tax, domain.ChannelTypeWhatsApp, "+5491155557777", true)
	response = e.do(t, request{method: http.MethodGet, path: base, token: token})
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	values := make([]string, 0, len(profile.Contacts))
	for _, c := range profile.Contacts {
		values = append(values, c.Value)
	}
	slices.Sort(values)
	if profile.OwnershipUnproved || profile.Name != "Dueño SA" || profile.LegalName == nil || *profile.LegalName != "Dueño SA" || profile.Address == nil ||
		strings.Join(values, ",") != "+5491155557777,new@example.test,owner@example.test" {
		t.Fatalf("proved profile: %+v", profile)
	}
	fiscal["contacts"] = claim["contacts"]
	if response = e.do(t, request{method: http.MethodPut, path: base, token: token, body: fiscal}); response.Code != http.StatusNoContent {
		t.Fatalf("proved fiscal write: %d %s", response.Code, response.Body)
	}
}

func TestCustomerProfile_NewCUITOwnsItsProfileFromRegistration(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Fresh registration")
	host, token := e.registerCustomer(t, account, "30712345671", "")
	base := "/v1/customer/sites/" + host + "/profile"
	body := map[string]any{"name": "Obra Nueva", "legal_name": "Obra Nueva SRL", "iva_condition": "REGISTERED", "contacts": []map[string]any{{"kind": "EMAIL", "value": "obra@example.test", "primary": true}}}
	if response := e.do(t, request{method: http.MethodPut, path: base, token: token, body: body}); response.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", response.Code, response.Body)
	}
	response := e.do(t, request{method: http.MethodGet, path: base, token: token})
	var profile adoptedProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.OwnershipUnproved || profile.Name != "Obra Nueva" || profile.LegalName == nil || *profile.LegalName != "Obra Nueva SRL" || len(profile.Contacts) != 1 || !profile.Contacts[0].Primary {
		t.Fatalf("fresh profile: %+v", profile)
	}
	var name, email string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT name,email FROM client WHERE account_id=$1 AND tax_id='30712345671'`, account).Scan(&name, &email); err != nil {
		t.Fatal(err)
	}
	if name != "Obra Nueva" || email != "obra@example.test" {
		t.Fatalf("fresh profile row: %s %s", name, email)
	}
}
