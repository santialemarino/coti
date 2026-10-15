package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// issuedInvoice issues the fixture's invoice and returns a copy of it as stored.
func issuedInvoice(t *testing.T, f invoiceFixture) domain.Invoice {
	t.Helper()
	invoice, err := f.issue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.store.accountLocked = false
	return *invoice
}

func TestInvoiceService_CreditNoteCancelsTheInvoiceInFull(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)

	note, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if note.Kind != domain.VoucherKindCreditNote || note.Status != domain.InvoiceStatusIssued ||
		note.CreditedInvoiceID == nil || *note.CreditedInvoiceID != invoice.ID || note.CreditedInvoice.ID != invoice.ID {
		t.Fatalf("note = %+v, want an issued credit note naming the invoice", note)
	}
	req := f.issuer.requests[1]
	want := domain.AssociatedVoucher{Type: domain.VoucherType{Kind: domain.VoucherKindInvoice, Letter: domain.InvoiceTypeA},
		PointOfSale: 3, Number: 23, IssuerCUIT: "30712345671", Date: invoice.IssuedOn}
	if req.VoucherType().ARCACode() != 3 || req.Associated == nil || *req.Associated != want ||
		req.PointOfSale != 3 || req.IssuerCUIT != invoice.IssuerCUIT || req.Receiver != invoice.Receiver {
		t.Fatalf("request = %+v, want a credit note A associated with invoice A 0003-23", req)
	}
	assertMoney(t, "credit note total", req.Amounts.Total, "1210.00")
	assertMoney(t, "credit note VAT", req.Amounts.VAT, invoice.Amounts.VAT.StringFixed(2))
	if !f.store.accountLocked || f.store.heldUnlocked {
		t.Fatal("the credit note was held without the account lock")
	}
}

// The service refuses a second credit note on its own, before the index is ever reached.
func TestInvoiceService_OneCreditNotePerInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)
	if _, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID); err != nil {
		t.Fatal(err)
	}
	f.store.noUniqueIndex = true

	_, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)

	if domain.CodeOf(err) != domain.CodeInvoiceAlreadyCredited || len(f.issuer.requests) != 2 {
		t.Fatalf("err = %v after %d requests, want INVOICE_ALREADY_CREDITED without asking ARCA", err, len(f.issuer.requests))
	}
}

func TestInvoiceService_APendingCreditNoteHoldsTheInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)
	f.issuer.err = domain.ErrInvoiceOutcomeUnknown
	if _, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID); domain.CodeOf(err) != domain.CodeCreditNoteInProgress {
		t.Fatalf("err = %v, want the unknown outcome kept pending", err)
	}
	f.store.invoice.UpdatedAt = invoiceNow
	f.store.noUniqueIndex = true

	_, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)

	if domain.CodeOf(err) != domain.CodeCreditNoteInProgress || len(f.issuer.requests) != 2 {
		t.Fatalf("err = %v after %d requests, want CREDIT_NOTE_IN_PROGRESS without asking ARCA", err, len(f.issuer.requests))
	}
}

// Only an issued invoice is credited: never one in flight, a refused one, or a credit note.
func TestInvoiceService_CreditNoteRefusesWhatIsNotAnIssuedInvoice(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)
	note, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending := domain.Invoice{ID: uuid.New(), BranchID: f.tenant.BranchID, Kind: domain.VoucherKindInvoice,
		Status: domain.InvoiceStatusPending, Environment: domain.ARCAEnvironmentHomologation}
	f.store.earlier = append(f.store.earlier, pending)

	for name, id := range map[string]uuid.UUID{"credit note": note.ID, "pending invoice": pending.ID} {
		_, err := f.service.IssueCreditNote(context.Background(), f.tenant, id)
		if domain.CodeOf(err) != domain.CodeInvoiceNotCreditable {
			t.Fatalf("%s: err = %v, want INVOICE_NOT_CREDITABLE", name, err)
		}
	}
	if _, err := f.service.IssueCreditNote(context.Background(), f.tenant, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown invoice: err = %v, want ErrNotFound", err)
	}
	if len(f.issuer.requests) != 2 {
		t.Fatalf("%d requests, want none past the two issued", len(f.issuer.requests))
	}
}

// ARCA refuses a credit note from a connection that cannot reach the invoice's issuer; so does Coti, first.
func TestInvoiceService_CreditNoteNeedsTheInvoicesConnection(t *testing.T) {
	t.Parallel()
	cases := map[string]func(f invoiceFixture){
		"another CUIT":  func(f invoiceFixture) { f.store.credential.CUIT = "20329642330" },
		"no connection": func(f invoiceFixture) { f.store.credential = nil },
		"disabled":      func(f invoiceFixture) { f.service.settings.Enabled = false },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvoiceFixture(t)
			invoice := issuedInvoice(t, f)
			breakIt(f)

			_, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)

			var notReady *domain.InvoiceNotReadyError
			if !errors.As(err, &notReady) || len(f.issuer.requests) != 1 {
				t.Fatalf("err = %v after %d requests, want INVOICE_NOT_READY before ARCA", err, len(f.issuer.requests))
			}
		})
	}
}

// Once credited, the sale is no longer invoiced: the preview shows the credited invoice as history and
// a new invoice goes out naming it.
func TestInvoiceService_ACreditedSaleIsInvoicedAgain(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)
	if _, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID); err != nil {
		t.Fatal(err)
	}

	preview, err := f.service.Preview(context.Background(), f.tenant, f.quoteID)
	if err != nil || preview.Invoice != nil || preview.CreditedInvoice == nil || preview.CreditedInvoice.ID != invoice.ID ||
		!preview.CreditedInvoice.Credited() {
		t.Fatalf("preview = %+v, err = %v, want no live invoice and the credited one as history", preview, err)
	}
	again, err := f.issue(context.Background())
	if err != nil || again.Status != domain.InvoiceStatusIssued || again.ReplacesInvoiceID == nil ||
		*again.ReplacesInvoiceID != invoice.ID {
		t.Fatalf("invoice = %+v, err = %v, want a new invoice following the credited one", again, err)
	}
}

// A pending credit note is read back in its own environment and series, whatever the server is set to.
func TestInvoiceService_PendingCreditNoteRecoveryAsksItsOwnEnvironment(t *testing.T) {
	t.Parallel()
	for name, recover := range map[string]func(f invoiceFixture, invoice, note domain.Invoice) error{
		"recover": func(f invoiceFixture, _, note domain.Invoice) error {
			_, err := f.service.Recover(context.Background(), f.tenant, note.ID)
			return err
		},
		"credit again": func(f invoiceFixture, invoice, _ domain.Invoice) error {
			_, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newInvoiceFixture(t)
			invoice := issuedInvoice(t, f)
			f.issuer.err = domain.ErrInvoiceOutcomeUnknown
			_, _ = f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)
			note := *f.store.invoice
			f.store.invoice.UpdatedAt = invoiceNow.Add(-time.Hour)
			f.issuer.err = nil
			f.service.settings.Environment = domain.ARCAEnvironmentProduction

			if err := recover(f, invoice, note); err != nil {
				t.Fatal(err)
			}
			if len(f.issuer.readWith) != 1 || f.issuer.readWith[0] != domain.ARCAEnvironmentHomologation ||
				f.issuer.consultedTypes[0] != (domain.VoucherType{Kind: domain.VoucherKindCreditNote, Letter: domain.InvoiceTypeA}) ||
				f.issuer.consulted[0] != 42 {
				t.Fatalf("read back %v in %v with %v, want credit note A 42 in homologation",
					f.issuer.consultedTypes, f.issuer.consulted, f.issuer.readWith)
			}
			if !slices.Contains(f.store.released, releasedIssue) {
				t.Fatalf("released %v, want the hold ARCA never authorized released", f.store.released)
			}
		})
	}
}

// A credit note never predates the invoice it cancels.
func TestCreditNoteFor_NeverPredatesTheInvoice(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct{ invoiced, want time.Time }{
		"earlier invoice": {day.AddDate(0, 0, -3), day},
		"later invoice":   {day.AddDate(0, 0, 1), day.AddDate(0, 0, 1)},
	} {
		note := creditNoteFor(domain.Invoice{ID: uuid.New(), IssuedOn: tc.invoiced}, day)
		if !note.IssuedOn.Equal(tc.want) {
			t.Errorf("%s: dated %s, want %s", name, note.IssuedOn, tc.want)
		}
	}
}

// Auto-archive can put an invoiced sale away; cancelling its invoice is still a fiscal correction to make.
func TestInvoiceService_CreditNoteOnAnArchivedQuote(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	invoice := issuedInvoice(t, f)
	archived := invoiceNow.Add(-time.Hour)
	f.quotes.quote.ArchivedAt = &archived

	note, err := f.service.IssueCreditNote(context.Background(), f.tenant, invoice.ID)

	if err != nil || note.Status != domain.InvoiceStatusIssued || len(f.issuer.requests) != 2 {
		t.Fatalf("note = %+v, err = %v, want the credit note issued on the archived sale", note, err)
	}
}
