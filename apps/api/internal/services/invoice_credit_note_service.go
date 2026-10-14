package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// IssueCreditNote asks ARCA for the credit note that cancels an issued invoice in full, in its environment;
// a pending one holds the invoice, so a second press never authorizes a second note.
func (s *InvoiceService) IssueCreditNote(ctx context.Context, tenant domain.Tenant, invoiceID uuid.UUID) (*domain.Invoice, error) {
	if err := requireBranch(tenant, "a credit note"); err != nil {
		return nil, err
	}

	var invoice *domain.Invoice
	var creds, recovery domain.ARCACredentials
	var stale *domain.Invoice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		if invoice, err = s.creditableInvoice(ctx, q, tenant, invoiceID); err != nil {
			return err
		}
		if note := invoice.CreditNote; note != nil && note.Status == domain.InvoiceStatusPending {
			if !s.isStale(*note) {
				return domain.WithCode(domain.CodeCreditNoteInProgress, domain.ErrConflict)
			}
			stale = note
		}
		credential, err := s.store.GetCredentialStatus(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}
		if issues := creditNoteIssues(s.settings.Enabled, credential, *invoice, s.now()); len(issues) > 0 {
			return &domain.InvoiceNotReadyError{Issues: issues}
		}
		// A credit note is authorized, and read back, in the environment of the invoice it cancels.
		if stale != nil {
			if recovery, err = s.openCredentials(ctx, q, tenant, stale.Environment); err != nil {
				return err
			}
		}
		creds, err = s.openCredentials(ctx, q, tenant, invoice.Environment)
		return err
	})
	if err != nil {
		return nil, err
	}

	// As with an invoice, the exchange outlives the seller's request once ARCA is being asked.
	detached := context.WithoutCancel(ctx)
	work, cancel := context.WithTimeout(detached, s.settings.IssueTimeout)
	defer cancel()

	if stale != nil {
		claimed := false
		err := s.db.InTenantTx(work, tenant, func(q repository.Querier) error {
			var err error
			claimed, err = s.store.ClaimRecovery(work, q, tenant.AccountID, tenant.BranchID, stale.ID, s.now().Add(-s.settings.ReconcileAfter))
			return err
		})
		if err != nil {
			return nil, err
		}
		if !claimed {
			return nil, domain.WithCode(domain.CodeCreditNoteInProgress, domain.ErrConflict)
		}
		adopted, err := s.reconcile(work, tenant, recovery, *stale)
		if err != nil || adopted != nil {
			return withCreditedInvoice(adopted, invoice), err
		}
	}

	pending, err := s.holdCreditNote(work, tenant, invoice.ID)
	if err != nil {
		return nil, err
	}
	auth, issueErr := s.issuer.Issue(work, creds, domain.InvoiceRequest{
		IssuerCUIT:  pending.IssuerCUIT,
		Kind:        domain.VoucherKindCreditNote,
		Type:        pending.Type,
		PointOfSale: pending.PointOfSale,
		Date:        pending.IssuedOn,
		Receiver:    pending.Receiver,
		Amounts:     pending.Amounts,
		Associated: &domain.AssociatedVoucher{
			Type: invoice.VoucherType(), PointOfSale: invoice.PointOfSale, Number: *invoice.Number,
			IssuerCUIT: invoice.IssuerCUIT, Date: invoice.IssuedOn,
		},
		Claim: func(ctx context.Context, number int64) error {
			return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
				return s.store.ClaimNumber(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID, number)
			})
		},
	})
	issued, err := s.settle(detached, tenant, *pending, auth, issueErr)
	return withCreditedInvoice(issued, invoice), err
}

// creditableInvoice loads an issued invoice of the active branch with its newest credit note, refusing
// anything a credit note cannot cancel.
func (s *InvoiceService) creditableInvoice(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, invoiceID uuid.UUID,
) (*domain.Invoice, error) {
	invoices, err := s.store.GetByIDs(ctx, q, tenant.AccountID, []uuid.UUID{invoiceID})
	if err != nil {
		return nil, err
	}
	invoice, ok := invoices[invoiceID]
	if !ok || invoice.BranchID != tenant.BranchID {
		return nil, domain.ErrNotFound
	}
	if invoice.Kind != domain.VoucherKindInvoice || invoice.Status != domain.InvoiceStatusIssued || invoice.Number == nil {
		return nil, domain.WithCode(domain.CodeInvoiceNotCreditable, domain.ErrConflict)
	}
	if err := s.attachCreditNotes(ctx, q, tenant.AccountID, &invoice); err != nil {
		return nil, err
	}
	if invoice.Credited() {
		return nil, domain.WithCode(domain.CodeInvoiceAlreadyCredited, domain.ErrConflict)
	}
	return &invoice, nil
}

// holdCreditNote writes the pending credit note that holds the invoice while ARCA is asked; the quote
// lock serializes it with a reactivation or a new invoice, the account lock with a CUIT change.
func (s *InvoiceService) holdCreditNote(ctx context.Context, tenant domain.Tenant, invoiceID uuid.UUID) (*domain.Invoice, error) {
	var pending *domain.Invoice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		invoice, err := s.creditableInvoice(ctx, q, tenant, invoiceID)
		if err != nil {
			return err
		}
		if _, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, invoice.QuoteID); err != nil {
			return err
		}
		if err := s.store.LockAccountForShare(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		// Read again under the locks: a credit note held meanwhile is the one this would duplicate.
		if invoice, err = s.creditableInvoice(ctx, q, tenant, invoiceID); err != nil {
			return err
		}
		if note := invoice.CreditNote; note != nil && note.Status == domain.InvoiceStatusPending {
			return domain.WithCode(domain.CodeCreditNoteInProgress, domain.ErrConflict)
		}
		pending, err = s.store.CreatePending(ctx, q, creditNoteFor(*invoice, domain.InvoiceDay(s.now())))
		if errors.Is(err, domain.ErrConflict) {
			return domain.WithCode(domain.CodeCreditNoteInProgress, domain.ErrConflict)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
}

// attachCreditNotes sets each issued invoice's newest credit note, in one read.
func (s *InvoiceService) attachCreditNotes(ctx context.Context, q repository.Querier, accountID uuid.UUID, invoices ...*domain.Invoice) error {
	ids := make([]uuid.UUID, 0, len(invoices))
	for _, inv := range invoices {
		if inv != nil && inv.Kind == domain.VoucherKindInvoice && inv.Status == domain.InvoiceStatusIssued {
			ids = append(ids, inv.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	notes, err := s.store.GetCreditNotesByInvoiceIDs(ctx, q, accountID, ids)
	if err != nil {
		return err
	}
	for _, inv := range invoices {
		if inv == nil {
			continue
		}
		// An attempt ARCA never answered is no verdict to show beside the invoice.
		if note, ok := notes[inv.ID]; ok && !released(note) {
			inv.CreditNote = &note
		}
	}
	return nil
}

// creditNoteFor is the credit note that cancels invoice in full, dated day but never before the invoice.
func creditNoteFor(invoice domain.Invoice, day time.Time) domain.Invoice {
	credited := invoice.ID
	if invoice.IssuedOn.After(day) {
		day = invoice.IssuedOn
	}
	return domain.Invoice{
		AccountID: invoice.AccountID, BranchID: invoice.BranchID, QuoteID: invoice.QuoteID,
		QuoteVersionID: invoice.QuoteVersionID, Environment: invoice.Environment,
		Kind: domain.VoucherKindCreditNote, CreditedInvoiceID: &credited,
		Type: invoice.Type, PointOfSale: invoice.PointOfSale, IssuedOn: day, IssuerCUIT: invoice.IssuerCUIT,
		Receiver: invoice.Receiver, Amounts: invoice.Amounts, Currency: invoice.Currency,
		Snapshot: invoice.Snapshot, IssuerCondition: invoice.IssuerCondition, IssuerProfile: invoice.IssuerProfile,
	}
}

// creditNoteIssues lists what stops a credit note: the connection must reach the invoice's environment and CUIT.
func creditNoteIssues(enabled bool, credential *domain.ARCACredentialStatus, invoice domain.Invoice, now time.Time) []string {
	var issues []string
	if !enabled {
		issues = append(issues, invoiceIssueDisabled)
	}
	switch {
	case credential == nil:
		issues = append(issues, invoiceIssueCredentials)
	case credential.Environment != invoice.Environment:
		issues = append(issues, invoiceIssueCredentialsEnv)
	case !credential.ExpiresAt.After(now):
		issues = append(issues, invoiceIssueCredentialsExpired)
	}
	if credential != nil && credential.CUIT != invoice.IssuerCUIT {
		issues = append(issues, invoiceIssueCredentialsCUIT)
	}
	return issues
}

// withCreditedInvoice sets the invoice a credit note cancels, so the answer carries what it names.
func withCreditedInvoice(note, invoice *domain.Invoice) *domain.Invoice {
	if note != nil && invoice != nil {
		credited := *invoice
		credited.CreditNote = nil
		note.CreditedInvoice = &credited
	}
	return note
}
