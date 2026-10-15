package services

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type invoiceDocumentStore interface {
	GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Invoice, error)
	List(context.Context, repository.Querier, uuid.UUID, []uuid.UUID, int, int) ([]domain.Invoice, int, error)
}

// List returns a bounded page of the fiscal records in the caller's branch reach: the active
// branch, or every branch the caller may read when none is selected.
func (s *InvoiceService) List(ctx context.Context, tenant domain.Tenant, page int) ([]domain.Invoice, int, error) {
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
		if invoices, total, err = store.List(ctx, q, tenant.AccountID, tenant.BranchFilter(), 25, (page-1)*25); err != nil {
			return err
		}
		return s.linkVouchers(ctx, q, tenant.AccountID, invoices)
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
		if invoice, err = store.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, id); err != nil {
			return err
		}
		linked := []domain.Invoice{*invoice}
		if err := s.linkVouchers(ctx, q, tenant.AccountID, linked); err != nil {
			return err
		}
		invoice = &linked[0]
		return nil
	})
	return invoice, err
}

// linkVouchers sets each invoice's newest credit note and each credit note's invoice, in two reads.
func (s *InvoiceService) linkVouchers(ctx context.Context, q repository.Querier, accountID uuid.UUID, vouchers []domain.Invoice) error {
	invoices := make([]*domain.Invoice, 0, len(vouchers))
	creditedIDs := make([]uuid.UUID, 0)
	for i := range vouchers {
		if vouchers[i].CreditedInvoiceID != nil {
			creditedIDs = append(creditedIDs, *vouchers[i].CreditedInvoiceID)
		} else {
			invoices = append(invoices, &vouchers[i])
		}
	}
	if err := s.attachCreditNotes(ctx, q, accountID, invoices...); err != nil {
		return err
	}
	if len(creditedIDs) == 0 {
		return nil
	}
	credited, err := s.store.GetByIDs(ctx, q, accountID, uniqueUUIDs(creditedIDs))
	if err != nil {
		return err
	}
	for i := range vouchers {
		if id := vouchers[i].CreditedInvoiceID; id != nil {
			if inv, ok := credited[*id]; ok {
				vouchers[i].CreditedInvoice = &inv
			}
		}
	}
	return nil
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
	return s.renderer.Render(*invoice, s.loadInvoiceLogo(ctx, tenant.AccountID, invoice.Snapshot.Supplier.LogoURL))
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
		creds, err = s.openCredentials(ctx, q, tenant, invoice.Environment)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, domain.WithCode(inProgressCode(*invoice), domain.ErrConflict)
	}
	if _, err := s.reconcile(ctx, tenant, creds, *invoice); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, tenant, id)
}

func (s *InvoiceService) loadInvoiceLogo(ctx context.Context, accountID uuid.UUID, rawURL *string) *domain.BrandLogo {
	if rawURL == nil || s.logos == nil || s.settings.LogoMaxBytes <= 0 || s.settings.LogoMaxPixels <= 0 {
		return nil
	}
	parsed, err := url.Parse(*rawURL)
	if err != nil {
		return nil
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "v1" || parts[1] != "public" || parts[2] != "account-logos" || parts[3] != accountID.String() {
		return nil
	}
	id, err := uuid.Parse(parts[4])
	if err != nil {
		return nil
	}
	object, err := s.logos.DownloadLogo(ctx, accountID, id)
	if err != nil || object == nil || object.Body == nil {
		return nil
	}
	defer object.Body.Close()
	content, err := io.ReadAll(io.LimitReader(object.Body, s.settings.LogoMaxBytes+1))
	if err != nil || int64(len(content)) > s.settings.LogoMaxBytes {
		return nil
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil || imageConfig.Width <= 0 || imageConfig.Height <= 0 || int64(imageConfig.Width)*int64(imageConfig.Height) > s.settings.LogoMaxPixels {
		return nil
	}
	_, format, err := image.Decode(bytes.NewReader(content))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil
	}
	contentType := "image/png"
	if format == "jpeg" {
		contentType = "image/jpeg"
	}
	return &domain.BrandLogo{Bytes: content, ContentType: contentType}
}
