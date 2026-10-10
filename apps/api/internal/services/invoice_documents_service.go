package services

import (
	"context"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type invoiceDocumentStore interface {
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Invoice, error)
	List(context.Context, repository.Querier, uuid.UUID, uuid.UUID, int, int) ([]domain.Invoice, int, error)
}

// List returns a bounded page of the active branch's fiscal records.
func (s *InvoiceService) List(ctx context.Context, tenant domain.Tenant, page int) ([]domain.Invoice, int, error) {
	if err := requireBranch(tenant, "invoices"); err != nil {
		return nil, 0, err
	}
	if page < 1 || page > 100000 {
		return nil, 0, domain.ErrInvalidInput
	}
	store, ok := s.store.(invoiceDocumentStore)
	if !ok {
		return nil, 0, domain.ErrNotConfigured
	}
	var invoices []domain.Invoice
	var total int
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		invoices, total, err = store.List(ctx, q, tenant.AccountID, tenant.BranchID, 25, (page-1)*25)
		return err
	})
	return invoices, total, err
}

// GetByID returns a fiscal record through tenant and branch boundaries.
func (s *InvoiceService) GetByID(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Invoice, error) {
	if err := requireBranch(tenant, "an invoice"); err != nil {
		return nil, err
	}
	store, ok := s.store.(invoiceDocumentStore)
	if !ok {
		return nil, domain.ErrNotConfigured
	}
	var invoice *domain.Invoice
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		invoice, err = store.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, id)
		return err
	})
	return invoice, err
}

// DownloadPDF renders an authorized record exclusively from its persisted snapshot.
func (s *InvoiceService) DownloadPDF(ctx context.Context, tenant domain.Tenant, id uuid.UUID) ([]byte, error) {
	invoice, err := s.GetByID(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	if invoice.Status != domain.InvoiceStatusIssued {
		return nil, domain.ErrConflict
	}
	if s.renderer == nil {
		return nil, domain.ErrNotConfigured
	}
	return s.renderer.Render(*invoice)
}

// Recover consults a stale pending record without requesting another authorization.
func (s *InvoiceService) Recover(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.Invoice, error) {
	invoice, err := s.GetByID(ctx, tenant, id)
	if err != nil {
		return nil, err
	}
	if invoice.Status != domain.InvoiceStatusPending {
		return invoice, nil
	}
	var creds domain.ARCACredentials
	claimed := false
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		claimed, err = s.store.ClaimRecovery(ctx, q, tenant.AccountID, tenant.BranchID, id, s.now().Add(-s.settings.ReconcileAfter))
		if err != nil || !claimed {
			return err
		}
		creds, err = s.openCredentials(ctx, q, tenant)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, domain.WithCode(domain.CodeInvoiceInProgress, domain.ErrConflict)
	}
	if _, err := s.reconcile(ctx, tenant, creds, *invoice); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, tenant, id)
}
