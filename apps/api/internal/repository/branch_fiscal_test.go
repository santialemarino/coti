//go:build integration

package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"testing"
)

func TestBranchRepository_ManualPointOfSale(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db, "Branch fiscal settings")
	branchID := branchOf(t, db, accountID)
	second := seedExtraBranch(t, db, accountID, "Second")
	otherAccount := seedAccount(t, db, "Other issuer")
	otherBranch := branchOf(t, db, otherAccount)
	repo := NewBranchRepository()
	ctx := context.Background()
	update := func(account, branch uuid.UUID, point *int) (*domain.Branch, error) {
		var got *domain.Branch
		err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
			var err error
			got, err = repo.Update(ctx, q, account, branch, domain.BranchUpdate{Name: "Branch", DefaultExpiryDays: 7, PointOfSale: point})
			return err
		})
		return got, err
	}
	one := 1
	got, err := update(accountID, branchID, &one)
	if err != nil || got.PointOfSale == nil || *got.PointOfSale != 1 {
		t.Fatalf("manual save: branch=%+v err=%v", got, err)
	}
	got, err = update(accountID, branchID, nil)
	if err != nil || got.PointOfSale == nil || *got.PointOfSale != 1 {
		t.Fatalf("omitted value: branch=%+v err=%v", got, err)
	}
	if _, err = update(accountID, second, &one); domain.CodeOf(err) != domain.CodePointOfSaleTaken {
		t.Fatalf("duplicate number: %v", err)
	}
	if _, err = update(otherAccount, otherBranch, &one); err != nil {
		t.Fatalf("independent account: %v", err)
	}
	if err = db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		point, err := NewInvoicingRepository().GetBranchPointOfSale(ctx, q, accountID, branchID)
		if err == nil && (point == nil || *point != 1) {
			t.Fatalf("invoice uses %v, want 1", point)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	got, err = update(accountID, branchID, &zero)
	if err != nil || got.PointOfSale != nil {
		t.Fatalf("clear: branch=%+v err=%v", got, err)
	}
}
