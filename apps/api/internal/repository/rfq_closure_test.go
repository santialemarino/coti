//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// closureOf reads the order of quoteID the way the detail does, for a server in env.
func closureOf(t *testing.T, db *DB, tenant domain.Tenant, quoteID uuid.UUID, env domain.ARCAEnvironment) domain.SaleClosure {
	t.Helper()
	ctx := context.Background()
	var rfqID uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx, `SELECT rfq_id FROM quote WHERE id = $1`, quoteID).Scan(&rfqID); err != nil {
		t.Fatal(err)
	}
	var item *domain.RfqListItem
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		item, err = NewRFQRepository().GetByRFQID(ctx, q, tenant, rfqID, env)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return item.Closure
}

// An issued invoice stands for the sale only in the environment it was issued in (or production), a
// voucher waiting for ARCA is reported in any environment, and a refused one is neither.
func TestRFQRepository_ClosureReadsTheInvoiceByEnvironment(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoices := NewInvoicingRepository()

	var pending *domain.Invoice
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		pending, err = invoices.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []domain.ARCAEnvironment{domain.ARCAEnvironmentHomologation, domain.ARCAEnvironmentProduction} {
		if closure := closureOf(t, db, tenant, quoteID, env); !closure.InvoicePending || closure.Invoiced {
			t.Fatalf("a homologation invoice pending, read from %s: %+v, want pending and not invoiced", env, closure)
		}
	}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := invoices.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID,
			domain.InvoiceAuthorization{Number: 23, CAE: "74123456789012",
				CAEExpiresOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if closure := closureOf(t, db, tenant, quoteID, domain.ARCAEnvironmentHomologation); !closure.Invoiced || closure.InvoicePending {
		t.Fatalf("issued in homologation, read from homologation: %+v, want invoiced", closure)
	}
	if closure := closureOf(t, db, tenant, quoteID, domain.ARCAEnvironmentProduction); closure.Invoiced || closure.InvoicePending {
		t.Fatalf("issued in homologation, read from production: %+v, want not invoiced", closure)
	}
}
