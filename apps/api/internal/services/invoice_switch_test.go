package services

import (
	"context"
	"errors"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func assertInvoicingOff(t *testing.T, err error, f invoiceFixture, requests, created int) {
	t.Helper()
	if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != domain.CodeAccountInvoicingOff {
		t.Fatalf("err = %v, want ACCOUNT_INVOICING_OFF", err)
	}
	if len(f.issuer.requests) != requests || f.store.created != created {
		t.Fatalf("ARCA asked %d times and %d records held, want %d and %d", len(f.issuer.requests), f.store.created,
			requests, created)
	}
}

// With the account's switch off nothing new is issued: not an invoice, not its preview, not a credit note.
func TestInvoiceService_SwitchedOffRefusesIssuing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("preview", func(t *testing.T) {
		t.Parallel()
		f := newInvoiceFixture(t)
		f.store.invoicingOff = true
		preview, err := f.service.Preview(ctx, f.tenant, f.quoteID)
		if preview != nil {
			t.Fatalf("preview = %+v, want none", preview)
		}
		assertInvoicingOff(t, err, f, 0, 0)
	})
	t.Run("invoice", func(t *testing.T) {
		t.Parallel()
		f := newInvoiceFixture(t)
		f.store.invoicingOff = true
		_, err := f.issue(ctx)
		assertInvoicingOff(t, err, f, 0, 0)
	})
	t.Run("credit note", func(t *testing.T) {
		t.Parallel()
		f := newInvoiceFixture(t)
		invoice := issuedInvoice(t, f)
		f.store.invoicingOff = true
		_, err := f.service.IssueCreditNote(ctx, f.tenant, invoice.ID)
		assertInvoicingOff(t, err, f, 1, 1)
	})
}

// Switched off after the first check, the hold still refuses: it reads the switch under the lock that
// switching off waits for.
func TestInvoiceService_SwitchedOffBeforeTheHoldRefusesIssuing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("invoice", func(t *testing.T) {
		t.Parallel()
		f := newInvoiceFixture(t)
		f.store.offOnceLocked = true
		_, err := f.issue(ctx)
		assertInvoicingOff(t, err, f, 0, 0)
	})
	t.Run("credit note", func(t *testing.T) {
		t.Parallel()
		f := newInvoiceFixture(t)
		invoice := issuedInvoice(t, f)
		f.store.offOnceLocked = true
		_, err := f.service.IssueCreditNote(ctx, f.tenant, invoice.ID)
		assertInvoicingOff(t, err, f, 1, 1)
	})
}
