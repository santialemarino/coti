//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedAcceptedSale accepts a draft quote with its version frozen, straight on the owner pool.
func (e *env) seedAcceptedSale(t *testing.T, accountID, branchID uuid.UUID, seller domain.AppUser) draft {
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
	return seeded
}

// closureRow is the sale's closing facts as the database holds them.
type closureRow struct {
	status                    string
	paidAt, waivedAt, updated *time.Time
	paidBy, waivedBy          *uuid.UUID
	statusChanges             int
}

func (e *env) closureOf(t *testing.T, quoteID uuid.UUID) closureRow {
	t.Helper()
	var row closureRow
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT current_status::text, paid_at, paid_by_user_id, invoice_waived_at, invoice_waived_by_user_id,
		        updated_at, (SELECT count(*) FROM quote_status_change WHERE quote_id = quote.id)
		 FROM quote WHERE id = $1`, quoteID).
		Scan(&row.status, &row.paidAt, &row.paidBy, &row.waivedAt, &row.waivedBy, &row.updated,
			&row.statusChanges); err != nil {
		t.Fatalf("read the closure: %v", err)
	}
	return row
}

// closureView is the sale's closing facts as the order detail reports them.
type closureView struct {
	PaidAt          *time.Time `json:"paid_at"`
	PaidBy          *string    `json:"paid_by"`
	InvoiceWaivedAt *time.Time `json:"invoice_waived_at"`
	InvoiceWaivedBy *string    `json:"invoice_waived_by"`
	Invoiced        bool       `json:"invoiced"`
	InvoicePending  bool       `json:"invoice_pending"`
	SaleComplete    bool       `json:"sale_complete"`
}

func (e *env) closureView(t *testing.T, token, branch string, rfqID uuid.UUID) (closureView, closureView) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + rfqID.String(), token: token, branch: branch})
	var detail struct {
		Rfq closureView `json:"rfq"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &detail) != nil {
		t.Fatalf("detail: status = %d, body = %s", rec.Code, rec.Body)
	}
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/rfqs?include_archived=true", token: token, branch: branch})
	var list []struct {
		ID uuid.UUID `json:"id"`
		closureView
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("list: status = %d, body = %.300s", rec.Code, rec.Body)
	}
	for _, item := range list {
		if item.ID == rfqID {
			return detail.Rfq, item.closureView
		}
	}
	t.Fatalf("list misses the order %s", rfqID)
	return closureView{}, closureView{}
}

func quotePath(quoteID uuid.UUID, action string) string {
	return "/v1/quotes/" + quoteID.String() + "/" + action
}

// Paid and «No requiere factura» are facts with an author and a time: they move updated_at, leave the
// status and its history alone, and the detail and list derive the complete sale from them.
func TestSaleClosure_PaidAndWaiverRoundTrip(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Cierre")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedAcceptedSale(t, accountID, branchID, admin)
	token, branch := e.tokenFor(t, admin), branchID.String()
	before := e.closureOf(t, sale.quoteID)

	rec := e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, "payment"), token: token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Fatalf("mark paid: status = %d, body = %s", rec.Code, rec.Body)
	}
	paid := e.closureOf(t, sale.quoteID)
	if paid.paidAt == nil || paid.paidBy == nil || *paid.paidBy != admin.ID || !paid.updated.After(*before.updated) {
		t.Fatalf("after marking paid: %+v, want paid by %s and updated_at moved", paid, admin.ID)
	}
	if paid.status != "ACCEPTED" || paid.statusChanges != before.statusChanges {
		t.Fatalf("marking paid moved the status: %s, %d changes (was %d)", paid.status, paid.statusChanges, before.statusChanges)
	}
	detail, listed := e.closureView(t, token, branch, sale.rfqID)
	if detail.PaidBy == nil || *detail.PaidBy != admin.Name || detail.SaleComplete || listed.SaleComplete || listed.PaidAt == nil {
		t.Fatalf("paid, waiting for the invoice: detail = %+v, list = %+v, want paid by %q and not complete", detail, listed, admin.Name)
	}

	// Marking it again keeps the first mark.
	rec = e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, "payment"), token: token, branch: branch})
	if again := e.closureOf(t, sale.quoteID); rec.Code != http.StatusOK || !again.paidAt.Equal(*paid.paidAt) {
		t.Fatalf("second mark: status = %d, paid_at %v, want 200 and %v kept", rec.Code, again.paidAt, paid.paidAt)
	}

	rec = e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, "invoice-waiver"), token: token, branch: branch})
	waived := e.closureOf(t, sale.quoteID)
	if rec.Code != http.StatusOK || waived.waivedBy == nil || *waived.waivedBy != admin.ID || !waived.updated.After(*paid.updated) {
		t.Fatalf("waive: status = %d, row = %+v, want waived by %s and updated_at moved", rec.Code, waived, admin.ID)
	}
	detail, listed = e.closureView(t, token, branch, sale.rfqID)
	if !detail.SaleComplete || !listed.SaleComplete || detail.InvoiceWaivedBy == nil || detail.Invoiced {
		t.Fatalf("paid and waived: detail = %+v, list = %+v, want complete", detail, listed)
	}

	rec = e.do(t, request{method: http.MethodDelete, path: quotePath(sale.quoteID, "invoice-waiver"), token: token, branch: branch})
	if row := e.closureOf(t, sale.quoteID); rec.Code != http.StatusOK || row.waivedAt != nil || row.waivedBy != nil {
		t.Fatalf("undo the waiver: status = %d, row = %+v", rec.Code, row)
	}
	rec = e.do(t, request{method: http.MethodDelete, path: quotePath(sale.quoteID, "payment"), token: token, branch: branch})
	if row := e.closureOf(t, sale.quoteID); rec.Code != http.StatusOK || row.paidAt != nil || row.paidBy != nil {
		t.Fatalf("unmark paid: status = %d, row = %+v", rec.Code, row)
	}
	if detail, _ = e.closureView(t, token, branch, sale.rfqID); detail.SaleComplete || detail.PaidAt != nil {
		t.Fatalf("after undoing both: %+v, want an open sale", detail)
	}
}

// Every closing write is refused outside an accepted sale still in the list.
func TestSaleClosure_RefusedOutsideAnOpenAcceptedSale(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Abierto")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	open := e.seedDraftQuote(t, accountID, branchID, admin, nil)
	archived := e.seedAcceptedSale(t, accountID, branchID, admin)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET archived_at = now() WHERE id = $1`, archived.quoteID); err != nil {
		t.Fatal(err)
	}
	token, branch := e.tokenFor(t, admin), branchID.String()

	for _, quote := range []struct {
		id   uuid.UUID
		code domain.ErrorCode
	}{{open.quoteID, domain.CodeQuoteNotAccepted}, {archived.quoteID, domain.CodeQuoteArchived}} {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			for _, action := range []string{"payment", "invoice-waiver"} {
				rec := e.do(t, request{method: method, path: quotePath(quote.id, action), token: token, branch: branch})
				if rec.Code != http.StatusConflict || errorCode(t, rec) != string(quote.code) {
					t.Errorf("%s %s: status = %d, body = %s, want 409 %s", method, action, rec.Code, rec.Body, quote.code)
				}
			}
		}
		if row := e.closureOf(t, quote.id); row.paidAt != nil || row.waivedAt != nil {
			t.Errorf("a refused write left a mark: %+v", row)
		}
	}
}

// «No requiere factura» waits for invoicing to be on and is refused while the sale has a live
// invoice; an invoiced paid sale is complete, and so is any paid sale of an account that does not
// invoice with Coti.
func TestSaleClosure_WaiverAndCompletenessFollowTheInvoice(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Factura")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token, branch := e.tokenFor(t, admin), branchID.String()
	e.setInvoicing(t, accountID, true)

	pending := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusPending)
	invoiced := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusIssued)
	for _, sale := range []invoicedSale{pending, invoiced} {
		rec := e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, "invoice-waiver"), token: token, branch: branch})
		if rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeQuoteAlreadyInvoiced) {
			t.Fatalf("live invoice: status = %d, body = %s, want 409 QUOTE_ALREADY_INVOICED", rec.Code, rec.Body)
		}
	}

	var rfqID, pendingRfqID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT rfq_id FROM quote WHERE id = $1`, invoiced.quoteID).Scan(&rfqID); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT rfq_id FROM quote WHERE id = $1`, pending.quoteID).Scan(&pendingRfqID); err != nil {
		t.Fatal(err)
	}
	// The queue reads the voucher in flight, so it offers no second invoice beside it.
	if _, row := e.closureView(t, token, branch, pendingRfqID); !row.InvoicePending || row.Invoiced {
		t.Fatalf("pending: list row %+v, want the invoice pending and not invoiced", row)
	}
	if detail, row := e.closureView(t, token, branch, rfqID); !detail.Invoiced || detail.SaleComplete || row.InvoicePending {
		t.Fatalf("invoiced, unpaid: %+v, want invoiced and still open", detail)
	}
	if rec := e.do(t, request{method: http.MethodPost, path: quotePath(invoiced.quoteID, "payment"), token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("mark paid: status = %d, body = %s", rec.Code, rec.Body)
	}
	if detail, _ := e.closureView(t, token, branch, rfqID); !detail.SaleComplete {
		t.Fatalf("invoiced and paid: %+v, want complete", detail)
	}

	// A credit note cancels the invoice, so the paid sale waits for one again.
	noteID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO invoice (id, account_id, branch_id, quote_id, quote_version_id, environment, kind, status,
		                      invoice_type, point_of_sale, number, issued_on, cae, cae_expires_on, issuer_cuit,
		                      receiver_name, receiver_doc_type, receiver_iva_condition, net_amount, exempt_amount,
		                      vat_amount, total, currency, credited_invoice_id)
		 VALUES ($1, $2, $3, $4, $5, 'HOMOLOGATION', 'CREDIT_NOTE', 'ISSUED', 'B', 3, 24, current_date,
		         '74123456789013', current_date + 10, '30712345671', 'Obra Norte', 'NONE', 'FINAL_CONSUMER',
		         1000, 0, 210, 1210, 'ARS', $6)`,
		noteID, accountID, branchID, invoiced.quoteID, invoiced.versionID, invoiced.invoiceID); err != nil {
		t.Fatalf("seed the credit note: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM invoice WHERE id = $1`, noteID) })
	if detail, _ := e.closureView(t, token, branch, rfqID); detail.Invoiced || detail.SaleComplete {
		t.Fatalf("credited and paid: %+v, want not invoiced and open", detail)
	}

	e.setInvoicing(t, accountID, false)
	cash := e.seedAcceptedSale(t, accountID, branchID, admin)
	rec := e.do(t, request{method: http.MethodPost, path: quotePath(cash.quoteID, "invoice-waiver"), token: token, branch: branch})
	if rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeAccountInvoicingOff) {
		t.Fatalf("waiver with invoicing off: status = %d, body = %s, want 409 ACCOUNT_INVOICING_OFF", rec.Code, rec.Body)
	}
	if detail, _ := e.closureView(t, token, branch, cash.rfqID); detail.SaleComplete {
		t.Fatalf("unpaid with invoicing off: %+v, want open", detail)
	}
	if rec := e.do(t, request{method: http.MethodPost, path: quotePath(cash.quoteID, "payment"), token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("mark paid: status = %d, body = %s", rec.Code, rec.Body)
	}
	if detail, _ := e.closureView(t, token, branch, cash.rfqID); !detail.SaleComplete {
		t.Fatalf("paid with invoicing off: %+v, want complete", detail)
	}
}

// A paid sale is reopened only after its mark is taken back, and reopening clears the waiver.
func TestSaleClosure_ReactivationNeedsTheMarkTakenBack(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Reabre")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedAcceptedSale(t, accountID, branchID, admin)
	token, branch := e.tokenFor(t, admin), branchID.String()
	for _, action := range []string{"payment", "invoice-waiver"} {
		if rec := e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, action), token: token, branch: branch}); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", action, rec.Code, rec.Body)
		}
	}
	reactivate := request{method: http.MethodPost, path: quotePath(sale.quoteID, "reactivate"), token: token, branch: branch,
		body: map[string]any{"mode": "RESEND"}}

	rec := e.do(t, reactivate)
	if row := e.closureOf(t, sale.quoteID); rec.Code != http.StatusConflict || errorCode(t, rec) != string(domain.CodeQuoteAlreadyPaid) ||
		row.status != "ACCEPTED" || row.waivedAt == nil {
		t.Fatalf("paid: status = %d, body = %s, row = %+v, want 409 QUOTE_ALREADY_PAID and nothing moved", rec.Code, rec.Body, row)
	}

	if rec := e.do(t, request{method: http.MethodDelete, path: quotePath(sale.quoteID, "payment"), token: token, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("unmark: status = %d, body = %s", rec.Code, rec.Body)
	}
	rec = e.do(t, reactivate)
	if row := e.closureOf(t, sale.quoteID); rec.Code != http.StatusOK || row.status != "QUOTED" || row.waivedAt != nil || row.waivedBy != nil {
		t.Fatalf("unpaid: status = %d, body = %s, row = %+v, want QUOTED with the waiver cleared", rec.Code, rec.Body, row)
	}
}

// Another account cannot mark, unmark or waive a sale it cannot see: the route answers not found
// and the row keeps its facts.
func TestSaleClosure_WritesStayInsideTheAccount(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Dueño")
	owner := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedAcceptedSale(t, accountID, branchID, owner)
	ownerToken, branch := e.tokenFor(t, owner), branchID.String()
	if rec := e.do(t, request{method: http.MethodPost, path: quotePath(sale.quoteID, "payment"), token: ownerToken, branch: branch}); rec.Code != http.StatusOK {
		t.Fatalf("owner marks paid: status = %d", rec.Code)
	}
	before := e.closureOf(t, sale.quoteID)

	otherID, otherBranch := e.seedAccount(t, "Corralón Ajeno")
	e.setInvoicing(t, otherID, true)
	intruder := e.tokenFor(t, e.seedUser(t, otherID, domain.UserRoleAdmin))
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		for _, action := range []string{"payment", "invoice-waiver"} {
			rec := e.do(t, request{method: method, path: quotePath(sale.quoteID, action), token: intruder, branch: otherBranch.String()})
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s from another account: status = %d, body = %s, want 404", method, action, rec.Code, rec.Body)
			}
		}
	}
	if after := e.closureOf(t, sale.quoteID); after.paidAt == nil || !after.updated.Equal(*before.updated) || after.waivedAt != nil {
		t.Fatalf("another account's writes reached the row: %+v (was %+v)", after, before)
	}
}

// The «Facturar también» default is read and written with the invoicing settings, and a body that
// leaves it out keeps it.
func TestSaleClosure_InvoiceOnPaymentSetting(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón Default")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)
	read := func() bool {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/invoicing/settings", token: token})
		var body struct {
			InvoiceOnPayment *bool `json:"invoice_on_payment"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.InvoiceOnPayment == nil {
			t.Fatalf("settings: status = %d, body = %s", rec.Code, rec.Body)
		}
		return *body.InvoiceOnPayment
	}
	if read() {
		t.Fatal("a new account starts with «Facturar también» ticked")
	}
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/settings", token: token,
		body: map[string]any{"prices_include_vat": true, "invoice_on_payment": true}})
	if rec.Code != http.StatusOK || !read() {
		t.Fatalf("turn on: status = %d, body = %s", rec.Code, rec.Body)
	}
	rec = e.do(t, request{method: http.MethodPut, path: "/v1/invoicing/settings", token: token,
		body: map[string]any{"prices_include_vat": true}})
	if rec.Code != http.StatusOK || !read() {
		t.Fatalf("omitted: status = %d, body = %s, want the default kept", rec.Code, rec.Body)
	}

	// A seller reads it on the account, since the settings themselves are an admin's.
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/account", token: e.tokenFor(t, seller)})
	var account struct {
		InvoiceOnPayment *bool `json:"invoice_on_payment"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &account) != nil ||
		account.InvoiceOnPayment == nil || !*account.InvoiceOnPayment {
		t.Fatalf("seller reads the account: status = %d, body = %s, want invoice_on_payment true", rec.Code, rec.Body)
	}
}

// A voucher still waiting for ARCA keeps a paid, waived sale open: it may yet be issued.
func TestSaleClosure_APendingInvoiceKeepsTheSaleOpen(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Pendiente Cierre")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusPending)
	ctx := context.Background()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE quote SET paid_at = now(), paid_by_user_id = $2, invoice_waived_at = now(),
		                  invoice_waived_by_user_id = $2
		 WHERE id = $1`, sale.quoteID, admin.ID); err != nil {
		t.Fatal(err)
	}
	var rfqID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT rfq_id FROM quote WHERE id = $1`, sale.quoteID).Scan(&rfqID); err != nil {
		t.Fatal(err)
	}
	token, branch := e.tokenFor(t, admin), branchID.String()

	if detail, listed := e.closureView(t, token, branch, rfqID); detail.SaleComplete || listed.SaleComplete {
		t.Fatalf("paid, waived, invoice pending: detail = %+v, list = %+v, want open", detail, listed)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE invoice SET status = 'REJECTED' WHERE id = $1`, sale.invoiceID); err != nil {
		t.Fatal(err)
	}
	if detail, _ := e.closureView(t, token, branch, rfqID); !detail.SaleComplete {
		t.Fatalf("paid and waived once ARCA refused the invoice: %+v, want complete", detail)
	}
}
