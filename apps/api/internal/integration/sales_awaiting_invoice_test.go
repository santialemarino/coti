//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const awaitingInvoicePath = "/v1/quotes/awaiting-invoice"

// awaitingPage is one page of GET /v1/quotes/awaiting-invoice.
type awaitingPage struct {
	Items []struct {
		QuoteID    uuid.UUID  `json:"quote_id"`
		RFQID      uuid.UUID  `json:"rfq_id"`
		BranchID   uuid.UUID  `json:"branch_id"`
		BranchName string     `json:"branch_name"`
		ClientID   *uuid.UUID `json:"client_id"`
		Client     string     `json:"client"`
		Total      string     `json:"total"`
		AcceptedAt time.Time  `json:"accepted_at"`
		PaidAt     *time.Time `json:"paid_at"`
	} `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

func (p awaitingPage) quoteIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(p.Items))
	for i, item := range p.Items {
		ids[i] = item.QuoteID
	}
	return ids
}

func (e *env) awaitingInvoice(t *testing.T, token, branch, query string) awaitingPage {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: awaitingInvoicePath + query, token: token, branch: branch})
	var page awaitingPage
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("awaiting invoice%s: status = %d, body = %.300s", query, rec.Code, rec.Body)
	}
	return page
}

// acceptOn records the sale's acceptance at day, which is what the list reads as its date.
func (e *env) acceptOn(t *testing.T, accountID, quoteID uuid.UUID, day time.Time) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO quote_status_change (account_id, quote_id, previous_status, new_status, changed_at)
		 VALUES ($1, $2, 'SENT', 'ACCEPTED', $3)`, accountID, quoteID, day); err != nil {
		t.Fatalf("record the acceptance: %v", err)
	}
}

// seedIssuedSale is seedInvoicedSale with the invoice authorized under number, so one account can
// hold several issued invoices.
func (e *env) seedIssuedSale(t *testing.T, accountID, branchID uuid.UUID, seller domain.AppUser, number int) invoicedSale {
	t.Helper()
	sale := e.seedInvoicedSale(t, accountID, branchID, seller, domain.InvoiceStatusRejected)
	e.exec(t, `UPDATE invoice SET status = 'ISSUED', number = $2, cae = '74123456789012',
	             cae_expires_on = current_date + 10 WHERE id = $1`, sale.invoiceID, number)
	return sale
}

func (e *env) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// The view holds exactly the accepted, unarchived sales nothing stands for: an issued invoice, one
// waiting for ARCA or a waiver each take a sale out, while a refused invoice or one a credit note
// cancelled leaves it waiting. Paid sales come first, then the longest accepted.
func TestSalesAwaitingInvoice_ListsWhatWaitsPaidFirstThenOldest(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Por Facturar")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	token, branch := e.tokenFor(t, admin), branchID.String()
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }

	unpaidOld := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.acceptOn(t, accountID, unpaidOld.quoteID, day(1))
	rejected := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusRejected)
	e.acceptOn(t, accountID, rejected.quoteID, day(2))
	credited := e.seedIssuedSale(t, accountID, branchID, admin, 41)
	e.seedCreditNote(t, credited, domain.InvoiceStatusIssued)
	e.acceptOn(t, accountID, credited.quoteID, day(3))
	unpaidNew := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.acceptOn(t, accountID, unpaidNew.quoteID, day(4))
	paidOld := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.acceptOn(t, accountID, paidOld.quoteID, day(5))
	paidNew := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.acceptOn(t, accountID, paidNew.quoteID, day(6))
	// A sale reactivated and accepted again dates from its latest acceptance.
	e.acceptOn(t, accountID, paidNew.quoteID, day(7))
	e.exec(t, `UPDATE quote SET paid_at = now(), paid_by_user_id = $2 WHERE id = ANY($1)`,
		[]uuid.UUID{paidOld.quoteID, paidNew.quoteID}, admin.ID)

	waived := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.exec(t, `UPDATE quote SET invoice_waived_at = now(), invoice_waived_by_user_id = $2 WHERE id = $1`,
		waived.quoteID, admin.ID)
	invoiced := e.seedIssuedSale(t, accountID, branchID, admin, 42)
	pending := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusPending)
	archived := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.exec(t, `UPDATE quote SET archived_at = now() WHERE id = $1`, archived.quoteID)
	sent := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.exec(t, `UPDATE quote SET current_status = 'SENT' WHERE id = $1`, sent.quoteID)
	draft := e.seedDraftQuote(t, accountID, branchID, admin, nil)

	page := e.awaitingInvoice(t, token, branch, "")
	want := []uuid.UUID{paidOld.quoteID, paidNew.quoteID, unpaidOld.quoteID, rejected.quoteID,
		credited.quoteID, unpaidNew.quoteID}
	if got := page.quoteIDs(); !slices.Equal(got, want) || page.Total != len(want) {
		t.Fatalf("awaiting = %v (total %d), want %v; waived %s, invoiced %s, pending %s, archived %s, sent %s, draft %s",
			got, page.Total, want, waived.quoteID, invoiced.quoteID, pending.quoteID, archived.quoteID,
			sent.quoteID, draft.quoteID)
	}
	first := page.Items[0]
	if first.RFQID != paidOld.rfqID || first.BranchID != branchID || first.BranchName != "Corralón Por Facturar Centro" ||
		first.Total != "1210.00" || !first.AcceptedAt.Equal(day(5)) || first.PaidAt == nil {
		t.Fatalf("first row = %+v, want the paid sale's order, branch, total and acceptance", first)
	}
	if !page.Items[1].AcceptedAt.Equal(day(7)) || page.Items[2].PaidAt != nil {
		t.Fatalf("rows = %+v, want the latest acceptance and the unpaid sale unpaid", page.Items)
	}

	// Waiving from the view takes the sale out of it.
	rec := e.do(t, request{method: http.MethodPost, path: quotePath(unpaidOld.quoteID, "invoice-waiver"), token: token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Fatalf("waive: status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := e.awaitingInvoice(t, token, branch, "").quoteIDs(); slices.Contains(got, unpaidOld.quoteID) {
		t.Fatalf("after the waiver: %v still lists %s", got, unpaidOld.quoteID)
	}
}

// An account that does not invoice has nothing waiting for an invoice.
func TestSalesAwaitingInvoice_EmptyWhileInvoicingIsOff(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Sin Facturación")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	sale := e.seedAcceptedSale(t, accountID, branchID, admin)
	token := e.tokenFor(t, admin)
	if got := e.awaitingInvoice(t, token, "", "").quoteIDs(); !slices.Equal(got, []uuid.UUID{sale.quoteID}) {
		t.Fatalf("invoicing on: %v, want the sale", got)
	}

	e.setInvoicing(t, accountID, false)
	if page := e.awaitingInvoice(t, token, "", ""); len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("invoicing off: %+v, want nothing", page)
	}
}

// The view reads only the caller's account, narrows to the active branch when one is chosen, and a
// seller sees the sales they own or nobody claimed, like every other list of orders.
func TestSalesAwaitingInvoice_StaysInsideTheTenantBranchAndSellerReach(t *testing.T) {
	e := newEnv(t)
	accountID, centro := e.seedAccount(t, "Corralón Alcance")
	norte := e.seedBranch(t, accountID, "Corralón Alcance Norte")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	e.assignBranch(t, accountID, seller, centro)
	e.setInvoicing(t, accountID, true)

	inCentro := e.seedAcceptedSale(t, accountID, centro, admin)
	unclaimed := e.seedAcceptedSale(t, accountID, centro, admin)
	e.exec(t, `UPDATE quote SET seller_id = NULL WHERE id = $1`, unclaimed.quoteID)
	sellers := e.seedAcceptedSale(t, accountID, centro, seller)
	inNorte := e.seedAcceptedSale(t, accountID, norte, admin)

	otherID, otherBranch := e.seedAccount(t, "Otro Corralón")
	otherAdmin := e.seedUser(t, otherID, domain.UserRoleAdmin)
	e.setInvoicing(t, otherID, true)
	foreign := e.seedAcceptedSale(t, otherID, otherBranch, otherAdmin)

	adminToken := e.tokenFor(t, admin)
	everywhere := e.awaitingInvoice(t, adminToken, "", "").quoteIDs()
	slices.SortFunc(everywhere, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	want := []uuid.UUID{inCentro.quoteID, unclaimed.quoteID, sellers.quoteID, inNorte.quoteID}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if !slices.Equal(everywhere, want) {
		t.Fatalf("admin, every branch: %v, want %v and never the other account's %s", everywhere, want, foreign.quoteID)
	}
	if got := e.awaitingInvoice(t, adminToken, norte.String(), "").quoteIDs(); !slices.Equal(got, []uuid.UUID{inNorte.quoteID}) {
		t.Fatalf("admin in Norte: %v, want only %s", got, inNorte.quoteID)
	}
	got := e.awaitingInvoice(t, e.tokenFor(t, seller), centro.String(), "").quoteIDs()
	if len(got) != 2 || !slices.Contains(got, unclaimed.quoteID) || !slices.Contains(got, sellers.quoteID) {
		t.Fatalf("seller in Centro: %v, want their own %s and the unclaimed %s", got, sellers.quoteID, unclaimed.quoteID)
	}
}

// The view pages like the invoice history: a full page, then the rest. A page that is not a number
// is malformed, and one out of range is refused.
func TestSalesAwaitingInvoice_Pages(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Páginas")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	var last uuid.UUID
	for i := range domain.InvoicePageSize + 1 {
		sale := e.seedAcceptedSale(t, accountID, branchID, admin)
		e.acceptOn(t, accountID, sale.quoteID, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i))
		last = sale.quoteID
	}
	token := e.tokenFor(t, admin)

	first := e.awaitingInvoice(t, token, "", "")
	if len(first.Items) != domain.InvoicePageSize || first.Total != domain.InvoicePageSize+1 ||
		first.Page != 1 || first.PageSize != domain.InvoicePageSize {
		t.Fatalf("page 1: %d items, total %d, page %d, size %d", len(first.Items), first.Total, first.Page, first.PageSize)
	}
	second := e.awaitingInvoice(t, token, "", "?page=2")
	if !slices.Equal(second.quoteIDs(), []uuid.UUID{last}) || second.Total != domain.InvoicePageSize+1 {
		t.Fatalf("page 2: %v (total %d), want only the newest %s", second.quoteIDs(), second.Total, last)
	}
	for bad, want := range map[string]int{
		"x": http.StatusBadRequest, "0": http.StatusUnprocessableEntity,
		"-1": http.StatusUnprocessableEntity, strconv.Itoa(100001): http.StatusUnprocessableEntity,
	} {
		rec := e.do(t, request{method: http.MethodGet, path: awaitingInvoicePath + "?page=" + bad, token: token})
		if rec.Code != want {
			t.Fatalf("page %s: status = %d, want %d", bad, rec.Code, want)
		}
	}
}

// A sale's client is the quote's, set when it was sent, so the view names the same client the
// client profile files the sale under even when the order's own client differs.
func TestSalesAwaitingInvoice_NamesTheQuotesClient(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Cliente De La Venta")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	quoteClient, rfqClient := uuid.New(), uuid.New()
	e.exec(t, `INSERT INTO client (id, account_id, name) VALUES ($1, $3, 'Obra Norte'), ($2, $3, 'Obra Vieja')`,
		quoteClient, rfqClient, accountID)
	sale := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.exec(t, `UPDATE quote SET client_id = $2 WHERE id = $1`, sale.quoteID, quoteClient)
	e.exec(t, `UPDATE rfq SET client_id = $2 WHERE id = $1`, sale.rfqID, rfqClient)

	page := e.awaitingInvoice(t, e.tokenFor(t, admin), branchID.String(), "")
	if len(page.Items) != 1 {
		t.Fatalf("awaiting = %v, want only %s", page.quoteIDs(), sale.quoteID)
	}
	if got := page.Items[0]; got.Client != "Obra Norte" || got.ClientID == nil || *got.ClientID != quoteClient {
		t.Fatalf("row client = %q (%v), want the quote's Obra Norte (%s)", got.Client, got.ClientID, quoteClient)
	}
}

// The client profile says where each accepted sale stands with its invoice.
func TestClientProfile_ReportsEachSalesInvoiceStatus(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Ficha")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	e.setInvoicing(t, accountID, true)
	clientID := uuid.New()
	e.exec(t, `INSERT INTO client (id, account_id, name) VALUES ($1, $2, 'Obra Norte')`, clientID, accountID)

	uninvoiced := e.seedAcceptedSale(t, accountID, branchID, admin)
	waived := e.seedAcceptedSale(t, accountID, branchID, admin)
	e.exec(t, `UPDATE quote SET invoice_waived_at = now(), invoice_waived_by_user_id = $2 WHERE id = $1`,
		waived.quoteID, admin.ID)
	invoiced := e.seedIssuedSale(t, accountID, branchID, admin, 41)
	pending := e.seedInvoicedSale(t, accountID, branchID, admin, domain.InvoiceStatusPending)
	credited := e.seedIssuedSale(t, accountID, branchID, admin, 42)
	e.seedCreditNote(t, credited, domain.InvoiceStatusIssued)
	want := map[uuid.UUID]string{
		uninvoiced.quoteID: "UNINVOICED", waived.quoteID: "WAIVED", invoiced.quoteID: "INVOICED",
		pending.quoteID: "PENDING", credited.quoteID: "UNINVOICED",
	}
	ids := make([]uuid.UUID, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	e.exec(t, `UPDATE quote SET client_id = $2 WHERE id = ANY($1)`, ids, clientID)

	rec := e.do(t, request{method: http.MethodGet, path: "/v1/clients/" + clientID.String(), token: e.tokenFor(t, admin)})
	var profile struct {
		Sales []struct {
			QuoteID       uuid.UUID `json:"quote_id"`
			InvoiceStatus string    `json:"invoice_status"`
		} `json:"sales"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &profile) != nil {
		t.Fatalf("profile: status = %d, body = %.300s", rec.Code, rec.Body)
	}
	if len(profile.Sales) != len(want) {
		t.Fatalf("profile lists %d sales, want %d", len(profile.Sales), len(want))
	}
	for _, sale := range profile.Sales {
		if sale.InvoiceStatus != want[sale.QuoteID] {
			t.Errorf("sale %s: invoice_status = %q, want %q", sale.QuoteID, sale.InvoiceStatus, want[sale.QuoteID])
		}
	}
}
