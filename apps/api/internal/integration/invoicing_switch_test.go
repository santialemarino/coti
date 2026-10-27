//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// invoicedSale is an accepted quote with one invoice recorded against it.
type invoicedSale struct {
	quoteID, versionID, invoiceID uuid.UUID
}

// seedInvoicedSale accepts a draft quote and records one invoice for it in status, straight on the
// owner pool: the switch decides what may be issued next, not what is already there.
func (e *env) seedInvoicedSale(
	t *testing.T, accountID, branchID uuid.UUID, seller domain.AppUser, status domain.InvoiceStatus,
) invoicedSale {
	t.Helper()
	ctx := context.Background()
	seeded := e.seedDraftQuote(t, accountID, branchID, seller, nil)
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote_version SET total = 1210, is_immutable = TRUE, frozen_at = now() WHERE id = $1`,
		seeded.versionID); err != nil {
		t.Fatalf("freeze the version: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote SET current_status = 'ACCEPTED' WHERE id = $1`, seeded.quoteID); err != nil {
		t.Fatalf("accept the quote: %v", err)
	}
	sale := invoicedSale{quoteID: seeded.quoteID, versionID: seeded.versionID, invoiceID: uuid.New()}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO invoice (id, account_id, branch_id, quote_id, quote_version_id, environment, kind,
		                      status, invoice_type, point_of_sale, number, issued_on, cae, cae_expires_on,
		                      issuer_cuit, receiver_name, receiver_doc_type, receiver_iva_condition,
		                      net_amount, exempt_amount, vat_amount, total, currency)
		 VALUES ($1, $2, $3, $4, $5, 'HOMOLOGATION', 'INVOICE', $6, 'B', 3,
		         CASE WHEN $6::invoice_status = 'ISSUED' THEN 23 END, current_date,
		         CASE WHEN $6::invoice_status = 'ISSUED' THEN '74123456789012' END,
		         CASE WHEN $6::invoice_status = 'ISSUED' THEN current_date + 10 END,
		         '30712345671', 'Obra Norte', 'NONE', 'FINAL_CONSUMER', 1000, 0, 210, 1210, 'ARS')`,
		sale.invoiceID, accountID, branchID, sale.quoteID, sale.versionID, status); err != nil {
		t.Fatalf("seed invoice: %v", err)
	}
	// Registered after the quote's own teardown, so it runs first and frees the quote.
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM invoice WHERE id = $1`, sale.invoiceID) })
	return sale
}

func (e *env) setInvoicing(t *testing.T, accountID uuid.UUID, enabled bool) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE account SET invoicing_enabled = $2 WHERE id = $1`, accountID, enabled); err != nil {
		t.Fatalf("set the switch: %v", err)
	}
}

func (e *env) invoicingEnabled(t *testing.T, accountID uuid.UUID) bool {
	t.Helper()
	var enabled bool
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT invoicing_enabled FROM account WHERE id = $1`, accountID).Scan(&enabled); err != nil {
		t.Fatalf("read the switch: %v", err)
	}
	return enabled
}

// Only an admin flips the switch; every user reads it on the account, with whether invoices exist.
func TestInvoicingSwitch_OnlyAnAdminFlipsItAndEveryoneReadsIt(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Switch")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/enabled", token: e.tokenFor(t, seller),
		body: map[string]any{"enabled": true}})
	if rec.Code != http.StatusForbidden || e.invoicingEnabled(t, accountID) {
		t.Fatalf("seller: status = %d, switch = %v, want 403 and the switch untouched", rec.Code, e.invoicingEnabled(t, accountID))
	}

	rec = e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/enabled", token: e.tokenFor(t, admin),
		body: map[string]any{}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no value: status = %d, want 400; body = %s", rec.Code, rec.Body)
	}

	rec = e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/enabled", token: e.tokenFor(t, admin),
		body: map[string]any{"enabled": true}})
	if rec.Code != http.StatusOK || !e.invoicingEnabled(t, accountID) {
		t.Fatalf("admin: status = %d, body = %s, want 200 and the switch on", rec.Code, rec.Body)
	}

	rec = e.do(t, request{method: http.MethodGet, path: "/v1/account", token: e.tokenFor(t, seller)})
	var account struct {
		InvoicingEnabled *bool `json:"invoicing_enabled"`
		HasInvoices      *bool `json:"has_invoices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &account); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("seller read: status = %d, err = %v", rec.Code, err)
	}
	if account.InvoicingEnabled == nil || !*account.InvoicingEnabled || account.HasInvoices == nil || *account.HasInvoices {
		t.Fatalf("seller reads %s, want invoicing_enabled true and has_invoices false", rec.Body)
	}
}

// Off refuses everything that would ask ARCA for something new, and leaves every issued record
// readable: the history, its PDF and the recovery of an invoice ARCA has not answered.
func TestInvoicingSwitch_OffRefusesIssuingAndKeepsTheRecords(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Apagado")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	sale := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusIssued)
	open := e.seedDraftQuote(t, accountID, branchID, admin, nil)
	token := e.tokenFor(t, admin)
	branch := branchID.String()

	refused := []request{
		{method: http.MethodGet, path: "/v1/quotes/" + open.quoteID.String() + "/invoice"},
		{method: http.MethodPost, path: "/v1/quotes/" + open.quoteID.String() + "/invoice", body: map[string]any{
			"fingerprint": strings.Repeat("a", 64), "version_id": open.versionID, "type": "B", "total": "1210.00"}},
		{method: http.MethodPost, path: "/v1/invoices/" + sale.invoiceID.String() + "/credit-note"},
	}
	for _, r := range refused {
		r.token, r.branch = token, branch
		rec := e.do(t, r)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeAccountInvoicingOff) {
			t.Errorf("%s %s: status = %d, body = %s, want 409 ACCOUNT_INVOICING_OFF", r.method, r.path, rec.Code, rec.Body)
		}
	}

	rec := e.do(t, request{method: http.MethodGet, path: "/v1/invoices", token: token, branch: branch})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), sale.invoiceID.String()) {
		t.Errorf("list: status = %d, body = %s, want the issued invoice", rec.Code, rec.Body)
	}
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/invoices/" + sale.invoiceID.String() + "/pdf",
		token: token, branch: branch})
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Errorf("pdf: status = %d, want 200 with a PDF; body = %.200s", rec.Code, rec.Body)
	}
	rec = e.do(t, request{method: http.MethodPost, path: "/v1/invoices/" + sale.invoiceID.String() + "/recover",
		token: token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Errorf("recover: status = %d, body = %s, want 200", rec.Code, rec.Body)
	}

	// Turned on, the same preview is answered by the invoicing rules rather than the switch.
	e.setInvoicing(t, accountID, true)
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/quotes/" + open.quoteID.String() + "/invoice",
		token: token, branch: branch})
	if rec.Code == http.StatusConflict && errorCode(t, rec) == string(domain.CodeAccountInvoicingOff) {
		t.Fatalf("with the switch on the preview is still refused: %s", rec.Body)
	}
}

// Switching off waits for ARCA: with an invoice pending it is refused and the switch stays on.
func TestInvoicingSwitch_OffIsRefusedWhileAnInvoiceIsPending(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Pendiente")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusPending)
	token := e.tokenFor(t, admin)

	rec := e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/enabled", token: token,
		body: map[string]any{"enabled": false}})
	if rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeInvoicePending) || !e.invoicingEnabled(t, accountID) {
		t.Fatalf("status = %d, body = %s, want 409 INVOICE_PENDING and the switch still on", rec.Code, rec.Body)
	}

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE invoice SET status = 'REJECTED' WHERE id = $1`, sale.invoiceID); err != nil {
		t.Fatal(err)
	}
	rec = e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/enabled", token: token,
		body: map[string]any{"enabled": false}})
	if rec.Code != http.StatusOK || e.invoicingEnabled(t, accountID) {
		t.Fatalf("status = %d, body = %s, want 200 and the switch off once ARCA answered", rec.Code, rec.Body)
	}
}
