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

/*
 * The two branch lists answer different questions and must not drift into each other. One says
 * which branches the caller may operate in — active, assigned — and backs the switcher, so a
 * closed branch reaching it would let a session pin itself to one the API refuses on every
 * request. The other says which branches the account has, closed ones included, so an
 * administrator can reopen one.
 */
func TestBranchRepository_ClosedBranchesAreAdministrationOnly(t *testing.T) {
	db := testDB(t)
	repo := NewBranchRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón con una cerrada")
	open := branchOf(t, db, account)
	closed := seedExtraBranch(t, db, account, "Sucursal Cerrada")
	closeBranch(t, db, closed)
	admin := seedUser(t, db, account, "ADMIN")

	var reach, all []domain.Branch
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
		var err error
		if reach, err = repo.ListForUser(ctx, q, account, admin, true); err != nil {
			return err
		}
		all, err = repo.ListAllForAccount(ctx, q, account)
		return err
	}); err != nil {
		t.Fatalf("listing branches = %v, want no error", err)
	}

	if len(reach) != 1 || reach[0].ID != open {
		t.Errorf("reach = %v, want only the open branch %v", ids(reach), open)
	}
	if len(all) != 2 {
		t.Fatalf("account-wide list = %v, want both branches", ids(all))
	}
	if !containsBranch(all, closed) {
		t.Errorf("account-wide list = %v, want the closed branch %v", ids(all), closed)
	}

	// Closed ones sort last, so administering the live branches never means scrolling past them.
	if !all[0].IsActive || all[1].IsActive {
		t.Errorf("order = [%t %t], want the active branch first", all[0].IsActive, all[1].IsActive)
	}

	// A closed branch is not reachable either, which is what makes the selection unusable and so
	// what the interface has to clear when it closes the active one.
	var accessible bool
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
		var err error
		accessible, err = repo.IsAccessibleBy(ctx, q, account, admin, closed, true)
		return err
	}); err != nil {
		t.Fatalf("IsAccessibleBy() = %v, want no error", err)
	}
	if accessible {
		t.Error("IsAccessibleBy() = true for a closed branch, want false")
	}

	// And it is still fetchable by id, which is what keeps a quote that came in through it
	// explainable after it closes.
	var fetched *domain.Branch
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
		var err error
		fetched, err = repo.GetByID(ctx, q, account, closed)
		return err
	}); err != nil {
		t.Fatalf("GetByID() = %v, want no error", err)
	}
	if fetched.IsActive {
		t.Error("GetByID() reported the closed branch as active")
	}
}

func closeBranch(t *testing.T, db *DB, branchID uuid.UUID) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`UPDATE branch SET is_active = FALSE WHERE id = $1`, branchID); err != nil {
		t.Fatalf("closing branch: %v", err)
	}
}

func ids(branches []domain.Branch) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.ID)
	}
	return out
}

func containsBranch(branches []domain.Branch, id uuid.UUID) bool {
	for _, b := range branches {
		if b.ID == id {
			return true
		}
	}
	return false
}
