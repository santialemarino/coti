//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Archive is an orthogonal flag, not a transition: it never touches current_status, so a boxed
// SENT quote still reads SENT. It only refuses a second archive and a closed quote.
func TestQuoteRepository_Archive_BoxesAndUnboxes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Archive flag")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, archiveErr := repo.Archive(ctx, q, accountID, branchID, quoteID)
		return archiveErr
	}); err != nil {
		t.Fatalf("Archive() = %v, want no error", err)
	}

	var archivedAt *time.Time
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT archived_at FROM quote WHERE id = $1`, quoteID).Scan(&archivedAt); err != nil {
		t.Fatalf("read archived_at: %v", err)
	}
	if archivedAt == nil {
		t.Fatal("archived_at = NULL, want it set")
	}

	// A second archive matches no row and reads as a conflict.
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, archiveErr := repo.Archive(ctx, q, accountID, branchID, quoteID)
		return archiveErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second Archive() = %v, want ErrConflict", err)
	}

	// Unarchive brings it back.
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, unarchiveErr := repo.Unarchive(ctx, q, accountID, branchID, quoteID)
		return unarchiveErr
	}); err != nil {
		t.Fatalf("Unarchive() = %v, want no error", err)
	}
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT archived_at FROM quote WHERE id = $1`, quoteID).Scan(&archivedAt); err != nil {
		t.Fatalf("read archived_at: %v", err)
	}
	if archivedAt != nil {
		t.Errorf("archived_at = %v, want NULL after unarchive", archivedAt)
	}
}

// A closed quote (accepted or rejected) has no reason to be boxed away: the predicate refuses it
// even though the row exists.
func TestQuoteRepository_Archive_RefusesAClosedQuote(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Archive closed")
	branchID := branchOf(t, db, accountID)
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, Role: domain.UserRoleAdmin}
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, updateErr := repo.UpdateStatus(ctx, q, accountID, branchID, quoteID,
			domain.QuoteStatusDraft, domain.QuoteStatusAccepted)
		return updateErr
	}); err != nil {
		t.Fatalf("move to ACCEPTED: %v", err)
	}

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, archiveErr := repo.Archive(ctx, q, accountID, branchID, quoteID)
		return archiveErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Archive() on ACCEPTED = %v, want ErrConflict", err)
	}
	var archivedAt *time.Time
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT archived_at FROM quote WHERE id = $1`, quoteID).Scan(&archivedAt); err != nil {
		t.Fatalf("read archived_at: %v", err)
	}
	if archivedAt != nil {
		t.Errorf("archived_at = %v, want NULL on a refused archive", archivedAt)
	}
}

// Another branch of the same account cannot archive, because the branch predicate is the only
// boundary row level security guards the account alone.
func TestQuoteRepository_Archive_NarrowsToTheQuotesBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "Archive branch scope")
	branchID := branchOf(t, db, accountID)
	otherBranchID := seedExtraBranch(t, db, accountID, "Sucursal Sur")
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, productID)

	repo := NewQuoteRepository()
	err := db.InTenantTx(ctx,
		domain.Tenant{AccountID: accountID, BranchID: otherBranchID, Role: domain.UserRoleAdmin},
		func(q Querier) error {
			_, archiveErr := repo.Archive(ctx, q, accountID, otherBranchID, quoteID)
			return archiveErr
		})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Archive() from another branch = %v, want ErrConflict", err)
	}
}
