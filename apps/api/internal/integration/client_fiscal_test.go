//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// clientFiscalRow is what the client row holds after a request, read on the owner pool.
type clientFiscalRow struct {
	legalName, taxID, ivaCondition, address *string
}

func (r clientFiscalRow) String() string {
	return fmt.Sprintf("{legal_name: %s, tax_id: %s, iva_condition: %s, address: %s}",
		text(r.legalName), text(r.taxID), text(r.ivaCondition), text(r.address))
}

// text reads a nullable column for a failure message.
func text(value *string) string {
	if value == nil {
		return "<null>"
	}
	return *value
}

func (e *env) seedFiscalClient(t *testing.T, accountID uuid.UUID, taxID *string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO client (id, account_id, name, tax_id) VALUES ($1, $2, 'Obra Norte', $3)`,
		id, accountID, taxID); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return id
}

func (e *env) clientFiscal(t *testing.T, clientID uuid.UUID) clientFiscalRow {
	t.Helper()
	var row clientFiscalRow
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT legal_name, tax_id, iva_condition::text, fiscal_address FROM client WHERE id = $1`, clientID,
	).Scan(&row.legalName, &row.taxID, &row.ivaCondition, &row.address); err != nil {
		t.Fatalf("read client: %v", err)
	}
	return row
}

// A CUIT enters a client only through the customer's own account: the fiscal route refuses one,
// keeps the one on file, and still saves everything else, inside the caller's account only.
func TestClientFiscal_CUITIsNeverWrittenByStaff(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Fiscal")
	otherID, _ := e.seedAccount(t, "Corralón Vecino")
	token := e.tokenFor(t, e.seedUser(t, accountID, domain.UserRoleSeller))
	cuit := "20123456786"
	withCUIT := e.seedFiscalClient(t, accountID, &cuit)
	without := e.seedFiscalClient(t, accountID, nil)
	path := func(id uuid.UUID) string { return "/v1/clients/" + id.String() + "/fiscal" }

	rec := e.do(t, request{method: http.MethodPut, path: path(without), token: token,
		body: map[string]any{"tax_id": "30712345671", "legal_name": "Obra Norte SA"}})
	if rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != string(domain.CodeTaxIDReadOnly) {
		t.Fatalf("tax_id in the body: status = %d, body = %s, want 422 TAX_ID_READ_ONLY", rec.Code, rec.Body)
	}
	if row := e.clientFiscal(t, without); row.taxID != nil || row.legalName != nil {
		t.Fatalf("a refused body wrote tax_id = %v, legal_name = %v", text(row.taxID), text(row.legalName))
	}

	rec = e.do(t, request{method: http.MethodPut, path: path(withCUIT), token: token,
		body: map[string]any{"legal_name": "Obra Norte SA", "iva_condition": "REGISTERED", "address": "Av. Siempreviva 742", "dni": "28123456"}})
	row := e.clientFiscal(t, withCUIT)
	if rec.Code != http.StatusOK || row.taxID == nil || *row.taxID != cuit {
		t.Fatalf("status = %d, tax_id = %v, want 200 and the CUIT on file kept", rec.Code, text(row.taxID))
	}
	if row.legalName == nil || *row.legalName != "Obra Norte SA" || row.ivaCondition == nil || *row.ivaCondition != "REGISTERED" ||
		row.address == nil || *row.address != "Av. Siempreviva 742" {
		t.Fatalf("row = %s, want the other fiscal fields saved", row.String())
	}
	// Omitted clears, except a CUIT: nothing a seller sends removes it.
	if rec = e.do(t, request{method: http.MethodPut, path: path(withCUIT), token: token, body: map[string]any{}}); rec.Code != http.StatusOK {
		t.Fatalf("empty body: status = %d, body = %s", rec.Code, rec.Body)
	}
	if row = e.clientFiscal(t, withCUIT); row.taxID == nil || *row.taxID != cuit || row.legalName != nil {
		t.Fatalf("after an empty body tax_id = %v, legal_name = %v, want the CUIT kept and the rest cleared", text(row.taxID), text(row.legalName))
	}

	// A consumidor final's DNI stays the seller's to load and clear.
	if rec = e.do(t, request{method: http.MethodPut, path: path(without), token: token,
		body: map[string]any{"dni": "28.123.456"}}); rec.Code != http.StatusOK {
		t.Fatalf("dni: status = %d, body = %s", rec.Code, rec.Body)
	}
	if row = e.clientFiscal(t, without); row.taxID == nil || *row.taxID != "28123456" {
		t.Fatalf("tax_id = %v, want the DNI stored", text(row.taxID))
	}
	rec = e.do(t, request{method: http.MethodPut, path: path(without), token: token, body: map[string]any{"dni": "20123456786"}})
	if rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != string(domain.CodeInvalidTaxID) {
		t.Fatalf("a CUIT as DNI: status = %d, body = %s, want 422 INVALID_TAX_ID", rec.Code, rec.Body)
	}
	if rec = e.do(t, request{method: http.MethodPut, path: path(without), token: token, body: map[string]any{}}); rec.Code != http.StatusOK {
		t.Fatalf("clear: status = %d", rec.Code)
	}
	if row = e.clientFiscal(t, without); row.taxID != nil {
		t.Fatalf("tax_id = %v, want the DNI cleared", text(row.taxID))
	}

	// Another account's seller cannot reach the client at all.
	foreign := e.tokenFor(t, e.seedUser(t, otherID, domain.UserRoleAdmin))
	rec = e.do(t, request{method: http.MethodPut, path: path(without), token: foreign, body: map[string]any{"legal_name": "Ajeno"}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-account: status = %d, want 404", rec.Code)
	}
	if row = e.clientFiscal(t, without); row.legalName != nil {
		t.Fatalf("a cross-account request wrote legal_name = %v", *row.legalName)
	}
}

// The preview offers the supplier's sign-up page exactly when the invoice waits on the buyer's CUIT,
// and an invoice B below the identification threshold goes to an unidentified buyer as before.
func TestClientFiscal_PreviewOffersSignupOnlyWhenACUITIsMissing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, branchID := e.seedAccount(t, "Corralón Registro")
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE account SET iva_condition = 'REGISTERED', public_hostname = 'registro.example.com' WHERE id = $1`, accountID); err != nil {
		t.Fatal(err)
	}
	clientID := e.seedFiscalClient(t, accountID, nil)
	productID := e.seedProduct(t, accountID, "Cemento", "Cemento 50 kg")
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, nil)
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description, quantity, unit_price_snapshot, subtotal)
		 VALUES ($1, $2, $3, 'Cemento', 1, 1000, 1000)`, accountID, seeded.versionID, productID); err != nil {
		t.Fatalf("seed line: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote_version SET total = 1000, is_immutable = TRUE, frozen_at = now() WHERE id = $1`, seeded.versionID); err != nil {
		t.Fatalf("freeze the version: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote SET current_status = 'ACCEPTED', client_id = $2 WHERE id = $1`, seeded.quoteID, clientID); err != nil {
		t.Fatal(err)
	}
	preview := func() (issues []string, signupURL string) {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/quotes/" + seeded.quoteID.String() + "/invoice",
			token: e.tokenFor(t, seller), branch: branchID.String()})
		var body struct {
			Issues    []string `json:"issues"`
			SignupURL string   `json:"customer_signup_url"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("preview: status = %d, body = %s", rec.Code, rec.Body)
		}
		return body.Issues, body.SignupURL
	}

	issues, url := preview()
	if slices.Contains(issues, "RECEIVER_ID_REQUIRED") || slices.Contains(issues, "RECEIVER_CUIT_REQUIRED") || url != "" {
		t.Fatalf("B below the threshold: issues = %v, url = %q, want no receiver issue and no link", issues, url)
	}

	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE client SET iva_condition = 'REGISTERED', legal_name = 'Obra Norte SA', fiscal_address = 'Main 123' WHERE id = $1`, clientID); err != nil {
		t.Fatal(err)
	}
	issues, url = preview()
	if !slices.Contains(issues, "RECEIVER_CUIT_REQUIRED") || url != "https://registro.example.com/signup" {
		t.Fatalf("A without CUIT: issues = %v, url = %q, want RECEIVER_CUIT_REQUIRED and the supplier's sign-up page", issues, url)
	}
}
