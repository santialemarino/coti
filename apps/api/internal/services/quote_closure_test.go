package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeInvoicingSwitch struct{ enabled bool }

func (f fakeInvoicingSwitch) InvoicingEnabled(context.Context, repository.Querier, uuid.UUID) (bool, error) {
	return f.enabled, nil
}

// closureFixture is a quote in status with invoicing switched on.
func closureFixture(status domain.QuoteStatus, archived bool) quoteFixture {
	quote := &domain.Quote{ID: uuid.New(), CurrentStatus: status}
	if archived {
		now := time.Now()
		quote.ArchivedAt = &now
	}
	f := quoteFixtureAt(quote)
	f.service.WithInvoicingSwitch(fakeInvoicingSwitch{enabled: true})
	return f
}

// Paid and «No requiere factura» are written only on an accepted sale still in the list; anything
// else is refused before a write, with the code that tells the two refusals apart.
func TestQuoteService_ClosingWritesNeedAnOpenAcceptedSale(t *testing.T) {
	t.Parallel()
	writes := map[string]func(*QuoteService, context.Context, domain.Tenant, uuid.UUID) (*domain.Quote, error){
		"MarkPaid":           (*QuoteService).MarkPaid,
		"UnmarkPaid":         (*QuoteService).UnmarkPaid,
		"WaiveInvoice":       (*QuoteService).WaiveInvoice,
		"RestoreInvoiceWait": (*QuoteService).RestoreInvoiceWait,
	}
	cases := []struct {
		name     string
		status   domain.QuoteStatus
		archived bool
		wantCode domain.ErrorCode
	}{
		{"sent", domain.QuoteStatusSent, false, domain.CodeQuoteNotAccepted},
		{"rejected", domain.QuoteStatusRejected, false, domain.CodeQuoteNotAccepted},
		{"quoted", domain.QuoteStatusQuoted, false, domain.CodeQuoteNotAccepted},
		{"archived accepted", domain.QuoteStatusAccepted, true, domain.CodeQuoteArchived},
	}
	for name, write := range writes {
		for _, tc := range cases {
			f := closureFixture(tc.status, tc.archived)
			_, err := write(f.service, context.Background(), f.tenant, f.quoteID)
			if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != tc.wantCode {
				t.Errorf("%s on %s: err = %v, want 409 %s", name, tc.name, err, tc.wantCode)
			}
			if len(f.quotes.paidWrites)+len(f.quotes.waiverWrites)+len(f.quotes.statusUpdates) != 0 {
				t.Errorf("%s on %s wrote despite the refusal", name, tc.name)
			}
		}
	}
}

// Marking paid stores who did it, and unmarking clears both; the status and its history stay put.
func TestQuoteService_MarkPaidRecordsWhoAndUnmarkClears(t *testing.T) {
	t.Parallel()
	f := closureFixture(domain.QuoteStatusAccepted, false)

	paid, err := f.service.MarkPaid(context.Background(), f.tenant, f.quoteID)
	if err != nil || paid.PaidAt == nil || paid.PaidByUserID == nil || *paid.PaidByUserID != f.tenant.UserID {
		t.Fatalf("MarkPaid() = %+v, %v, want paid by %s", paid, err, f.tenant.UserID)
	}
	if paid.CurrentStatus != domain.QuoteStatusAccepted || len(f.quotes.statusUpdates)+len(f.quotes.statusChanges) != 0 {
		t.Fatalf("marking paid touched the status: %s, %d updates", paid.CurrentStatus, len(f.quotes.statusUpdates))
	}
	unpaid, err := f.service.UnmarkPaid(context.Background(), f.tenant, f.quoteID)
	if err != nil || unpaid.PaidAt != nil || unpaid.PaidByUserID != nil {
		t.Fatalf("UnmarkPaid() = %+v, %v, want the mark gone", unpaid, err)
	}
	if len(f.quotes.paidWrites) != 2 || f.quotes.paidWrites[1] != nil {
		t.Fatalf("paid writes = %v, want a mark then a clear", f.quotes.paidWrites)
	}
}

// «No requiere factura» is refused while invoicing is off or the sale has a live invoice, and is
// recorded with its author otherwise.
func TestQuoteService_WaiveInvoice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		invoicing *fakeInvoicingSwitch
		live      bool
		wantCode  domain.ErrorCode
	}{
		{"waived", &fakeInvoicingSwitch{enabled: true}, false, ""},
		{"live invoice", &fakeInvoicingSwitch{enabled: true}, true, domain.CodeQuoteAlreadyInvoiced},
		{"invoicing off", &fakeInvoicingSwitch{enabled: false}, false, domain.CodeAccountInvoicingOff},
		{"switch not wired", nil, false, domain.CodeAccountInvoicingOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := quoteFixtureAt(&domain.Quote{ID: uuid.New(), CurrentStatus: domain.QuoteStatusAccepted})
			if tc.invoicing != nil {
				f.service.WithInvoicingSwitch(*tc.invoicing)
			}
			f.quotes.liveInvoice = tc.live

			waived, err := f.service.WaiveInvoice(context.Background(), f.tenant, f.quoteID)

			if tc.wantCode != "" {
				if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != tc.wantCode || len(f.quotes.waiverWrites) != 0 {
					t.Fatalf("err = %v, writes = %d, want 409 %s and nothing written", err, len(f.quotes.waiverWrites), tc.wantCode)
				}
				return
			}
			if err != nil || waived.InvoiceWaivedAt == nil || *waived.InvoiceWaivedByUserID != f.tenant.UserID {
				t.Fatalf("WaiveInvoice() = %+v, %v, want waived by the caller", waived, err)
			}
			restored, err := f.service.RestoreInvoiceWait(context.Background(), f.tenant, f.quoteID)
			if err != nil || restored.InvoiceWaivedAt != nil {
				t.Fatalf("RestoreInvoiceWait() = %+v, %v, want the waiver gone", restored, err)
			}
		})
	}
}

// A paid sale is reopened only after its mark is taken back, whichever way it is reopened.
func TestQuoteService_Reactivate_RefusesAPaidSale(t *testing.T) {
	t.Parallel()
	for _, mode := range []domain.QuoteReactivationMode{domain.QuoteReactivationResend, domain.QuoteReactivationEdit} {
		versionID, payer := uuid.New(), uuid.New()
		now := time.Now()
		quote := &domain.Quote{ID: uuid.New(), CurrentVersionID: &versionID,
			CurrentStatus: domain.QuoteStatusAccepted, PaidAt: &now, PaidByUserID: &payer}
		f := quoteFixtureAt(quote)
		f.quotes.version = &domain.QuoteVersion{ID: versionID, QuoteID: quote.ID, IsImmutable: true}

		_, err := f.service.Reactivate(context.Background(), f.tenant, quote.ID, mode)

		if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != domain.CodeQuoteAlreadyPaid ||
			len(f.quotes.statusUpdates) != 0 || len(f.quotes.createdVersions) != 0 {
			t.Fatalf("%s: err = %v, want 409 QUOTE_ALREADY_PAID and the quote untouched", mode, err)
		}
	}
}

// Reopening a sale clears «No requiere factura» in the same transaction as the transition.
func TestQuoteService_Reactivate_ClearsTheInvoiceWaiver(t *testing.T) {
	t.Parallel()
	versionID, waiver := uuid.New(), uuid.New()
	now := time.Now()
	quote := &domain.Quote{ID: uuid.New(), CurrentVersionID: &versionID,
		CurrentStatus: domain.QuoteStatusAccepted, InvoiceWaivedAt: &now, InvoiceWaivedByUserID: &waiver}
	f := quoteFixtureAt(quote)
	f.quotes.version = &domain.QuoteVersion{ID: versionID, QuoteID: quote.ID, IsImmutable: true}

	moved, err := f.service.Reactivate(context.Background(), f.tenant, quote.ID, domain.QuoteReactivationResend)

	if err != nil || len(f.quotes.statusUpdates) != 1 || f.quotes.statusUpdates[0].to != domain.QuoteStatusQuoted ||
		moved.InvoiceWaivedAt != nil {
		t.Fatalf("Reactivate() = %+v, %v, want QUOTED with the waiver cleared", moved, err)
	}
	if len(f.quotes.waiverWrites) != 1 || f.quotes.waiverWrites[0] != nil || f.db.transactions != 1 {
		t.Fatalf("waiver writes = %v in %d transactions, want one clear in the reactivation's", f.quotes.waiverWrites, f.db.transactions)
	}
}
