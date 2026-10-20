//go:build integration

package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// invoicingBackfill is the UPDATE the 00057 migration runs, read from the file so the test pins
// what ships rather than a copy of it.
func invoicingBackfill(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../migrations/00057_account_invoicing_enabled.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(strings.Split(string(raw), "-- +goose Down")[0], ";")
	for _, statement := range up {
		if trimmed := strings.TrimSpace(statement); strings.Contains(trimmed, "UPDATE account") {
			return trimmed
		}
	}
	t.Fatal("the migration has no backfill")
	return ""
}

// The migration starts invoicing on for an account connected to ARCA or holding any invoice, and
// off for the rest. It runs inside a transaction that is rolled back, so no other account changes.
func TestAccountMigration_InvoicingBackfill(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	invoiced := tenant.AccountID
	connected := seedAccount(t, db, "Connected")
	bare := seedAccount(t, db, "Bare")
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO arca_setup (account_id, environment, tax_id, csr, sealed_key)
		 VALUES ($1, 'HOMOLOGATION', '30712345671', 'csr', 'sealed')`, connected); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM arca_setup WHERE account_id = $1`, connected)
	})
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, err := NewInvoicingRepository().CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ids := []uuid.UUID{invoiced, connected, bare}
	if _, err := tx.Exec(ctx, `UPDATE account SET invoicing_enabled = FALSE WHERE id = ANY($1)`, ids); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, invoicingBackfill(t)); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uuid.UUID]bool{invoiced: true, connected: true, bare: false} {
		var got bool
		if err := tx.QueryRow(ctx, `SELECT invoicing_enabled FROM account WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("account %s invoicing_enabled = %v after the backfill, want %v", id, got, want)
		}
	}
}

// The switch round-trips, the account read says whether an authorized voucher exists, and the share
// lock reports the switch it holds. A pending or refused attempt is no fiscal record.
func TestAccountRepository_InvoicingSwitch(t *testing.T) {
	db, tenant, quoteID, versionID, _ := invoiceFixture(t)
	ctx := context.Background()
	accounts, invoices := NewAccountRepository(), NewInvoicingRepository()

	var fresh, on, whilePending, afterRefusal, locked *domain.Account
	var lockedEnabled, readEnabled bool
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var err error
		if fresh, err = accounts.GetByID(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		if on, err = accounts.SetInvoicingEnabled(ctx, q, tenant.AccountID, true); err != nil {
			return err
		}
		refused, err := invoices.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		if whilePending, err = accounts.GetByID(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		if err = invoices.MarkRejected(ctx, q, tenant.AccountID, tenant.BranchID, refused.ID,
			domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo"}}); err != nil {
			return err
		}
		if afterRefusal, err = accounts.GetByID(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		pending, err := invoices.CreatePending(ctx, q, pendingInvoice(tenant, quoteID, versionID))
		if err != nil {
			return err
		}
		if _, err = invoices.MarkIssued(ctx, q, tenant.AccountID, tenant.BranchID, pending.ID,
			domain.InvoiceAuthorization{Number: 23, CAE: "74123456789012",
				CAEExpiresOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}); err != nil {
			return err
		}
		if locked, err = accounts.GetByIDForUpdate(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		if readEnabled, err = invoices.InvoicingEnabled(ctx, q, tenant.AccountID); err != nil {
			return err
		}
		lockedEnabled, err = invoices.LockAccountForShare(ctx, q, tenant.AccountID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if fresh.InvoicingEnabled || fresh.HasInvoices {
		t.Fatalf("new account = %+v, want invoicing off and no invoices", fresh)
	}
	if whilePending.HasInvoices || afterRefusal.HasInvoices {
		t.Fatalf("has_invoices = %v pending, %v refused, want false for both", whilePending.HasInvoices,
			afterRefusal.HasInvoices)
	}
	if !on.InvoicingEnabled || !locked.InvoicingEnabled || !locked.HasInvoices {
		t.Fatalf("after switching on and invoicing: returned = %+v, read = %+v", on, locked)
	}
	if !readEnabled || !lockedEnabled {
		t.Fatalf("InvoicingEnabled = %v, LockAccountForShare = %v, want both on", readEnabled, lockedEnabled)
	}

	var off bool
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if _, err := accounts.SetInvoicingEnabled(ctx, q, tenant.AccountID, false); err != nil {
			return err
		}
		var err error
		off, err = invoices.LockAccountForShare(ctx, q, tenant.AccountID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if off {
		t.Fatal("LockAccountForShare reports invoicing on after it was switched off")
	}
}
