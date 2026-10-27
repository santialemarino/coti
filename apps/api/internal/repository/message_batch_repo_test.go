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

func TestMessageBatchRepository_OpenOrExtendResetsUpToTheCap(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "window account")
	branchID := branchOf(t, db, accountID)
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, seedProduct(t, db, accountID, "cemento"))
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM message_batch WHERE quote_id = $1`, quoteID)
	})
	repo := NewMessageBatchRepository()
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	opened := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)

	window := func(at time.Time) *domain.MessageBatch {
		t.Helper()
		var batch *domain.MessageBatch
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			batch, err = repo.OpenOrExtend(ctx, q, accountID, branchID, quoteID, at,
				2*time.Minute, 10*time.Minute)
			return err
		}); err != nil {
			t.Fatalf("OpenOrExtend(%s) = %v, want no error", at, err)
		}
		return batch
	}

	first := window(opened)
	if first.Status != domain.MessageBatchStatusOpen || !first.OpenedAt.Equal(opened) ||
		!first.ClosesAt.Equal(opened.Add(2*time.Minute)) {
		t.Fatalf("opened window = %+v, want OPEN at 12:00 closing 12:02", first)
	}
	extended := window(opened.Add(time.Minute))
	if extended.ID != first.ID || !extended.OpenedAt.Equal(opened) ||
		!extended.ClosesAt.Equal(opened.Add(3*time.Minute)) {
		t.Errorf("extended window = %+v, want the same window closing 12:03", extended)
	}
	capped := window(opened.Add(9 * time.Minute))
	if capped.ID != first.ID || !capped.ClosesAt.Equal(opened.Add(10*time.Minute)) {
		t.Errorf("late window = %+v, want the same window capped at 12:10", capped)
	}
}

// RLS is off on the owner pool, so only the repository's own predicates refuse another tenant.
func TestMessageBatchRepository_OpenOrExtendRefusesAnotherAccountOrBranch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountID := seedAccount(t, db, "window owner")
	branchID := branchOf(t, db, accountID)
	otherAccount := seedAccount(t, db, "window stranger")
	otherBranch := seedExtraBranch(t, db, accountID, "Sucursal Norte")
	quoteID, _, _ := seedQuoteChain(t, db, accountID, branchID, seedProduct(t, db, accountID, "arena"))
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM message_batch WHERE quote_id = $1`, quoteID)
	})
	repo := NewMessageBatchRepository()

	for name, scope := range map[string][2]uuid.UUID{
		"other account": {otherAccount, branchID},
		"other branch":  {accountID, otherBranch},
	} {
		tx, err := db.AdminTx(ctx)
		if err != nil {
			t.Fatalf("AdminTx() = %v", err)
		}
		_, err = repo.OpenOrExtend(ctx, tx, scope[0], scope[1], quoteID, time.Now(), time.Minute,
			time.Hour)
		_ = tx.Rollback(ctx)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: OpenOrExtend() = %v, want ErrNotFound", name, err)
		}
	}
}
