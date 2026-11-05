//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCustomerPortal_AdminIsolationAndConcurrentClaims(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) { cfg.CustomerPortal.BaseURL = "http://localhost:3001" })
	a, _ := e.seedAccount(t, "Portal A")
	b, _ := e.seedAccount(t, "Portal B")
	adminA := e.tokenFor(t, e.seedUser(t, a, domain.UserRoleAdmin))
	adminB := e.tokenFor(t, e.seedUser(t, b, domain.UserRoleAdmin))
	seller := e.tokenFor(t, e.seedUser(t, a, domain.UserRoleSeller))
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rec := e.do(t, request{method: method, path: "/v1/account/portal", token: seller, body: map[string]any{"subdomain": "seller"}})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("seller %s: %d %s", method, rec.Code, rec.Body)
		}
	}
	label := "p" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var codes [2]int
	var wg sync.WaitGroup
	for i, token := range []string{adminA, adminB} {
		wg.Add(1)
		go func(i int, token string) {
			defer wg.Done()
			codes[i] = e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": " " + strings.ToUpper(label) + " "}}).Code
		}(i, token)
	}
	wg.Wait()
	if !((codes[0] == 200 && codes[1] == 409) || (codes[0] == 409 && codes[1] == 200)) {
		t.Fatalf("concurrent claims: %v", codes)
	}
	for i, token := range []string{adminA, adminB} {
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/account/portal", token: token})
		var portal struct{ Hostname, URL string }
		if err := json.Unmarshal(rec.Body.Bytes(), &portal); err != nil {
			t.Fatal(err)
		}
		if codes[i] == 200 && (portal.Hostname != label+".localhost" || portal.URL != "http://"+label+".localhost:3001") {
			t.Fatalf("saved portal: %s", rec.Body)
		}
		if codes[i] == 409 && portal.Hostname != "" {
			t.Fatalf("other account changed: %s", rec.Body)
		}
	}
	for i, token := range []string{adminA, adminB} {
		hostname := label + ".localhost"
		if codes[i] == 409 {
			hostname = label + "b.localhost"
			rec := e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": label + "b"}})
			if rec.Code != 200 {
				t.Fatalf("second hostname: %d %s", rec.Code, rec.Body)
			}
		}
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + hostname + "/auth/google/options"})
		var options struct {
			SupplierName string `json:"supplier_name"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &options); err != nil {
			t.Fatal(err)
		}
		want := "Portal A"
		if i == 1 {
			want = "Portal B"
		}
		if rec.Code != 200 || options.SupplierName != want {
			t.Fatalf("hostname isolation %s: %d %s", hostname, rec.Code, rec.Body)
		}
	}
}

func TestCustomerPortal_ConfirmationLegacyAndChecklist(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) { cfg.CustomerPortal.BaseURL = "http://localhost:3001" })
	a, _ := e.seedAccount(t, "Portal checklist")
	token := e.tokenFor(t, e.seedUser(t, a, domain.UserRoleAdmin))
	ctx := context.Background()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO account_onboarding(account_id, status) VALUES($1, 'DISMISSED')`, a); err != nil {
		t.Fatal(err)
	}
	first := "p" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, label := range []string{"www", "api", "a.b", "-x"} {
		if rec := e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": label}}); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid %q: %d %s", label, rec.Code, rec.Body)
		}
	}
	for _, progress := range []map[string]any{
		{"step": "PORTAL", "step_status": "SKIPPED", "current_step": "FIRST_BRANCH"},
	} {
		if rec := e.do(t, request{method: http.MethodPut, path: "/v1/onboarding", token: token, body: progress}); rec.Code != 200 {
			t.Fatal(rec.Body)
		}
	}
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/onboarding", token: token})
	if !strings.Contains(rec.Body.String(), `"PORTAL":"SKIPPED"`) {
		t.Fatal(rec.Body)
	}
	rec = e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": first}})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/onboarding", token: token})
	if !strings.Contains(rec.Body.String(), `"PORTAL":"COMPLETED"`) {
		t.Fatalf("checklist not reconciled: %s", rec.Body)
	}
	for _, confirmed := range []bool{false, true} {
		rec = e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": first + "b", "expected_hostname": first + ".localhost", "confirm_change": confirmed}})
		want := 409
		if confirmed {
			want = 200
		}
		if rec.Code != want {
			t.Fatalf("confirmation %v: %d %s", confirmed, rec.Code, rec.Body)
		}
	}
	legacy := fmt.Sprintf("%s.legacy.test", a)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE account SET public_hostname=$2 WHERE id=$1`, a, legacy); err != nil {
		t.Fatal(err)
	}
	rec = e.do(t, request{method: http.MethodPut, path: "/v1/account/portal", token: token, body: map[string]any{"subdomain": first, "expected_hostname": legacy, "confirm_change": true}})
	if rec.Code != 403 {
		t.Fatalf("legacy overwritten: %d %s", rec.Code, rec.Body)
	}
}
