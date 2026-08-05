//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func accountBody(name string, overrides map[string]any) map[string]any {
	body := map[string]any{"name": name}
	for key, value := range overrides {
		body[key] = value
	}
	return body
}

// The profile the backoffice edits, end to end: every field the request carries lands, and the
// response is what the screen re-renders from.
func TestAccount_UpdateReplacesTheProfileAndBrand(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: e.tokenFor(t, admin),
		body: accountBody("Corralón San Martín", map[string]any{
			"legal_name":     "Corralón San Martín S.R.L.",
			"tax_id":         "30-71234567-9",
			"brand_logo_url": "https://cdn.corralon.test/logo.png",
			"brand_color":    "#C2410C",
		})})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	var body struct {
		Name         string  `json:"name"`
		LegalName    *string `json:"legal_name"`
		TaxID        *string `json:"tax_id"`
		BrandLogoURL *string `json:"brand_logo_url"`
		BrandColor   *string `json:"brand_color"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Name != "Corralón San Martín" {
		t.Errorf("name = %q, want the updated one", body.Name)
	}
	for field, got := range map[string]*string{
		"legal_name": body.LegalName, "tax_id": body.TaxID,
		"brand_logo_url": body.BrandLogoURL, "brand_color": body.BrandColor,
	} {
		if got == nil {
			t.Errorf("%s = null, want the value that was sent", field)
		}
	}

	var stored string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT brand_color FROM account WHERE id = $1`, accountID).Scan(&stored); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if stored != "#C2410C" {
		t.Errorf("stored brand_color = %q, want #C2410C", stored)
	}
}

// Omitting an optional field clears it, because the route replaces the record rather than patching
// it — which is what lets a corralón remove a logo it no longer wants on its quotes.
func TestAccount_UpdateClearsAnOmittedField(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	if rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Corralón A", map[string]any{"tax_id": "30-71234567-9"})}); rec.Code != http.StatusOK {
		t.Fatalf("first update: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Corralón A", nil)}); rec.Code != http.StatusOK {
		t.Fatalf("second update: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	var taxID *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT tax_id FROM account WHERE id = $1`, accountID).Scan(&taxID); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if taxID != nil {
		t.Errorf("tax_id = %q, want NULL once it was left out", *taxID)
	}
}

/*
 * The brand is rendered by the client webapp, so a malformed colour or logo would break a screen
 * a customer sees rather than one the corralón can fix. Both formats are refused at the edge.
 */
func TestAccount_UpdateRefusesAMalformedBrand(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	refused := []struct {
		name string
		body map[string]any
	}{
		{"colour without a hash", accountBody("Corralón A", map[string]any{"brand_color": "C2410C"})},
		{"colour that is a name", accountBody("Corralón A", map[string]any{"brand_color": "naranja"})},
		{"colour of the wrong length", accountBody("Corralón A", map[string]any{"brand_color": "#C2410"})},
		{"logo that is not a url", accountBody("Corralón A", map[string]any{"brand_logo_url": "cdn.corralon.test/logo.png"})},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token, body: tc.body})
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}

	accepted := []string{"#C2410C", "#c2410c", "#FFF"}
	for _, colour := range accepted {
		t.Run("accepts "+colour, func(t *testing.T) {
			rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
				body: accountBody("Corralón A", map[string]any{"brand_color": colour})})
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
			}
		})
	}

	var stored *string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT brand_color FROM account WHERE id = $1`, accountID).Scan(&stored); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if stored == nil || *stored != "#FFF" {
		t.Errorf("stored brand_color = %v, want the last accepted one", stored)
	}
}

// A name of nothing but spaces would leave the corralón with a blank identity on every screen and
// every quote, so the service refuses it after trimming — 422, not 400: the field is present.
func TestAccount_UpdateRefusesABlankName(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: e.tokenFor(t, admin),
		body: accountBody("   ", nil)})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusUnprocessableEntity, rec.Body)
	}
}

// Reading the account is not admin-only, because the shell names the corralón. Writing it is.
func TestAccount_UpdateIsAdminOnlyAndReadIsNot(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	token := e.tokenFor(t, seller)

	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/account", token: token}); rec.Code != http.StatusOK {
		t.Errorf("GET as seller: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: token,
		body: accountBody("Otro nombre", nil)})
	if rec.Code != http.StatusForbidden {
		t.Errorf("PUT as seller: status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body)
	}

	var name string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT name FROM account WHERE id = $1`, accountID).Scan(&name); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if name != "Corralón A" {
		t.Errorf("name = %q, want it unchanged by the refused write", name)
	}
}

// The route carries no account id — it writes whatever the session resolved — so this pins that
// there is nothing on the wire another account could name.
func TestAccount_UpdateTouchesOnlyTheCallersAccount(t *testing.T) {
	e := newEnv(t)
	accountA, _ := e.seedAccount(t, "Corralón A")
	accountB, _ := e.seedAccount(t, "Corralón B")
	admin := e.seedUser(t, accountA, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/account", token: e.tokenFor(t, admin),
		body: accountBody("Corralón A renombrado", map[string]any{"brand_color": "#123456"})})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	for _, tc := range []struct {
		id       uuid.UUID
		wantName string
	}{{accountA, "Corralón A renombrado"}, {accountB, "Corralón B"}} {
		var name string
		var colour *string
		if err := e.db.CrossAccount().QueryRow(context.Background(),
			`SELECT name, brand_color FROM account WHERE id = $1`, tc.id).Scan(&name, &colour); err != nil {
			t.Fatalf("read account: %v", err)
		}
		if name != tc.wantName {
			t.Errorf("account %v name = %q, want %q", tc.id, name, tc.wantName)
		}
		if tc.id == accountB && colour != nil {
			t.Errorf("account B brand_color = %q, want it untouched", *colour)
		}
	}
}
