//go:build integration

package repository

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Replace is the whole assignment set, so the interesting cases are the ones that shrink it:
// a removed branch has to disappear, and an empty set has to clear the user entirely.
func TestUserBranchRepository_ReplaceIsTheWholeSet(t *testing.T) {
	db := testDB(t)
	repo := NewUserBranchRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	first := branchOf(t, db, accountID)
	second := seedExtraBranch(t, db, accountID, "Sucursal Dos")
	userID := seedUser(t, db, accountID, "SELLER")
	tenant := domain.Tenant{AccountID: accountID}

	replace := func(t *testing.T, ids []uuid.UUID) []uuid.UUID {
		t.Helper()
		var got map[uuid.UUID][]uuid.UUID
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			if err := repo.Replace(ctx, q, accountID, userID, ids); err != nil {
				return err
			}
			var listErr error
			got, listErr = repo.ListByUsers(ctx, q, accountID, []uuid.UUID{userID})
			return listErr
		}); err != nil {
			t.Fatalf("Replace(%v) = %v, want no error", ids, err)
		}
		return got[userID]
	}

	if got := replace(t, []uuid.UUID{first, second}); len(got) != 2 {
		t.Errorf("after assigning both, got %v, want 2 branches", got)
	}

	// Shrinking must drop the branch that left the set.
	got := replace(t, []uuid.UUID{second})
	if !reflect.DeepEqual(got, []uuid.UUID{second}) {
		t.Errorf("after shrinking, got %v, want [%v]", got, second)
	}

	// Repeating the same set is idempotent, because uq_user_branch is the conflict target.
	if got := replace(t, []uuid.UUID{second}); !reflect.DeepEqual(got, []uuid.UUID{second}) {
		t.Errorf("after repeating, got %v, want [%v]", got, second)
	}

	// An empty set clears the user. A nil array would make `<> ALL` yield NULL and delete
	// nothing, which is why the statement coalesces it.
	if got := replace(t, []uuid.UUID{}); len(got) != 0 {
		t.Errorf("after clearing, got %v, want none", got)
	}
	if got := replace(t, nil); len(got) != 0 {
		t.Errorf("after clearing with nil, got %v, want none", got)
	}
}

// ListByUsers is one query for many users, so a list screen never walks the table per row.
func TestUserBranchRepository_ListByUsersIsBatched(t *testing.T) {
	db := testDB(t)
	repo := NewUserBranchRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	branchID := branchOf(t, db, accountID)
	first := seedUser(t, db, accountID, "SELLER")
	second := seedUser(t, db, accountID, "SELLER")
	third := seedUser(t, db, accountID, "SELLER")
	linkUserBranch(t, db, accountID, first, branchID)
	linkUserBranch(t, db, accountID, second, branchID)

	var got map[uuid.UUID][]uuid.UUID
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var listErr error
		got, listErr = repo.ListByUsers(ctx, q, accountID, []uuid.UUID{first, second, third})
		return listErr
	}); err != nil {
		t.Fatalf("ListByUsers() = %v, want no error", err)
	}

	if len(got[first]) != 1 || len(got[second]) != 1 {
		t.Errorf("assigned users got %v and %v, want one branch each", got[first], got[second])
	}
	// A user with no assignment must be absent rather than carry an empty entry, so the
	// caller can tell "none" from "not asked about".
	if _, present := got[third]; present {
		t.Errorf("unassigned user has an entry %v, want none", got[third])
	}
}

// Another account's user_branch rows must be invisible even though the ids are guessable.
func TestUserBranchRepository_AnotherAccountsAssignmentsAreInvisible(t *testing.T) {
	db := testDB(t)
	repo := NewUserBranchRepository()
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralón A")
	accountB := seedAccount(t, db, "Corralón B")
	branchB := branchOf(t, db, accountB)
	userB := seedUser(t, db, accountB, "SELLER")
	linkUserBranch(t, db, accountB, userB, branchB)

	var got map[uuid.UUID][]uuid.UUID
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		var listErr error
		got, listErr = repo.ListByUsers(ctx, q, accountA, []uuid.UUID{userB})
		return listErr
	}); err != nil {
		t.Fatalf("ListByUsers() = %v, want no error", err)
	}
	if len(got) != 0 {
		t.Errorf("ListByUsers() leaked %v from another account", got)
	}
}
