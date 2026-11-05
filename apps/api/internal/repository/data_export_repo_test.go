//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func exportFilter(accountID uuid.UUID) domain.DataExportFilter {
	return domain.DataExportFilter{AccountID: accountID}
}

func seedExportBranch(t *testing.T, db *DB, accountID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	reportExec(t, db, "branch", `INSERT INTO branch (id, account_id, name) VALUES ($1, $2, $3)`,
		id, accountID, name)
	return id
}

func TestDataExportRepositoryListsQuotesWithTheirCurrentLines(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Exportación")
	otherAccount := seedAccount(t, db, "Exportación ajena")
	branchID := branchOf(t, db, accountID)
	northID := seedExportBranch(t, db, accountID, "Norte")
	clientID := seedClient(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento")
	reportExec(t, db, "code", `UPDATE product SET code = 'CEM-1' WHERE id = $1`, productID)
	inPeriod, versionID, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	before, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)
	north, _, _ := seedQuoteChain(t, db, accountID, northID, productID)
	foreignProduct := seedProduct(t, db, otherAccount, "Cemento ajeno")
	seedQuoteChain(t, db, otherAccount, branchOf(t, db, otherAccount), foreignProduct)

	reportExec(t, db, "quote", `UPDATE quote SET client_id = $2, current_status = 'ACCEPTED',
	  created_at = '2026-11-01T03:00:00Z' WHERE id = $1`, inPeriod, clientID)
	// 23:59 on October 31 in Argentina, so outside a period that starts on November 1.
	reportExec(t, db, "early quote", `UPDATE quote SET created_at = '2026-11-01T02:59:00Z' WHERE id = $1`, before)
	reportExec(t, db, "north quote", `UPDATE quote SET created_at = '2026-11-01T15:00:00Z' WHERE id = $1`, north)
	reportExec(t, db, "total", `UPDATE quote_version SET total = 900 WHERE id = $1`, versionID)
	reportExec(t, db, "line", `UPDATE quote_item SET unit_price_snapshot = 100, subtotal = 1000
	  WHERE version_id = $1`, versionID)
	reportExec(t, db, "unmatched line", `INSERT INTO quote_item
	  (account_id, version_id, requested_description, quantity, match_status, created_at)
	  VALUES ($1, $2, 'ladrillo raro', 3, 'NO_MATCH', now() + interval '1 second')`, accountID, versionID)

	repo := NewDataExportRepository()
	var all, narrowed []domain.DataExportQuote
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var err error
		if all, err = repo.ListQuotes(ctx, q, exportFilter(accountID)); err != nil {
			return err
		}
		filter := exportFilter(accountID)
		start := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
		end := time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)
		filter.BranchID, filter.StartAt, filter.EndAt = &branchID, &start, &end
		narrowed, err = repo.ListQuotes(ctx, q, filter)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if len(all) != 3 {
		t.Fatalf("quotes = %d, want the account's three and not the other account's", len(all))
	}
	if len(narrowed) != 1 || narrowed[0].ID != inPeriod {
		t.Fatalf("narrowed = %+v, want only the in-period quote at the head office", narrowed)
	}
	quote := narrowed[0]
	if quote.Status != domain.QuoteStatusAccepted || quote.ClientID == nil || *quote.ClientID != clientID ||
		quote.Client == nil || *quote.Total != "900.00" || *quote.ItemsSubtotal != "1000.00" {
		t.Fatalf("quote = %+v", quote)
	}
	if len(quote.Items) != 2 || *quote.Items[0].ProductCode != "CEM-1" || *quote.Items[0].Subtotal != "1000.00" ||
		quote.Items[1].MatchStatus != domain.ItemMatchStatusNoMatch || quote.Items[1].ProductCode != nil {
		t.Fatalf("items = %+v, want the matched line and the flagged one in order", quote.Items)
	}
}

func TestDataExportRepositoryListsTheCatalogPerBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Exportación catálogo")
	branchID := branchOf(t, db, accountID)
	northID := seedExportBranch(t, db, accountID, "Norte")
	carried := seedProduct(t, db, accountID, "Arena")
	priceCleanup(t, db, carried)
	uncarried := seedProduct(t, db, accountID, "Cal")
	reportExec(t, db, "codes", `UPDATE product SET code = CASE id WHEN $1 THEN 'A-1' ELSE 'C-1' END
	  WHERE id IN ($1, $2)`, carried, uncarried)
	reportExec(t, db, "availability", `INSERT INTO branch_product (account_id, branch_id, product_id, stock, is_active)
	  VALUES ($1, $2, $4, 5, TRUE), ($1, $3, $4, NULL, FALSE)`, accountID, branchID, northID, carried)
	reportExec(t, db, "prices", `INSERT INTO product_price
	  (account_id, branch_id, product_id, price, min_price, valid_from, valid_to)
	  VALUES ($1, $2, $3, 80, NULL, now() - interval '2 days', now() - interval '1 day'),
	         ($1, $2, $3, 100, 90, now() - interval '1 day', NULL)`, accountID, branchID, carried)

	repo := NewDataExportRepository()
	var all, narrowed []domain.DataExportProduct
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var err error
		if all, err = repo.ListCatalog(ctx, q, exportFilter(accountID)); err != nil {
			return err
		}
		filter := exportFilter(accountID)
		filter.BranchID = &northID
		narrowed, err = repo.ListCatalog(ctx, q, filter)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if len(all) != 2 || all[0].ID != carried || all[1].ID != uncarried || len(all[1].Branches) != 0 {
		t.Fatalf("catalog = %+v, want both products, the uncarried one without branches", all)
	}
	branches := all[0].Branches
	if len(branches) != 2 || branches[0].Branch != "Exportación catálogo Centro" ||
		*branches[0].Price != "100.00" || *branches[0].MinPrice != "90.00" || *branches[0].Stock != "5.00" ||
		branches[1].Branch != "Norte" || branches[1].IsActive || branches[1].Price != nil {
		t.Fatalf("branches = %+v, want the current head-office price and the unpriced north", branches)
	}
	if len(narrowed) != 2 || len(narrowed[0].Branches) != 1 || narrowed[0].Branches[0].Branch != "Norte" {
		t.Fatalf("narrowed = %+v, want every product with only the north branch", narrowed)
	}
}

func TestDataExportRepositoryListsLiveClientsAndContacts(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Exportación clientes")
	otherAccount := seedAccount(t, db, "Exportación clientes ajena")
	kept := seedClient(t, db, accountID)
	merged := seedClient(t, db, accountID)
	seedClient(t, db, otherAccount)
	tagID := uuid.New()
	reportExec(t, db, "tag", `INSERT INTO tag (id, account_id, name) VALUES ($1, $2, 'Mayorista')`, tagID, accountID)
	reportExec(t, db, "client tag", `INSERT INTO client_tag (account_id, client_id, tag_id) VALUES ($1, $2, $3)`,
		accountID, kept, tagID)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM client_tag WHERE tag_id = $1`, tagID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM tag WHERE id = $1`, tagID)
	})
	reportExec(t, db, "merge", `UPDATE client SET merged_into = $2 WHERE id = $1`, merged, kept)
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `UPDATE client SET merged_into = NULL WHERE id = $1`, merged)
	})
	reportExec(t, db, "contacts", `INSERT INTO client_contact
	  (account_id, client_id, kind, value, is_main, source, verified_at, retired_at)
	  VALUES ($1, $2, 'WHATSAPP', '+5491100000001', TRUE, 'SELLER', now(), NULL),
	         ($1, $2, 'EMAIL', 'obra@example.com', FALSE, 'SELLER', NULL, NULL),
	         ($1, $2, 'EMAIL', 'vieja@example.com', FALSE, 'SELLER', now(), now())`, accountID, kept)

	var clients []domain.DataExportClient
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var err error
		clients, err = NewDataExportRepository().ListClients(ctx, q, exportFilter(accountID))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if len(clients) != 1 || clients[0].ID != kept {
		t.Fatalf("clients = %+v, want only the live client of this account", clients)
	}
	client := clients[0]
	if len(client.Tags) != 1 || client.Tags[0] != "Mayorista" {
		t.Fatalf("tags = %v", client.Tags)
	}
	if len(client.Contacts) != 2 || !client.Contacts[0].IsMain || !client.Contacts[0].Verified ||
		client.Contacts[1].Value != "obra@example.com" || client.Contacts[1].Verified {
		t.Fatalf("contacts = %+v, want the main one first and the retired one left out", client.Contacts)
	}
}

func TestDataExportRepositoryListsInvoicesInThePeriod(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoicing := NewInvoicingRepository()
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		pending, err := invoicing.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		_, err = invoicing.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, domain.InvoiceAuthorization{
			Number: 23, CAE: "74123456789012", CAEExpiresOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
			RawRequest: []byte("<req/>"), RawResponse: []byte("<res/>"),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	repo := NewDataExportRepository()
	read := func(from, to string) []domain.DataExportInvoice {
		t.Helper()
		var invoices []domain.DataExportInvoice
		filter := exportFilter(tenant.AccountID)
		filter.FromDate, filter.ToDate = &from, &to
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			invoices, err = repo.ListInvoices(ctx, q, filter)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return invoices
	}

	invoices := read("2026-10-05", "2026-10-05")
	if len(invoices) != 1 {
		t.Fatalf("invoices = %+v, want the one issued that day", invoices)
	}
	invoice := invoices[0]
	if invoice.Status != domain.InvoiceStatusIssued || *invoice.Number != 23 || *invoice.CAE != "74123456789012" ||
		invoice.IssuedOn != "2026-10-05" || *invoice.CAEExpiresOn != "2026-10-15" || invoice.Total != "1210.00" ||
		invoice.ReceiverName != "Obra Norte" || invoice.Branch != "Invoicing Centro" {
		t.Fatalf("invoice = %+v", invoice)
	}
	if got := read("2026-10-06", "2026-10-31"); len(got) != 0 {
		t.Fatalf("invoices after the issue date = %+v, want none", got)
	}
	if got := read("2026-10-01", "2026-10-04"); len(got) != 0 {
		t.Fatalf("invoices before the issue date = %+v, want none", got)
	}
}
