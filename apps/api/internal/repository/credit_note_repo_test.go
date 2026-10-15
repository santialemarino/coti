//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// creditNoteAuth is ARCA's authorization under number.
func creditNoteAuth(number int64) domain.InvoiceAuthorization {
	return domain.InvoiceAuthorization{Number: number, CAE: "74123456789012",
		CAEExpiresOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
}

// issueVoucher creates in and records ARCA's authorization on it, numbered 23.
func issueVoucher(t *testing.T, db *DB, tenant domain.Tenant, in domain.Invoice) *domain.Invoice {
	t.Helper()
	ctx := context.Background()
	repo := NewInvoicingRepository()
	var issued *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		pending, err := repo.CreatePending(ctx, q, in)
		if err != nil {
			return err
		}
		issued, err = repo.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, creditNoteAuth(23))
		return err
	}); err != nil {
		t.Fatalf("issue %s: %v", in.Kind, err)
	}
	return issued
}

func creditNoteOf(invoice domain.Invoice) domain.Invoice {
	note := invoice
	note.ID, note.Number, note.CAE, note.CAEExpiresOn = uuid.Nil, nil, nil, nil
	note.Kind, note.CreditedInvoiceID, note.ReplacesInvoiceID = domain.VoucherKindCreditNote, &invoice.ID, nil
	return note
}

func createPending(db *DB, tenant domain.Tenant, in domain.Invoice) (*domain.Invoice, error) {
	ctx := context.Background()
	var pending *domain.Invoice
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		pending, err = NewInvoicingRepository().CreatePending(ctx, q, in)
		return err
	})
	return pending, err
}

func hasLiveInvoice(t *testing.T, db *DB, tenant domain.Tenant, quoteID uuid.UUID, env domain.ARCAEnvironment) bool {
	t.Helper()
	ctx := context.Background()
	var live bool
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		live, err = NewQuoteRepository().HasLiveInvoice(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, env)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return live
}

// An issued credit note frees the sale: the invoice stops being live, a second credit note is
// refused, and a new invoice goes in only by naming the credited one.
func TestInvoicingRepository_AnIssuedCreditNoteFreesTheSale(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	repo := NewInvoicingRepository()
	env := domain.ARCAEnvironmentHomologation
	invoice := issueVoucher(t, db, tenant, pendingInvoice(tenant, quoteID, versionID))

	pending, err := createPending(db, tenant, creditNoteOf(*invoice))
	if err != nil {
		t.Fatalf("credit note: %v", err)
	}
	if !hasLiveInvoice(t, db, tenant, quoteID, env) {
		t.Fatal("a sale whose credit note is still pending reads as free")
	}
	// Until ARCA authorizes the credit note, the invoice is still the sale's, not history.
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		latest, err := repo.GetLatestByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, env)
		if err != nil {
			return err
		}
		credited, err := repo.GetLatestCreditedByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, env)
		if err != nil {
			return err
		}
		if latest == nil || latest.ID != invoice.ID || credited != nil {
			t.Errorf("pending credit note: latest = %+v, credited = %+v, want the invoice still live", latest, credited)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := createPending(db, tenant, creditNoteOf(*invoice)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second credit note while one is pending = %v, want ErrConflict", err)
	}
	again := pendingInvoice(tenant, quoteID, versionID)
	again.ReplacesInvoiceID = &invoice.ID
	if _, err := createPending(db, tenant, again); err == nil {
		t.Fatal("an invoice replaced one whose credit note is not issued")
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, creditNoteAuth(23))
		return err
	}); err != nil {
		t.Fatalf("issue the credit note (same number as the invoice, its own series): %v", err)
	}
	if hasLiveInvoice(t, db, tenant, quoteID, env) {
		t.Fatal("a sale whose invoice an issued credit note cancelled still reads as invoiced")
	}
	var latest, credited *domain.Invoice
	var notes map[uuid.UUID]domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		if latest, err = repo.GetLatestByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, env); err != nil {
			return err
		}
		if credited, err = repo.GetLatestCreditedByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID, env); err != nil {
			return err
		}
		notes, err = repo.GetCreditNotesByInvoiceIDs(ctx, q, tenant.AccountID, []uuid.UUID{invoice.ID})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if latest != nil || credited == nil || credited.ID != invoice.ID || notes[invoice.ID].ID != pending.ID ||
		notes[invoice.ID].Status != domain.InvoiceStatusIssued {
		t.Fatalf("latest = %+v, credited = %+v, notes = %+v, want the invoice credited and nothing live", latest, credited, notes)
	}
	if _, err := createPending(db, tenant, creditNoteOf(*invoice)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("credit note for a credited invoice = %v, want ErrConflict", err)
	}

	if _, err := createPending(db, tenant, pendingInvoice(tenant, quoteID, versionID)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("new invoice not naming the credited one = %v, want ErrConflict", err)
	}
	reinvoiced, err := createPending(db, tenant, again)
	if err != nil {
		t.Fatalf("new invoice after the credit note = %v", err)
	}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := repo.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, reinvoiced.ID, creditNoteAuth(24))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !hasLiveInvoice(t, db, tenant, quoteID, env) {
		t.Fatal("the sale invoiced again reads as free")
	}
	if _, err := createPending(db, tenant, again); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("a second invoice after the same credit note = %v, want ErrConflict", err)
	}
}

// A credit note is a fiscal record like the invoice it cancels, and it cancels only an issued invoice.
func TestInvoicingRepository_CreditNoteIntegrity(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoice := issueVoucher(t, db, tenant, pendingInvoice(tenant, quoteID, versionID))
	note := issueVoucher(t, db, tenant, creditNoteOf(*invoice))

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := q.Exec(ctx, `UPDATE invoice SET issues = '["edited"]' WHERE account_id = $1 AND id = $2`, tenant.AccountID, note.ID)
		return err
	})
	if err == nil {
		t.Fatal("an issued credit note was edited")
	}
	if _, err := createPending(db, tenant, creditNoteOf(*note)); err == nil {
		t.Fatal("a credit note cancelled a credit note")
	}

	other := pendingInvoice(tenant, quoteID, versionID)
	other.Environment = domain.ARCAEnvironmentProduction
	pending, err := createPending(db, tenant, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createPending(db, tenant, creditNoteOf(*pending)); err == nil {
		t.Fatal("a credit note cancelled a pending invoice")
	}
}

// A real invoice holds the sale on a test server until a real credit note cancels it.
func TestQuoteRepository_ARealInvoiceIsFreedOnlyByARealCreditNote(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	in := pendingInvoice(tenant, quoteID, versionID)
	in.Environment = domain.ARCAEnvironmentProduction
	real := issueVoucher(t, db, tenant, in)

	if !hasLiveInvoice(t, db, tenant, quoteID, domain.ARCAEnvironmentHomologation) {
		t.Fatal("a real invoice does not hold the sale on a test server")
	}
	issueVoucher(t, db, tenant, creditNoteOf(*real))
	for _, env := range []domain.ARCAEnvironment{domain.ARCAEnvironmentHomologation, domain.ARCAEnvironmentProduction} {
		if hasLiveInvoice(t, db, tenant, quoteID, env) {
			t.Fatalf("%s: a real invoice cancelled by a real credit note still holds the sale", env)
		}
	}
}
