//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// recoveryCaps sets the destination caps under test and allows codes back to back.
func recoveryCaps(perSupplier, failures int) func(*config.Config) {
	return func(c *config.Config) {
		c.CustomerAuth.ContactVerificationCooldown = time.Nanosecond
		c.CustomerAuth.ContactDestinationMax, c.CustomerAuth.ContactFailureMax = perSupplier, failures
	}
}

// recoverableCustomer registers a customer whose only contact is the given verified email.
func (e *env) recoverableCustomer(t *testing.T, name, email string) (string, uuid.UUID) {
	t.Helper()
	account, _ := e.seedAccount(t, name)
	host, _ := e.registerCustomer(t, account, recoveryTaxID)
	e.verifyCustomerContact(t, account, recoveryTaxID, domain.ChannelTypeEmail, email)
	options, _ := e.recoveryOptions(t, host, recoveryTaxID)
	if len(options) != 1 {
		t.Fatalf("options=%+v", options)
	}
	return host, options[0].ID
}

func (e *env) postRecovery(t *testing.T, host, step string, body map[string]any) (int, string) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/" + host + "/auth/recovery/" + step, body: body})
	return rec.Code, rec.Body.String()
}

func expectLocked(t *testing.T, status int, body, what string) {
	t.Helper()
	if status != http.StatusTooManyRequests || !strings.Contains(body, `"code":"ACCOUNT_LOCKED"`) {
		t.Fatalf("%s: %d %s, want 429 ACCOUNT_LOCKED", what, status, body)
	}
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func TestAccountRecovery_SupplierDestinationCapBlocksCodes(t *testing.T) {
	// The verification proof is the destination's first; two recovery codes fill the cap of three.
	e := newEnv(t, recoveryCaps(3, 1000))
	host, contact := e.recoverableCustomer(t, "Recovery cap", "cap+"+uuid.NewString()+"@example.test")
	for range 2 {
		e.requestRecoveryCode(t, host, recoveryTaxID, contact)
	}
	status, body := e.postRecovery(t, host, "code", map[string]any{"tax_id": recoveryTaxID, "contact_id": contact})
	expectLocked(t, status, body, "code past the supplier cap")
}

func TestAccountRecovery_WrongCodesCountTowardTheLockout(t *testing.T) {
	e := newEnv(t, recoveryCaps(1000, 2))
	host, contact := e.recoverableCustomer(t, "Recovery lockout", "lockout+"+uuid.NewString()+"@example.test")
	challenge, code := e.requestRecoveryCode(t, host, recoveryTaxID, contact)
	for range 2 {
		if status, _ := e.resetWithRecoveryCode(t, host, challenge, wrongCode(code), "Replacement-pass123!"); status != http.StatusUnauthorized {
			t.Fatalf("wrong code: %d", status)
		}
	}
	status, body := e.postRecovery(t, host, "reset", map[string]any{"id": challenge, "code": code, "password": "Replacement-pass123!"})
	expectLocked(t, status, body, "right code on a locked destination")
	status, body = e.postRecovery(t, host, "code", map[string]any{"tax_id": recoveryTaxID, "contact_id": contact})
	expectLocked(t, status, body, "new code for a locked destination")
}

func TestAccountRecovery_LockoutAtOneSupplierLeavesAnotherOpen(t *testing.T) {
	e := newEnv(t, recoveryCaps(1000, 2))
	email := "shared+" + uuid.NewString() + "@example.test"
	hostA, contactA := e.recoverableCustomer(t, "Recovery lockout A", email)
	hostB, contactB := e.recoverableCustomer(t, "Recovery lockout B", email)
	challengeA, codeA := e.requestRecoveryCode(t, hostA, recoveryTaxID, contactA)
	for range 2 {
		e.resetWithRecoveryCode(t, hostA, challengeA, wrongCode(codeA), "Replacement-pass123!")
	}
	status, body := e.postRecovery(t, hostA, "reset", map[string]any{"id": challengeA, "code": codeA, "password": "Replacement-pass123!"})
	expectLocked(t, status, body, "supplier A after its lockout")
	challengeB, codeB := e.requestRecoveryCode(t, hostB, recoveryTaxID, contactB)
	if status, _ := e.resetWithRecoveryCode(t, hostB, challengeB, codeB, "Replacement-pass123!"); status != http.StatusCreated {
		t.Fatalf("supplier A's lockout blocked recovery at supplier B: %d", status)
	}
}
