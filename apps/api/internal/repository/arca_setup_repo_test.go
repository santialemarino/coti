//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestARCASetupRepository_TenantIsolation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := NewARCASetupRepository()
	a := seedAccount(t, db, "ARCA test A")
	b := seedAccount(t, db, "ARCA test B")
	setup := domain.ARCASetup{ID: uuid.New(), TaxID: "20329642330", CSR: "public", SealedKey: "encrypted"}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.Create(ctx, q, a, setup) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mustCleanup(t, db.CrossAccount(), `DELETE FROM arca_setup WHERE account_id = $1`, a) })
	for account, want := range map[uuid.UUID]bool{a: true, b: false} {
		var exists bool
		if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
			var err error
			exists, err = repo.Exists(ctx, q, account)
			return err
		}); err != nil || exists != want {
			t.Fatalf("Exists(%s) = %v, %v, want %v", account, exists, err, want)
		}
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: b}, func(q Querier) error {
		_, err := repo.Get(ctx, q, a)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("RLS leaked account A: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, db.CrossAccount(), b); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("account predicate leaked A: %v", err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.Create(ctx, q, a, setup) }); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second setup = %v, want ErrConflict", err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: a}, func(q Querier) error { return repo.Delete(ctx, q, a) }); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, db.CrossAccount(), a); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete = %v, want ErrNotFound", err)
	}
}
