//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These tests prove the catalog's account boundary against a real database: the explicit
// account_id predicate and the row level security policy together, through the restricted
// role the API actually connects as.

// seedProduct inserts a product through the owner pool and removes it when the test ends.
func seedProduct(t *testing.T, db *DB, accountID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, $3)`,
		id, accountID, name); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.CrossAccount().Exec(ctx,
			`DELETE FROM product_alternative WHERE base_product_id = $1 OR alternative_product_id = $1`, id)
		_, _ = db.CrossAccount().Exec(ctx, `DELETE FROM product_synonym WHERE product_id = $1`, id)
		_, _ = db.CrossAccount().Exec(ctx, `DELETE FROM product WHERE id = $1`, id)
	})
	return id
}

// The acceptance criterion of the catalog ticket: a product of one account is neither
// visible nor modifiable from another. Read, list, update, and delete are all checked,
// because a missing predicate on any one of them is the same leak.
func TestProductRepository_IsInvisibleToAnotherAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")

	repo := NewProductRepository()
	scopeB := domain.Tenant{AccountID: accountB}

	t.Run("get by id", func(t *testing.T) {
		err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
			_, getErr := repo.GetByID(ctx, q, accountB, productA)
			return getErr
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetByID() from another account = %v, want %v", err, domain.ErrNotFound)
		}
	})

	// The account id is spoofed here on purpose: even if the predicate carried account A,
	// the policy denies the read, so B cannot reach the row by asking for it directly.
	t.Run("get by id claiming the owner account", func(t *testing.T) {
		err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
			_, getErr := repo.GetByID(ctx, q, accountA, productA)
			return getErr
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetByID() with a spoofed account = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("list", func(t *testing.T) {
		var page domain.ProductPage
		if err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
			var listErr error
			page, listErr = repo.List(ctx, q, accountB, domain.ProductFilter{Limit: 100})
			return listErr
		}); err != nil {
			t.Fatalf("InTenantTx() = %v, want no error", err)
		}
		for _, p := range page.Items {
			if p.ID == productA {
				t.Fatalf("List() from account B returned account A's product %v", productA)
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
			_, updErr := repo.Update(ctx, q, accountB, productA, domain.ProductUpdate{
				CanonicalName: "Hijacked",
			})
			return updErr
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Update() from another account = %v, want %v", err, domain.ErrNotFound)
		}

		var name string
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT canonical_name FROM product WHERE id = $1`, productA).Scan(&name); err != nil {
			t.Fatalf("read product: %v", err)
		}
		if name != "Cemento Portland 50kg" {
			t.Errorf("canonical_name after the foreign update = %q, want it untouched", name)
		}
	})

	t.Run("delete", func(t *testing.T) {
		err := db.InTenantTx(ctx, scopeB, func(q Querier) error {
			return repo.Delete(ctx, q, accountB, productA)
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Delete() from another account = %v, want %v", err, domain.ErrNotFound)
		}

		var isActive bool
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT is_active FROM product WHERE id = $1`, productA).Scan(&isActive); err != nil {
			t.Fatalf("read product: %v", err)
		}
		if !isActive {
			t.Error("is_active after the foreign delete = false, want the product still active")
		}
	})
}

// A synonym or an alternative pointing at another account's product would pass the foreign
// key — constraint checks bypass row level security — so the link is only ever written
// after the product has been read inside the tenant scope. This test pins the reason that
// read exists: the database alone does not stop it.
func TestProductRepository_ForeignKeysDoNotEnforceTheAccountBoundary(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")

	synonyms := NewProductSynonymRepository()
	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
		_, createErr := synonyms.Create(ctx, q, accountB, productA, "portland",
			domain.SynonymSourceManual)
		return createErr
	})
	if err != nil {
		t.Fatalf("Create() = %v; the foreign key was expected to accept it, which is why the "+
			"service reads the product inside the tenant scope first", err)
	}

	// Cleanup: the row belongs to account B and hangs off account A's product.
	_, _ = db.CrossAccount().Exec(ctx,
		`DELETE FROM product_synonym WHERE account_id = $1 AND product_id = $2`, accountB, productA)
}

func TestProductRepository_CreateRejectsADuplicateCode(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	repo := NewProductRepository()
	code := "CEM-" + uuid.NewString()[:8]

	var created uuid.UUID
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		p, createErr := repo.Create(ctx, q, accountA, domain.NewProduct{
			Code: &code, CanonicalName: "Cemento Portland 50kg",
		})
		if createErr != nil {
			return createErr
		}
		created = p.ID
		return nil
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	t.Cleanup(func() {
		_, _ = db.CrossAccount().Exec(context.Background(), `DELETE FROM product WHERE id = $1`, created)
	})

	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, domain.NewProduct{
			Code: &code, CanonicalName: "Cemento Portland 50kg (bis)",
		})
		return createErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Create() with a duplicate code = %v, want %v", err, domain.ErrConflict)
	}
}

// Two accounts may each carry the same code: the unique index is scoped to the account.
func TestProductRepository_CreateAllowsTheSameCodeInAnotherAccount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")
	repo := NewProductRepository()
	code := "CEM-" + uuid.NewString()[:8]

	for _, accountID := range []uuid.UUID{accountA, accountB} {
		var created uuid.UUID
		if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
			p, createErr := repo.Create(ctx, q, accountID, domain.NewProduct{
				Code: &code, CanonicalName: "Cemento Portland 50kg",
			})
			if createErr != nil {
				return createErr
			}
			created = p.ID
			return nil
		}); err != nil {
			t.Fatalf("Create() for account %v = %v, want no error", accountID, err)
		}
		t.Cleanup(func() {
			_, _ = db.CrossAccount().Exec(context.Background(), `DELETE FROM product WHERE id = $1`, created)
		})
	}
}

// The source column is a native enum, so the database is what refuses a value outside the
// closed set — not just the DTO's oneof tag. That matters because the learning pipeline and
// the bulk import will write this column without going through a request body.
func TestProductSynonymRepository_SourceRejectsAValueOutsideTheEnum(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")

	err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		_, execErr := q.Exec(ctx,
			`INSERT INTO product_synonym (account_id, product_id, term, source)
			 VALUES ($1, $2, 'portland', 'whatever')`,
			accountA, productA)
		return execErr
	})
	if err == nil {
		t.Fatal("INSERT with a free-text source = nil error, want the enum type to reject it")
	}
}

func TestProductSynonymRepository_CreateRejectsARepeatedTerm(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	productA := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	repo := NewProductSynonymRepository()
	tenant := domain.Tenant{AccountID: accountA}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, productA, "portland", domain.SynonymSourceManual)
		return createErr
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	// Different casing, same term to a matcher.
	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, productA, "Portland", domain.SynonymSourceLearned)
		return createErr
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Create() with a repeated term = %v, want %v", err, domain.ErrConflict)
	}
}

func TestProductAlternativeRepository_ListReadsBothDirections(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	base := seedProduct(t, db, accountA, "Cemento Portland 50kg")
	alternative := seedProduct(t, db, accountA, "Cemento albanileria 50kg")

	repo := NewProductAlternativeRepository()
	tenant := domain.Tenant{AccountID: accountA}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		_, createErr := repo.Create(ctx, q, accountA, base, alternative, domain.ProductAlternativeEconomy)
		return createErr
	}); err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}

	cases := []struct {
		name        string
		anchor      uuid.UUID
		direction   domain.AlternativeDirection
		wantFarEnd  uuid.UUID
		wantMatches int
	}{
		{"outgoing from the base", base, domain.AlternativeDirectionOutgoing, alternative, 1},
		{"incoming to the alternative", alternative, domain.AlternativeDirectionIncoming, base, 1},
		{"outgoing from the alternative", alternative, domain.AlternativeDirectionOutgoing, uuid.Nil, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var views []domain.ProductAlternativeView
			if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				var listErr error
				views, listErr = repo.List(ctx, q, accountA, tc.anchor, tc.direction)
				return listErr
			}); err != nil {
				t.Fatalf("InTenantTx() = %v, want no error", err)
			}
			if len(views) != tc.wantMatches {
				t.Fatalf("List() returned %d links, want %d", len(views), tc.wantMatches)
			}
			if tc.wantMatches > 0 && views[0].Product.ID != tc.wantFarEnd {
				t.Errorf("List() joined product = %v, want %v", views[0].Product.ID, tc.wantFarEnd)
			}
		})
	}
}
