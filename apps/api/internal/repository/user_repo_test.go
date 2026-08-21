//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These tests exercise the real SQL behind user administration. The account boundary is the
// point: under row level security a foreign account's user must be indistinguishable from one
// that does not exist.

func newUser(email, role string) domain.NewUser {
	return domain.NewUser{Name: "Nuevo", Email: email, Role: domain.UserRole(role)}
}

// An admin of one account must not reach another account's users at all — not to read them,
// not to rename them, not to disable them. Row level security is what makes every one of
// these come back empty rather than forbidden.
func TestUserRepository_AnotherAccountsUserIsInvisible(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralón A")
	accountB := seedAccount(t, db, "Corralón B")
	userB := seedUser(t, db, accountB, "SELLER")

	tenantA := domain.Tenant{AccountID: accountA}

	t.Run("get", func(t *testing.T) {
		err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
			_, getErr := repo.GetByID(ctx, q, accountA, userB)
			return getErr
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetByID(other account's user) = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("list", func(t *testing.T) {
		var users []domain.AppUser
		if err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
			var listErr error
			users, listErr = repo.List(ctx, q, accountA)
			return listErr
		}); err != nil {
			t.Fatalf("List() = %v, want no error", err)
		}
		for _, u := range users {
			if u.ID == userB {
				t.Fatal("List() leaked a user from another account")
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
			_, updErr := repo.Update(ctx, q, accountA, userB, domain.UserUpdate{
				Name: "Secuestrado", Email: "hijack@test.local", Role: domain.UserRoleAdmin,
			})
			return updErr
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Update(other account's user) = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("deactivate", func(t *testing.T) {
		err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
			return repo.Deactivate(ctx, q, accountA, userB)
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Deactivate(other account's user) = %v, want %v", err, domain.ErrNotFound)
		}
	})

	// The row must be untouched by all of the above.
	var name string
	var active bool
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT name, is_active FROM app_user WHERE id = $1`, userB).Scan(&name, &active); err != nil {
		t.Fatalf("read user B: %v", err)
	}
	if name != "Test" || !active {
		t.Errorf("user B is now name=%q active=%v; another account modified it", name, active)
	}
}

// An address identifies exactly one user across every account, because login resolves by
// email alone and could not otherwise pick a row deterministically.
func TestUserRepository_EmailUniquenessIsGlobal(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()

	accountA := seedAccount(t, db, "Corralón A")
	accountB := seedAccount(t, db, "Corralón B")
	// Unique per run: the address has to be shared inside this test and nowhere else, because
	// the index is global and `go test ./internal/...` runs this package beside the one that
	// exercises the same rule through the API.
	shared := "compras+" + uuid.NewString() + "@corralon.test"

	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM app_user WHERE account_id = ANY($1)`, []uuid.UUID{accountA, accountB})
	})

	created, err := createUser(t, db, accountA, repo, newUser(shared, "SELLER"))
	if err != nil {
		t.Fatalf("first create = %v, want no error", err)
	}
	if created.Email != shared {
		t.Errorf("Email = %q, want %q", created.Email, shared)
	}

	if _, err := createUser(t, db, accountA, repo, newUser(shared, "SELLER")); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("duplicate email in the same account = %v, want %v", err, domain.ErrConflict)
	}

	if _, err := createUser(t, db, accountB, repo, newUser(shared, "SELLER")); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("same email in another account = %v, want %v", err, domain.ErrConflict)
	}

	// Case is not a way around it: the index is on lower(email), so it holds even if a writer
	// forgets to normalize.
	if _, err := createUser(t, db, accountB, repo, newUser(strings.ToUpper(shared), "SELLER")); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("same email in a different case = %v, want %v", err, domain.ErrConflict)
	}
}

// A confirmation proves one address reachable. Carrying it across an address change would let
// an account be pointed at a mailbox nobody proved, while still reading as verified — which is
// exactly what AUTH_REQUIRE_VERIFIED_EMAIL is there to prevent.
func TestUserRepository_UpdateClearsVerificationOnlyWhenTheAddressChanges(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	userID := seedUser(t, db, accountID, "SELLER")
	tenant := domain.Tenant{AccountID: accountID}

	verifiedAt := func(t *testing.T) *time.Time {
		t.Helper()
		var at *time.Time
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT email_verified_at FROM app_user WHERE id = $1`, userID).Scan(&at); err != nil {
			t.Fatalf("read email_verified_at: %v", err)
		}
		return at
	}
	stamp := func(t *testing.T) {
		t.Helper()
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.MarkEmailVerified(ctx, q, accountID, userID)
		}); err != nil {
			t.Fatalf("MarkEmailVerified() = %v, want no error", err)
		}
	}
	update := func(t *testing.T, name, email string) {
		t.Helper()
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			_, updErr := repo.Update(ctx, q, accountID, userID, domain.UserUpdate{
				Name: name, Email: email, Role: domain.UserRoleSeller,
			})
			return updErr
		}); err != nil {
			t.Fatalf("Update() = %v, want no error", err)
		}
	}

	var current string
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT email FROM app_user WHERE id = $1`, userID).Scan(&current); err != nil {
		t.Fatalf("read email: %v", err)
	}

	t.Run("an unrelated edit keeps it", func(t *testing.T) {
		stamp(t)
		update(t, "Renombrado", current)
		if verifiedAt(t) == nil {
			t.Error("email_verified_at was cleared by an edit that left the address alone")
		}
	})

	// Only the case differs, and the unique index is on lower(email) — so this is the same
	// address, and dropping the confirmation here would be a needless re-verification.
	t.Run("a case-only change keeps it", func(t *testing.T) {
		stamp(t)
		update(t, "Renombrado", strings.ToUpper(current))
		if verifiedAt(t) == nil {
			t.Error("email_verified_at was cleared by a change of case alone")
		}
	})

	t.Run("a real address change drops it", func(t *testing.T) {
		stamp(t)
		update(t, "Renombrado", "otra+"+uuid.NewString()+"@corralon.test")
		if at := verifiedAt(t); at != nil {
			t.Errorf("email_verified_at = %v after the address changed, want null", at)
		}
	})
}

/*
 * The hash in the predicate is what keeps a password moved inside the bcrypt window from being
 * spent on an address change. Only the real SQL can be asked about it: a fake repository proves
 * the service's mapping and nothing about the clause.
 */
func TestUserRepository_UpdateEmailRefusesAHashThatMoved(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	userID := seedUser(t, db, accountID, "SELLER")
	tenant := domain.Tenant{AccountID: accountID}

	var storedHash, before string
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT password_hash, email FROM app_user WHERE id = $1`,
		userID).Scan(&storedHash, &before); err != nil {
		t.Fatalf("read the seeded user: %v", err)
	}

	changeTo := func(t *testing.T, email, hash string) error {
		t.Helper()
		return db.InTenantTx(ctx, tenant, func(q Querier) error {
			_, updErr := repo.UpdateEmail(ctx, q, accountID, userID, email, hash)
			return updErr
		})
	}
	currentEmail := func(t *testing.T) string {
		t.Helper()
		var email string
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT email FROM app_user WHERE id = $1`, userID).Scan(&email); err != nil {
			t.Fatalf("read email: %v", err)
		}
		return email
	}

	t.Run("a stale hash matches no row", func(t *testing.T) {
		err := changeTo(t, "otra+"+uuid.NewString()+"@corralon.test", storedHash+"-rotado")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("UpdateEmail with a stale hash = %v, want %v", err, domain.ErrNotFound)
		}
		if got := currentEmail(t); got != before {
			t.Errorf("the address moved to %q on a stale hash", got)
		}
	})

	t.Run("the current hash writes and drops the confirmation", func(t *testing.T) {
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			return repo.MarkEmailVerified(ctx, q, accountID, userID)
		}); err != nil {
			t.Fatalf("MarkEmailVerified() = %v, want no error", err)
		}

		after := "otra+" + uuid.NewString() + "@corralon.test"
		if err := changeTo(t, after, storedHash); err != nil {
			t.Fatalf("UpdateEmail() = %v, want no error", err)
		}
		if got := currentEmail(t); got != after {
			t.Errorf("email = %q, want %q", got, after)
		}
		var verifiedAt *time.Time
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT email_verified_at FROM app_user WHERE id = $1`, userID).Scan(&verifiedAt); err != nil {
			t.Fatalf("read email_verified_at: %v", err)
		}
		if verifiedAt != nil {
			t.Errorf("email_verified_at = %v after the address changed, want null", verifiedAt)
		}
	})

	// The global index is what login depends on, so it has to refuse here too.
	t.Run("an address another user holds is a conflict", func(t *testing.T) {
		otherAccount := seedAccount(t, db, "Corralón Vecino")
		otherUser := seedUser(t, db, otherAccount, "SELLER")
		var taken string
		if err := db.CrossAccount().QueryRow(ctx,
			`SELECT email FROM app_user WHERE id = $1`, otherUser).Scan(&taken); err != nil {
			t.Fatalf("read the other address: %v", err)
		}

		err := changeTo(t, taken, storedHash)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("UpdateEmail onto a taken address = %v, want %v", err, domain.ErrConflict)
		}
		if got := domain.CodeOf(err); got != domain.CodeEmailTaken {
			t.Errorf("code = %q, want %q", got, domain.CodeEmailTaken)
		}
	})
}

// Deactivating bumps nothing on its own — the service pairs it with the epoch bump. This
// asserts the two repository writes a deactivation is built from.
func TestUserRepository_DeactivateAndEpochBump(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	userID := seedUser(t, db, accountID, "SELLER")
	tenant := domain.Tenant{AccountID: accountID}

	var before, after int
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		user, getErr := repo.GetByID(ctx, q, accountID, userID)
		if getErr != nil {
			return getErr
		}
		before = user.SessionEpoch

		if deErr := repo.Deactivate(ctx, q, accountID, userID); deErr != nil {
			return deErr
		}
		epoch, bumpErr := repo.BumpSessionEpoch(ctx, q, accountID, userID)
		after = epoch
		return bumpErr
	}); err != nil {
		t.Fatalf("deactivate = %v, want no error", err)
	}

	if after != before+1 {
		t.Errorf("session epoch = %d, want %d", after, before+1)
	}

	var active bool
	var epoch int
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT is_active, session_epoch FROM app_user WHERE id = $1`, userID).Scan(&active, &epoch); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if active {
		t.Error("is_active = true after Deactivate")
	}
	if epoch != before+1 {
		t.Errorf("persisted epoch = %d, want %d", epoch, before+1)
	}
}

// A nil IsActive must leave the flag alone, or an edit form would silently revive a
// deactivated user.
func TestUserRepository_UpdateLeavesIsActiveAloneWhenNil(t *testing.T) {
	db := testDB(t)
	repo := NewUserRepository()
	ctx := context.Background()

	accountID := seedAccount(t, db, "Corralón")
	userID := seedUser(t, db, accountID, "SELLER")
	tenant := domain.Tenant{AccountID: accountID}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.Deactivate(ctx, q, accountID, userID)
	}); err != nil {
		t.Fatalf("Deactivate() = %v, want no error", err)
	}

	var updated *domain.AppUser
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		var updErr error
		updated, updErr = repo.Update(ctx, q, accountID, userID, domain.UserUpdate{
			Name: "Renombrado", Email: "renombrado@test.local", Role: domain.UserRoleSeller,
		})
		return updErr
	}); err != nil {
		t.Fatalf("Update() = %v, want no error", err)
	}

	if updated.IsActive {
		t.Error("Update with a nil IsActive revived a deactivated user")
	}
	if updated.Name != "Renombrado" {
		t.Errorf("Name = %q, want %q", updated.Name, "Renombrado")
	}
}

func createUser(t *testing.T, db *DB, accountID uuid.UUID, repo *UserRepository, in domain.NewUser) (*domain.AppUser, error) {
	t.Helper()
	var created *domain.AppUser
	err := db.InTenantTx(context.Background(), domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var createErr error
		created, createErr = repo.Create(context.Background(), q, accountID, in, "hashed")
		return createErr
	})
	return created, err
}
