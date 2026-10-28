//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// sharedDestinationCaps lifts the destination caps out of the way of tests that reuse fixed
// destinations on a database that outlives one run.
func sharedDestinationCaps(cfg config.CustomerAuthConfig) config.CustomerAuthConfig {
	cfg.ContactDestinationMax, cfg.ContactDestinationWindow = 1000, time.Hour
	cfg.ContactFailureMax, cfg.ContactLockout = 1000, time.Hour
	cfg.ContactSpamMax, cfg.ContactSpamWindow = 1000, time.Hour
	return cfg
}

// destinationCaps is a verification config with the given per-supplier, failure and cross-supplier
// caps, and a cooldown short enough to request back to back.
func destinationCaps(perSupplier, failures, allSuppliers int) config.CustomerAuthConfig {
	return config.CustomerAuthConfig{ContactVerificationTTL: time.Minute, ContactVerificationCooldown: time.Nanosecond, ContactVerificationAttempts: 5,
		ContactDestinationMax: perSupplier, ContactDestinationWindow: 24 * time.Hour,
		ContactFailureMax: failures, ContactLockout: time.Hour,
		ContactSpamMax: allSuppliers, ContactSpamWindow: 24 * time.Hour}
}

type destinationCustomer struct {
	principal domain.CustomerPrincipal
	verifier  *services.ClientContactVerificationService
	messenger *captureContactMessenger
}

// customersSharingDestination registers one customer at each of n suppliers, all declaring the same
// email address.
func (e *env) customersSharingDestination(t *testing.T, n int, cfg config.CustomerAuthConfig, destination string) []destinationCustomer {
	t.Helper()
	ctx := context.Background()
	customers := make([]destinationCustomer, 0, n)
	for i := range n {
		account, _ := e.seedAccount(t, fmt.Sprintf("Shared destination %d", i))
		const tax = "20123456786"
		e.registerCustomer(t, account, tax)
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

func (c destinationCustomer) request(destination string) error {
	_, err := c.verifier.Request(context.Background(), c.principal, domain.ChannelTypeEmail, destination)
	return err
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

func victimDestination() string {
	return "victim-" + uuid.NewString()[:8] + "@example.test"
}

func TestContactVerification_SupplierCapDoesNotBlockAnotherSupplier(t *testing.T) {
	e := newEnv(t)
	destination := victimDestination()
	customers := e.customersSharingDestination(t, 2, destinationCaps(2, 100, 100), destination)
	a, b := customers[0], customers[1]

	for range 2 {
		if err := a.request(destination); err != nil {
			t.Fatalf("request inside the supplier cap: %v", err)
		}
	}
	if err := a.request(destination); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("request past the supplier cap: %v", err)
	}
	if err := b.request(destination); err != nil {
		t.Fatalf("another supplier was capped by the first one's requests: %v", err)
	}
	if err := a.request("customer20123456786@example.test"); err != nil {
		t.Fatalf("another destination was capped too: %v", err)
	}
}

func TestContactVerification_CrossSupplierSpamCapHolds(t *testing.T) {
	e := newEnv(t)
	destination := victimDestination()
	customers := e.customersSharingDestination(t, 2, destinationCaps(100, 100, 3), destination)
	a, b := customers[0], customers[1]

	for _, step := range []destinationCustomer{a, a, b} {
		if err := step.request(destination); err != nil {
			t.Fatalf("request inside the spam cap: %v", err)
		}
	}
	for _, step := range []destinationCustomer{b, a} {
		if err := step.request(destination); !errors.Is(err, domain.ErrLocked) {
			t.Fatalf("request past the spam cap from either supplier: %v", err)
		}
	}
	if sent := len(a.messenger.messages) + len(b.messenger.messages); sent != 3 {
		t.Fatalf("destination received %d codes, want 3", sent)
	}
}

func TestContactVerification_LockoutAtOneSupplierDoesNotBlockAnother(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	destination := victimDestination()
	customers := e.customersSharingDestination(t, 2, destinationCaps(100, 3, 100), destination)
	a, b := customers[0], customers[1]

	proofA, err := a.verifier.Request(ctx, a.principal, domain.ChannelTypeEmail, destination)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := a.verifier.Confirm(ctx, a.principal, proofA.ID, "wrong"); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("wrong proof: %v", err)
		}
	}
	if err := a.verifier.Confirm(ctx, a.principal, proofA.ID, linkSecret(t, a.messenger.messages[0])); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("right proof inside the lockout: %v", err)
	}
	if err := a.request(destination); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("new code for a locked destination: %v", err)
	}

	proofB, err := b.verifier.Request(ctx, b.principal, domain.ChannelTypeEmail, destination)
	if err != nil {
		t.Fatalf("lockout at one supplier blocked a request at another: %v", err)
	}
	if err := b.verifier.Confirm(ctx, b.principal, proofB.ID, linkSecret(t, b.messenger.messages[0])); err != nil {
		t.Fatalf("lockout at one supplier blocked the owner at another: %v", err)
	}
	var verifiedAt []uuid.UUID
	rows, err := e.db.CrossAccount().Query(ctx, `SELECT account_id FROM client_contact WHERE value=$1 AND verified_at IS NOT NULL`, destination)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var account uuid.UUID
		if err := rows.Scan(&account); err != nil {
			t.Fatal(err)
		}
		verifiedAt = append(verifiedAt, account)
	}
	if rows.Err() != nil || len(verifiedAt) != 1 || verifiedAt[0] != b.principal.AccountID {
		t.Fatalf("verified at %v, want only the second supplier: %v", verifiedAt, rows.Err())
	}
}

// The cap is checked and the proof inserted in one transaction per request, so without the destination
// lock several requests read the same count and all pass.
func TestContactVerification_ConcurrentRequestsAtTheCapLetExactlyOneThrough(t *testing.T) {
	e := newEnv(t)
	destination := victimDestination()
	customers := e.customersSharingDestination(t, 6, destinationCaps(100, 100, 1), destination)

	start := make(chan struct{})
	results := make([]error, len(customers))
	var wg sync.WaitGroup
	for i, customer := range customers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = customer.request(destination)
		}()
	}
	close(start)
	wg.Wait()

	sent := 0
	for _, err := range results {
		switch {
		case err == nil:
			sent++
		case !errors.Is(err, domain.ErrLocked):
			t.Fatalf("request: %v", err)
		}
	}
	if sent != 1 {
		t.Fatalf("%d concurrent requests passed a cap of 1", sent)
	}
}

func TestClientContacts_ProfileContactCapIsEnforced(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, _ := e.seedAccount(t, "Contact cap")
	const tax = "20123456786"
	host, token := e.registerCustomer(t, account, tax)
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
	save := func(n int) *httptest.ResponseRecorder {
		return e.do(t, request{method: http.MethodPut, path: base + "/profile", token: token, body: map[string]any{"name": fmt.Sprintf("Juan Perez %d", n), "contacts": contacts(n)}})
	}

	if over := save(11); over.Code != http.StatusUnprocessableEntity || codeOf(over.Body.Bytes()) != string(domain.CodeContactLimit) {
		t.Fatalf("profile past the cap: %d %s", over.Code, over.Body)
	}
	if at := save(10); at.Code != http.StatusNoContent {
		t.Fatalf("profile at the cap: %d %s", at.Code, at.Body)
	}

	// A merge can leave a profile above the cap; saving it as it stands must still work.
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source)
 SELECT account_id,id,'EMAIL',v,FALSE,'SELLER' FROM client, unnest(ARRAY['cap10@example.test','cap11@example.test']) AS v WHERE account_id=$1 AND tax_id=$2`, account, tax); err != nil {
		t.Fatal(err)
	}
	if merged := save(12); merged.Code != http.StatusNoContent {
		t.Fatalf("profile a merge took past the cap: %d %s", merged.Code, merged.Body)
	}
	if grown := save(13); grown.Code != http.StatusUnprocessableEntity || codeOf(grown.Body.Bytes()) != string(domain.CodeContactLimit) {
		t.Fatalf("profile grown past its stored count: %d %s", grown.Code, grown.Body)
	}

	other, _ := e.seedAccount(t, "Contact cap signup")
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE account SET public_hostname=$2 WHERE id=$1`, other, "cap-"+other.String()+".coti.test"); err != nil {
		t.Fatal(err)
	}
	signup := e.do(t, request{method: http.MethodPost, path: "/v1/customer/sites/cap-" + other.String() + ".coti.test/auth/register", body: map[string]any{"tax_id": tax, "password": "Customer-pass123!", "name": "Juan Perez", "contacts": contacts(11)}})
	if signup.Code != http.StatusUnprocessableEntity || codeOf(signup.Body.Bytes()) != string(domain.CodeContactLimit) {
		t.Fatalf("signup past the cap: %d %s", signup.Code, signup.Body)
	}
}
