//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestARCASetupRepository_TenantAndBranchIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := NewARCASetupRepository()
	a := seedAccount(t, db, "ARCA test A")
	b := seedAccount(t, db, "ARCA test B")
	var branchA, branchB uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx, `SELECT id FROM branch WHERE account_id = $1`, a).Scan(&branchA); err != nil {
		t.Fatal(err)
	}
	if err := db.CrossAccount().QueryRow(ctx, `SELECT id FROM branch WHERE account_id = $1`, b).Scan(&branchB); err != nil {
		t.Fatal(err)
	}
	setup := domain.ARCASetup{ID: uuid.New(), TaxID: "20329642330", CSR: "public", SealedKey: "encrypted"}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.Create(ctx, q, a, setup) }); err != nil {
		t.Fatal(err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: b}, func(q Querier) error {
		_, err := repo.Get(ctx, q, a, branchA)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("RLS leaked account A: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, db.CrossAccount(), b, branchA); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("account predicate leaked A: %v", err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.SaveBranch(ctx, q, a, branchB, setup.ID, 1) }); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("accepted foreign branch: %v", err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.SaveBranch(ctx, q, a, branchA, setup.ID, 1) }); err != nil {
		t.Fatal(err)
	}
	read, err := repo.Get(ctx, db.CrossAccount(), a, branchB)
	if err != nil || read.PointOfSale != 0 || read.VerifiedAt != nil {
		t.Fatal("branch verification leaked")
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.Delete(ctx, q, a) }); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM branch_arca_setup WHERE account_id = $1`, a).Scan(&count); err != nil || count != 0 {
		t.Fatal("disconnect left branch verification")
	}
}
