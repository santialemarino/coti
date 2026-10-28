//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const recoveryTaxID = "20123456786"

type recoveryOption struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	Masked      string    `json:"masked"`
	Main        bool      `json:"main"`
	Available   bool      `json:"available"`
	Preselected bool      `json:"preselected"`
}

func (e *env) recoveryOptions(t *testing.T, host, taxID string) ([]recoveryOption, string) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/options", body: map[string]any{"tax_id": taxID}})
	if rec.Code != http.StatusOK {
		t.Fatalf("recovery options: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Contacts []recoveryOption `json:"contacts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Contacts, rec.Body.String()
}

// requestRecoveryCode sends a code to one contact and returns the challenge and the code it carried.
func (e *env) requestRecoveryCode(t *testing.T, host, taxID string, contactID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	sent := len(e.contacts.messages)
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/code", body: map[string]any{"tax_id": taxID, "contact_id": contactID}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("recovery code: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(e.contacts.messages) != sent+1 {
		t.Fatal("recovery code was not dispatched")
	}
	message := e.contacts.messages[sent]
	if message.Purpose != domain.ContactPurposeRecovery || !strings.Contains(message.Body, message.Code) {
		t.Fatalf("recovery message: %+v", message)
	}
	return body.ID, message.Code
}

func (e *env) resetWithRecoveryCode(t *testing.T, host string, challenge uuid.UUID, code, password string) (int, string) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/reset", body: map[string]any{"id": challenge, "code": code, "password": password}})
	var body struct {
		Token string `json:"session_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body.Token
}

func customerEmail(taxID string) string { return "customer" + taxID + "@example.test" }

func TestAccountRecovery_ByEmailResetsPasswordRevokesSessionsAndSignsIn(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Recovery by email")
	host, oldSession := e.registerCustomer(t, account, recoveryTaxID)
	e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelTypeEmail, customerEmail(recoveryTaxID))

	options, raw := e.recoveryOptions(t, host, "20-12345678-6")
	if len(options) != 1 || options[0].Kind != "EMAIL" || !options[0].Main || !options[0].Available || !options[0].Preselected {
		t.Fatalf("options=%+v", options)
	}
	if options[0].Masked != "c•••@example.test" || strings.Contains(raw, customerEmail(recoveryTaxID)) || strings.Contains(raw, recoveryTaxID) {
		t.Fatalf("recovery options reveal more than the mask: %s", raw)
	}
	challenge, code := e.requestRecoveryCode(t, host, recoveryTaxID, options[0].ID)
	if to := e.contacts.messages[len(e.contacts.messages)-1].To; to != customerEmail(recoveryTaxID) {
		t.Fatalf("code went to %q", to)
	}

	status, session := e.resetWithRecoveryCode(t, host, challenge, code, "Replacement-pass123!")
	if status != http.StatusCreated || session == "" {
		t.Fatalf("reset: %d", status)
	}
	base := "/v1/customer/sites/" + host
	if rec := e.do(t, request{method: http.MethodGet, path: base + "/me", token: session}); rec.Code != http.StatusOK {
		t.Fatalf("recovered session: %d", rec.Code)
	}
	if rec := e.do(t, request{method: http.MethodGet, path: base + "/me", token: oldSession}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session survived recovery: %d", rec.Code)
	}
	if rec := e.do(t, request{method: http.MethodPost, path: base + "/auth/login", body: map[string]any{"tax_id": recoveryTaxID, "password": "Replacement-pass123!"}}); rec.Code != http.StatusOK {
		t.Fatalf("login with the new password: %d %s", rec.Code, rec.Body)
	}
	if status, _ := e.resetWithRecoveryCode(t, host, challenge, code, "Another-pass1234!"); status != http.StatusUnauthorized {
		t.Fatalf("recovery code reused: %d", status)
	}
}

func TestAccountRecovery_WithoutVerifiedContactOffersNothingAndUnknownCUITLooksTheSame(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Recovery unverified")
	host, _ := e.registerCustomer(t, account, recoveryTaxID)

	options, unverified := e.recoveryOptions(t, host, recoveryTaxID)
	_, unknown := e.recoveryOptions(t, host, "30712345671")
	if len(options) != 0 || unverified != unknown {
		t.Fatalf("unverified=%s unknown=%s", unverified, unknown)
	}
	var contact uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM client_contact WHERE account_id=$1`, account).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/code", body: map[string]any{"tax_id": recoveryTaxID, "contact_id": contact}})
	if rec.Code != http.StatusNotFound || len(e.contacts.messages) != 0 {
		t.Fatalf("code sent to an unverified contact: %d", rec.Code)
	}
}

func TestAccountRecovery_WhatsAppWaitsForTheTemplate(t *testing.T) {
	phone := "+5491155550101"
	setup := func(t *testing.T, e *env, contacts []map[string]any) (string, uuid.UUID) {
		t.Helper()
		account, _ := e.seedAccount(t, "Recovery WhatsApp")
		host, token := e.registerCustomer(t, account, recoveryTaxID)
		if rec := e.do(t, request{method: http.MethodPut, path: "/v1/customer/sites/" + host + "/profile", token: token, body: map[string]any{"name": "Juan Perez", "contacts": contacts}}); rec.Code != http.StatusNoContent {
			t.Fatalf("profile: %d %s", rec.Code, rec.Body)
		}
		for _, c := range contacts {
			e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelType(c["kind"].(string)), c["value"].(string), true)
		}
		return host, account
	}
	both := []map[string]any{{"kind": "EMAIL", "value": customerEmail(recoveryTaxID), "primary": true}, {"kind": "WHATSAPP", "value": phone, "primary": true}}

	t.Run("disabled while the template is empty", func(t *testing.T) {
		e := newEnv(t)
		host, _ := setup(t, e, both)
		options, _ := e.recoveryOptions(t, host, recoveryTaxID)
		if len(options) != 2 || options[0].Kind != "WHATSAPP" || !options[0].Main || options[0].Available || options[0].Preselected || options[0].Masked != "+54 9 ••• ••••-0101" {
			t.Fatalf("WhatsApp option=%+v", options)
		}
		if options[1].Kind != "EMAIL" || !options[1].Available || !options[1].Preselected {
			t.Fatalf("email option=%+v", options[1])
		}
		rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/code", body: map[string]any{"tax_id": recoveryTaxID, "contact_id": options[0].ID}})
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("WhatsApp code without a template: %d", rec.Code)
		}
	})
	t.Run("only WhatsApp leaves nothing usable", func(t *testing.T) {
		e := newEnv(t)
		host, _ := setup(t, e, []map[string]any{{"kind": "WHATSAPP", "value": phone, "primary": true}})
		options, _ := e.recoveryOptions(t, host, recoveryTaxID)
		if len(options) != 1 || options[0].Available || options[0].Preselected {
			t.Fatalf("options=%+v", options)
		}
	})
	t.Run("usable once the template is set", func(t *testing.T) {
		e := newEnv(t, func(c *config.Config) { c.CustomerAuth.ContactCodeTemplate = "contact_code" })
		host, _ := setup(t, e, both)
		options, _ := e.recoveryOptions(t, host, recoveryTaxID)
		if !options[0].Available || !options[0].Preselected || options[1].Preselected {
			t.Fatalf("options=%+v", options)
		}
		e.requestRecoveryCode(t, host, recoveryTaxID, options[0].ID)
		if message := e.contacts.messages[len(e.contacts.messages)-1]; message.Kind != domain.ChannelTypeWhatsApp || message.To != phone {
			t.Fatalf("WhatsApp code: %+v", message)
		}
	})
}

func TestAccountRecovery_AttemptsAreLimited(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Recovery attempts")
	host, _ := e.registerCustomer(t, account, recoveryTaxID)
	e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelTypeEmail, customerEmail(recoveryTaxID))
	options, _ := e.recoveryOptions(t, host, recoveryTaxID)
	challenge, code := e.requestRecoveryCode(t, host, recoveryTaxID, options[0].ID)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for range 3 {
		if status, _ := e.resetWithRecoveryCode(t, host, challenge, wrong, "Replacement-pass123!"); status != http.StatusUnauthorized {
			t.Fatalf("wrong code: %d", status)
		}
	}
	if status, _ := e.resetWithRecoveryCode(t, host, challenge, code, "Replacement-pass123!"); status != http.StatusUnauthorized {
		t.Fatalf("right code after the attempts ran out: %d", status)
	}
}

func TestAccountRecovery_IsIsolatedPerSupplier(t *testing.T) {
	e := newEnv(t)
	supplierA, _ := e.seedAccount(t, "Recovery supplier A")
	supplierB, _ := e.seedAccount(t, "Recovery supplier B")
	hostA, _ := e.registerCustomer(t, supplierA, recoveryTaxID)
	hostB, _ := e.registerCustomer(t, supplierB, recoveryTaxID)
	e.verifyCustomerContact(t, supplierA, recoveryTaxID, domain.ChannelTypeEmail, customerEmail(recoveryTaxID))

	if options, _ := e.recoveryOptions(t, hostB, recoveryTaxID); len(options) != 0 {
		t.Fatalf("supplier B offered supplier A's contact: %+v", options)
	}
	options, _ := e.recoveryOptions(t, hostA, recoveryTaxID)
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + hostB + "/auth/recovery/code", body: map[string]any{"tax_id": recoveryTaxID, "contact_id": options[0].ID}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("supplier B sent a code to supplier A's contact: %d", rec.Code)
	}
	challenge, code := e.requestRecoveryCode(t, hostA, recoveryTaxID, options[0].ID)
	if status, _ := e.resetWithRecoveryCode(t, hostB, challenge, code, "Replacement-pass123!"); status != http.StatusUnauthorized {
		t.Fatalf("supplier A's code redeemed at supplier B: %d", status)
	}
	if status, _ := e.resetWithRecoveryCode(t, hostA, challenge, code, "Replacement-pass123!"); status != http.StatusCreated {
		t.Fatalf("supplier A's code at supplier A: %d", status)
	}
}

func TestAccountRecovery_MainContactIsAVerifiedOne(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Recovery main contact")
	host, token := e.registerCustomer(t, account, recoveryTaxID)
	base := "/v1/customer/sites/" + host + "/profile"
	first, second, pending := customerEmail(recoveryTaxID), "second@example.test", "pending@example.test"
	save := func(values ...string) {
		t.Helper()
		contacts := make([]map[string]any, 0, len(values))
		for i, value := range values {
			contacts = append(contacts, map[string]any{"kind": "EMAIL", "value": value, "primary": i == 0})
		}
		if rec := e.do(t, request{method: http.MethodPut, path: base, token: token, body: map[string]any{"name": "Juan Perez", "contacts": contacts}}); rec.Code != http.StatusNoContent {
			t.Fatalf("profile: %d %s", rec.Code, rec.Body)
		}
	}
	// Added in a later save, so it is loaded after the first.
	save(first, pending)
	save(first, pending, second)
	e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelTypeEmail, first, true)
	e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelTypeEmail, second, true)
	main := func() string {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: base, token: token})
		var profile struct {
			Contacts []struct {
				Value string `json:"value"`
				Main  bool   `json:"main"`
			} `json:"contacts"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &profile); err != nil {
			t.Fatal(err)
		}
		found := ""
		for _, c := range profile.Contacts {
			if c.Main {
				if found != "" {
					t.Fatalf("two main contacts: %s", rec.Body)
				}
				found = c.Value
			}
		}
		return found
	}
	if got := main(); got != first {
		t.Fatalf("default main=%q, want the first verified contact", got)
	}
	choose := func(value string) int {
		return e.do(t, request{method: http.MethodPut, path: base + "/main-contact", token: token, body: map[string]any{"kind": "EMAIL", "value": value}}).Code
	}
	if status := choose(pending); status != http.StatusNotFound || main() != first {
		t.Fatalf("unverified contact made main: %d", status)
	}
	if status := choose(second); status != http.StatusNoContent || main() != second {
		t.Fatalf("verified contact not made main: %d", status)
	}
	if options, _ := e.recoveryOptions(t, host, recoveryTaxID); !options[0].Main || options[0].Masked != "s•••@example.test" || !options[0].Preselected {
		t.Fatalf("recovery does not lead with the chosen main contact: %+v", options)
	}
	save(first, pending)
	if got := main(); got != first {
		t.Fatalf("main after removing it=%q, want the remaining verified contact", got)
	}
}

func TestAccountRecovery_SeparateRecoveryEmailIsGone(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Recovery email gone")
	host, token := e.registerCustomer(t, account, recoveryTaxID)
	base := "/v1/customer/sites/" + host + "/auth"
	for _, r := range []request{
		{method: http.MethodPost, path: base + "/forgot-password", body: map[string]any{"tax_id": recoveryTaxID, "email": customerEmail(recoveryTaxID)}},
		{method: http.MethodPost, path: base + "/verify-email", body: map[string]any{"token": strings.Repeat("a", 43)}},
		{method: http.MethodPost, path: base + "/reset-password", body: map[string]any{"token": strings.Repeat("a", 43), "password": "Replacement-pass123!"}},
		{method: http.MethodGet, path: base + "/recovery-email", token: token},
		{method: http.MethodPut, path: base + "/recovery-email", token: token, body: map[string]any{"email": "recovery@example.test"}},
	} {
		if rec := e.do(t, r); rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s still answers: %d", r.method, r.path, rec.Code)
		}
	}
	var columns int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_name='customer_identity' AND column_name LIKE 'recovery_email%'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("recovery email columns=%d %v", columns, err)
	}
	var purposes string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT string_agg(enumlabel,',' ORDER BY enumsortorder) FROM pg_enum WHERE enumtypid='customer_auth_purpose'::regtype`).Scan(&purposes); err != nil || purposes != "GOOGLE_SIGNUP" {
		t.Fatalf("customer_auth_purpose=%q %v", purposes, err)
	}
	if e.mail.count() != 0 || len(e.contacts.messages) != 0 {
		t.Fatal("registration sent a recovery message")
	}
}
