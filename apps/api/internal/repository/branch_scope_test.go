//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// The branch scope is what confines a seller who sends no X-Branch-Id. These tests pin the
// two halves of it: which ids the scope resolves to, and how a per-branch read behaves for a
// nil, populated, or empty set.

func TestBranchRepository_ListIDsForUser(t *testing.T) {
	db := testDB(t)
	repo := NewBranchRepository()
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralón A")
	firstA := branchOf(t, db, accountA)
	secondA := seedExtraBranch(t, db, accountA, "Sucursal Dos")
	accountB := seedAccount(t, db, "Corralón B")
	branchB := branchOf(t, db, accountB)

	seller := seedUser(t, db, accountA, "SELLER")
	admin := seedUser(t, db, accountA, "ADMIN")
	linkUserBranch(t, db, accountA, seller, firstA)
	// An assignment reaching into another account must not widen the scope.
	linkUserBranch(t, db, accountB, seller, branchB)

	list := func(t *testing.T, userID uuid.UUID, isAdmin bool) []uuid.UUID {
		t.Helper()
		var ids []uuid.UUID
		if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
			var listErr error
			ids, listErr = repo.ListIDsForUser(ctx, q, accountA, userID, isAdmin)
			return listErr
		}); err != nil {
			t.Fatalf("ListIDsForUser() = %v, want no error", err)
		}
		return ids
	}

	sellerIDs := list(t, seller, false)
	if len(sellerIDs) != 1 || sellerIDs[0] != firstA {
		t.Errorf("seller scope = %v, want [%v]", sellerIDs, firstA)
	}

	adminIDs := list(t, admin, true)
	if len(adminIDs) != 2 {
		t.Errorf("admin scope = %v, want both branches of the account", adminIDs)
	}
	for _, id := range adminIDs {
		if id != firstA && id != secondA {
			t.Errorf("admin scope contains %v, which is not a branch of the account", id)
		}
	}

	// A seller assigned nowhere must get an empty, non-nil set: nil means every branch.
	unassigned := seedUser(t, db, accountA, "SELLER")
	none := list(t, unassigned, false)
	if none == nil {
		t.Error("ListIDsForUser() = nil for an unassigned seller, want an empty set")
	}
	if len(none) != 0 {
		t.Errorf("ListIDsForUser() = %v for an unassigned seller, want an empty set", none)
	}
}

func TestBranchRepository_ExistAllInAccount(t *testing.T) {
	db := testDB(t)
	repo := NewBranchRepository()
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralón A")
	firstA := branchOf(t, db, accountA)
	secondA := seedExtraBranch(t, db, accountA, "Sucursal Dos")
	accountB := seedAccount(t, db, "Corralón B")
	branchB := branchOf(t, db, accountB)

	cases := []struct {
		name string
		ids  []uuid.UUID
		want bool
	}{
		{"one of ours", []uuid.UUID{firstA}, true},
		{"both of ours", []uuid.UUID{firstA, secondA}, true},
		{"a repeated id still counts once", []uuid.UUID{firstA, firstA}, true},
		{"another account's branch", []uuid.UUID{branchB}, false},
		{"ours plus another account's", []uuid.UUID{firstA, branchB}, false},
		{"an id that exists nowhere", []uuid.UUID{uuid.New()}, false},
		{"an empty set is vacuously true", []uuid.UUID{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
				var checkErr error
				got, checkErr = repo.ExistAllInAccount(ctx, q, accountA, tc.ids)
				return checkErr
			}); err != nil {
				t.Fatalf("ExistAllInAccount() = %v, want no error", err)
			}
			if got != tc.want {
				t.Errorf("ExistAllInAccount(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}

// The branch filter rides into SQL as a uuid[]. Nil has to arrive as NULL (read everything)
// and an empty slice as '{}' (read nothing) — if pgx collapsed the two, a seller with no
// assignments would read the whole account.
func TestProductPriceRepository_BranchFilterArraySemantics(t *testing.T) {
	db := testDB(t)
	priceRepo := NewProductPriceRepository()
	availabilityRepo := NewBranchProductRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	first := branchOf(t, db, accountID)
	second := seedExtraBranch(t, db, accountID, "Sucursal Dos")
	productID := seedProduct(t, db, accountID, "Cemento Portland 50kg")
	priceCleanup(t, db, productID)
	tenant := domain.Tenant{AccountID: accountID}
	priceStart := time.Now().Add(-time.Hour)

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		for _, branchID := range []uuid.UUID{first, second} {
			if _, err := availabilityRepo.Save(ctx, q, accountID, branchID, productID,
				domain.BranchAvailability{IsActive: true}); err != nil {
				return err
			}
			if _, err := priceRepo.Create(ctx, q, accountID, branchID, productID, nil,
				domain.NewProductPrice{
					Price:     decimal.RequireFromString("1000.00"),
					Currency:  domain.DefaultCurrency,
					ValidFrom: priceStart,
				}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed prices: %v", err)
	}

	cases := []struct {
		name   string
		filter []uuid.UUID
		want   int
	}{
		{"nil reads every branch", nil, 2},
		{"one branch reads only it", []uuid.UUID{first}, 1},
		{"both branches read both", []uuid.UUID{first, second}, 2},
		{"an empty set reads nothing", []uuid.UUID{}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var prices []domain.ProductPrice
			var availability []domain.BranchProduct
			if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
				var listErr error
				prices, listErr = priceRepo.ListByProduct(ctx, q, accountID, productID, tc.filter)
				if listErr != nil {
					return listErr
				}
				availability, listErr = availabilityRepo.ListByProduct(ctx, q, accountID, productID, tc.filter)
				return listErr
			}); err != nil {
				t.Fatalf("list = %v, want no error", err)
			}
			if len(prices) != tc.want {
				t.Errorf("prices = %d rows, want %d", len(prices), tc.want)
			}
			if len(availability) != tc.want {
				t.Errorf("availability = %d rows, want %d", len(availability), tc.want)
			}
		})
	}
}
