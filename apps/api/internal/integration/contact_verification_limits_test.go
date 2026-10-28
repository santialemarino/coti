//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// sharedDestinationCaps lifts the cross-account caps out of the way of tests that reuse fixed
// destinations on a database that outlives one run.
func sharedDestinationCaps(cfg config.CustomerAuthConfig) config.CustomerAuthConfig {
	cfg.ContactDestinationMax, cfg.ContactDestinationWindow = 1000, time.Hour
	cfg.ContactFailureMax, cfg.ContactLockout = 1000, time.Hour
	return cfg
}

type destinationCustomer struct {
	principal domain.CustomerPrincipal
	verifier  *services.ClientContactVerificationService
	messenger *captureContactMessenger
}

// customersSharingDestination registers one customer at each of two suppliers, both declaring the
// same email address.
func (e *env) customersSharingDestination(t *testing.T, cfg config.CustomerAuthConfig, destination string) []destinationCustomer {
	t.Helper()
	ctx := context.Background()
	customers := make([]destinationCustomer, 0, 2)
	for i := range 2 {
		account, _ := e.seedAccount(t, fmt.Sprintf("Shared destination %d", i))
		const tax = "20123456786"
		e.registerCustomer(t, account, tax, "")
		p := domain.CustomerPrincipal{AccountID: account, TaxID: tax}
		if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&p.CustomerID); err != nil {
			t.Fatal(err)
		}
		store := repository.NewClientContactRepository()
		service := services.NewClientContactService(e.db, repository.NewClientRepository(), store)
		if err := service.UpdateProfile(ctx, p, domain.ClientContactUpdate{Name: "Juan Perez", Contacts: []domain.ClientContact{
			{Kind: domain.ChannelTypeEmail, Value: "customer" + tax + "@example.test", Primary: true},
			{Kind: domain.ChannelTypeEmail, Value: destination},
		}}); err != nil {
			t.Fatal(err)
		}
		messenger := &captureContactMessenger{}
		customers = append(customers, destinationCustomer{p, services.NewClientContactVerificationService(service, store, messenger, cfg), messenger})
	}
	return customers
}

func linkSecret(t *testing.T, message domain.ContactMessage) string {
	t.Helper()
	fields := strings.Fields(message.Body)
	link, err := url.Parse(fields[len(fields)-1])
	if err != nil {
		t.Fatal(err)
	}
	return link.Query().Get("token")
}

func TestContactVerification_DestinationCapHoldsAcrossAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	destination := "victim-" + uuid.NewString()[:8] + "@example.test"
	cfg := config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Nanosecond, ContactVerificationAttempts: 3,
		ContactDestinationMax: 3, ContactDestinationWindow: 24 * time.Hour, ContactFailureMax: 100, ContactLockout: time.Hour}
	customers := e.customersSharingDestination(t, cfg, destination)
	a, b := customers[0], customers[1]

	for _, step := range []destinationCustomer{a, a, b} {
		if _, err := step.verifier.Request(ctx, step.principal, domain.ChannelTypeEmail, destination); err != nil {
			t.Fatalf("request inside the cap: %v", err)
		}
	}
	for _, step := range []destinationCustomer{b, a} {
		if _, err := step.verifier.Request(ctx, step.principal, domain.ChannelTypeEmail, destination); !errors.Is(err, domain.ErrLocked) {
			t.Fatalf("request past the cap from either supplier: %v", err)
		}
	}
	if sent := len(a.messenger.messages) + len(b.messenger.messages); sent != 3 {
		t.Fatalf("destination received %d codes, want 3", sent)
	}
	if _, err := a.verifier.Request(ctx, a.principal, domain.ChannelTypeEmail, "customer20123456786@example.test"); err != nil {
		t.Fatalf("another destination was capped too: %v", err)
	}
}

func TestContactVerification_FailureLockoutHoldsAcrossAccounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	destination := "victim-" + uuid.NewString()[:8] + "@example.test"
	cfg := config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Nanosecond, ContactVerificationAttempts: 5,
		ContactDestinationMax: 100, ContactDestinationWindow: 24 * time.Hour, ContactFailureMax: 3, ContactLockout: time.Hour}
	customers := e.customersSharingDestination(t, cfg, destination)
	a, b := customers[0], customers[1]

	proofA, err := a.verifier.Request(ctx, a.principal, domain.ChannelTypeEmail, destination)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := a.verifier.Confirm(ctx, a.principal, proofA.ID, "wrong"); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("wrong proof: %v", err)
		}
	}
	proofB, err := b.verifier.Request(ctx, b.principal, domain.ChannelTypeEmail, destination)
	if err != nil {
		t.Fatalf("lock before the failure cap: %v", err)
	}
	if err := b.verifier.Confirm(ctx, b.principal, proofB.ID, "wrong"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("wrong proof: %v", err)
	}
	if err := b.verifier.Confirm(ctx, b.principal, proofB.ID, linkSecret(t, b.messenger.messages[0])); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("right proof inside the lockout: %v", err)
	}
	if _, err := a.verifier.Request(ctx, a.principal, domain.ChannelTypeEmail, destination); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("new code for a locked destination: %v", err)
	}
	var verified bool
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact WHERE value=$1 AND verified_at IS NOT NULL)`, destination).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if verified {
		t.Fatal("a locked destination was verified")
	}
}

func TestClientContacts_ProfileContactCapIsEnforced(t *testing.T) {
	e := newEnv(t)
	account, _ := e.seedAccount(t, "Contact cap")
	host, token := e.registerCustomer(t, account, "20123456786", "")
	base := "/v1/customer/sites/" + host
	contacts := func(n int) []map[string]any {
		list := make([]map[string]any, 0, n)
		for i := range n {
			list = append(list, map[string]any{"kind": "EMAIL", "value": fmt.Sprintf("cap%d@example.test", i), "primary": i == 0})
		}
		return list
	}
	codeOf := func(body []byte) string {
		var response struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &response)
		return response.Code
	}

	over := e.do(t, request{method: http.MethodPut, path: base + "/profile", token: token, body: map[string]any{"name": "Juan Perez", "contacts": contacts(11)}})
	if over.Code != http.StatusUnprocessableEntity || codeOf(over.Body.Bytes()) != string(domain.CodeContactLimit) {
		t.Fatalf("profile past the cap: %d %s", over.Code, over.Body)
	}
	if at := e.do(t, request{method: http.MethodPut, path: base + "/profile", token: token, body: map[string]any{"name": "Juan Perez", "contacts": contacts(10)}}); at.Code != http.StatusNoContent {
		t.Fatalf("profile at the cap: %d %s", at.Code, at.Body)
	}

	other, _ := e.seedAccount(t, "Contact cap signup")
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE account SET public_hostname=$2 WHERE id=$1`, other, "cap-"+other.String()+".coti.test"); err != nil {
		t.Fatal(err)
	}
	signup := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/cap-" + other.String() + ".coti.test/auth/register", body: map[string]any{"tax_id": "20123456786", "password": "Customer-pass123!", "name": "Juan Perez", "contacts": contacts(11)}})
	if signup.Code != http.StatusUnprocessableEntity || codeOf(signup.Body.Bytes()) != string(domain.CodeContactLimit) {
		t.Fatalf("signup past the cap: %d %s", signup.Code, signup.Body)
	}
}
