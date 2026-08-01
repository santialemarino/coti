//go:build integration

package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These tests prove the row level security wiring end to end from Go. Asserting it in psql
// is not enough: the risk lives in how the pool and the GUC interact.

func testDB(t *testing.T) *DB {
	t.Helper()

	appURL := os.Getenv("TEST_DATABASE_URL")
	adminURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_DATABASE_ADMIN_URL are required for integration tests")
	}

	db, err := NewDB(context.Background(), config.DatabaseConfig{
		URL:             appURL,
		AdminURL:        adminURL,
		MaxConns:        4,
		MinConns:        1,
		MaxConnLifetime: time.Minute,
		MaxConnIdleTime: time.Minute,
		ConnectTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewDB() = %v, want no error", err)
	}
	t.Cleanup(db.Close)
	return db
}

// seedAccount inserts an account with one branch through the owner pool and removes
// both when the test ends.
func seedAccount(t *testing.T, db *DB, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	accountID := uuid.New()

	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO account (id, name) VALUES ($1, $2)`, accountID, name); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (account_id, name) VALUES ($1, $2)`, accountID, name+" Centro"); err != nil {
		t.Fatalf("seed branch: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM branch WHERE account_id = $1`, accountID)
		_, _ = db.CrossAccount().Exec(cleanupCtx, `DELETE FROM account WHERE id = $1`, accountID)
	})
	return accountID
}

func countBranches(t *testing.T, q Querier) int {
	t.Helper()
	var n int
	if err := q.QueryRow(context.Background(), `SELECT count(*) FROM branch`).Scan(&n); err != nil {
		t.Fatalf("count branches: %v", err)
	}
	return n
}

func TestInTenantTx_SeesOnlyItsOwnAccount(t *testing.T) {
	db := testDB(t)
	accountA := seedAccount(t, db, "Corralon A")
	seedAccount(t, db, "Corralon B")

	var branches int
	err := db.InTenantTx(context.Background(), domain.Tenant{AccountID: accountA}, func(q Querier) error {
		branches = countBranches(t, q)
		return nil
	})
	if err != nil {
		t.Fatalf("InTenantTx() = %v, want no error", err)
	}
	if branches != 1 {
		t.Errorf("branches visible to account A = %d, want 1 (its own)", branches)
	}
}

// The GUC is transaction-scoped, so a connection returned to the pool must not carry
// the previous request's account into the next one.
func TestInTenantTx_ScopeDoesNotLeakBetweenTransactions(t *testing.T) {
	db := testDB(t)
	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")
	ctx := context.Background()

	var seenByA, seenByB uuid.UUID
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
		return q.QueryRow(ctx, `SELECT id FROM account`).Scan(&seenByA)
	}); err != nil {
		t.Fatalf("InTenantTx(A) = %v, want no error", err)
	}
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountB}, func(q Querier) error {
		return q.QueryRow(ctx, `SELECT id FROM account`).Scan(&seenByB)
	}); err != nil {
		t.Fatalf("InTenantTx(B) = %v, want no error", err)
	}

	if seenByA != accountA {
		t.Errorf("account visible to A = %v, want %v", seenByA, accountA)
	}
	if seenByB != accountB {
		t.Errorf("account visible to B = %v, want %v", seenByB, accountB)
	}
}

// A write naming another account must be rejected by the policy's WITH CHECK, not
// silently accepted.
func TestInTenantTx_CrossAccountWriteIsRejected(t *testing.T) {
	db := testDB(t)
	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")

	err := db.InTenantTx(context.Background(), domain.Tenant{AccountID: accountA}, func(q Querier) error {
		_, execErr := q.Exec(context.Background(),
			`INSERT INTO tag (account_id, name) VALUES ($1, $2)`, accountB, "stolen")
		return execErr
	})
	if err == nil {
		t.Fatal("InTenantTx() = nil error, want the row level security policy to reject the write")
	}
}

func TestInTenantTx_RequiresAnAccount(t *testing.T) {
	db := testDB(t)

	err := db.InTenantTx(context.Background(), domain.Tenant{}, func(Querier) error {
		t.Error("fn ran without tenant context; it must not be called")
		return nil
	})
	if !errors.Is(err, domain.ErrNoTenantContext) {
		t.Errorf("InTenantTx() = %v, want %v", err, domain.ErrNoTenantContext)
	}
}

// An error from fn must roll the transaction back, leaving nothing behind.
func TestInTenantTx_RollsBackOnError(t *testing.T) {
	db := testDB(t)
	accountA := seedAccount(t, db, "Corralon A")
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: accountA}
	wantErr := errors.New("business rule failed")

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		if _, execErr := q.Exec(ctx,
			`INSERT INTO tag (account_id, name) VALUES ($1, $2)`, accountA, "rolled-back"); execErr != nil {
			return execErr
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("InTenantTx() = %v, want %v", err, wantErr)
	}

	var tags int
	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return q.QueryRow(ctx, `SELECT count(*) FROM tag WHERE name = 'rolled-back'`).Scan(&tags)
	}); err != nil {
		t.Fatalf("InTenantTx() = %v, want no error", err)
	}
	if tags != 0 {
		t.Errorf("tags left after rollback = %d, want 0", tags)
	}
}

// The owner pool bypasses row level security on purpose, for the follow-up cron and
// the pre-auth lookups that cannot know the account yet.
func TestCrossAccount_SeesEveryAccount(t *testing.T) {
	db := testDB(t)
	seedAccount(t, db, "Corralon A")
	seedAccount(t, db, "Corralon B")

	if got := countBranches(t, db.CrossAccount()); got < 2 {
		t.Errorf("branches visible to the owner pool = %d, want at least 2", got)
	}
}

func TestBranchRepository_IsAccessibleBy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	accountA := seedAccount(t, db, "Corralon A")
	accountB := seedAccount(t, db, "Corralon B")

	branchA := branchOf(t, db, accountA)
	branchB := branchOf(t, db, accountB)
	seller := seedUser(t, db, accountA, "SELLER")

	repo := NewBranchRepository()

	cases := []struct {
		name     string
		branchID uuid.UUID
		isAdmin  bool
		link     bool
		want     bool
	}{
		{"seller assigned to the branch", branchA, false, true, true},
		{"seller not assigned to it", branchA, false, false, false},
		{"admin needs no assignment", branchA, true, false, true},
		// The one that matters: another account's branch is invisible even to an admin.
		{"branch of another account", branchB, true, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.link {
				linkUserBranch(t, db, accountA, seller, tc.branchID)
			} else {
				unlinkUserBranch(t, db, seller)
			}

			var got bool
			if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountA}, func(q Querier) error {
				var err error
				got, err = repo.IsAccessibleBy(ctx, q, accountA, seller, tc.branchID, tc.isAdmin)
				return err
			}); err != nil {
				t.Fatalf("InTenantTx() = %v, want no error", err)
			}
			if got != tc.want {
				t.Errorf("IsAccessibleBy() = %v, want %v", got, tc.want)
			}
		})
	}
}

func branchOf(t *testing.T, db *DB, accountID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM branch WHERE account_id = $1 LIMIT 1`, accountID).Scan(&id); err != nil {
		t.Fatalf("read branch: %v", err)
	}
	return id
}

func seedUser(t *testing.T, db *DB, accountID uuid.UUID, role string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO app_user (id, account_id, name, email, password_hash, role)
		 VALUES ($1, $2, 'Test', $3, 'x', $4)`,
		id, accountID, id.String()+"@test.local", role); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.CrossAccount().Exec(context.Background(), `DELETE FROM user_branch WHERE user_id = $1`, id)
		_, _ = db.CrossAccount().Exec(context.Background(), `DELETE FROM app_user WHERE id = $1`, id)
	})
	return id
}

func linkUserBranch(t *testing.T, db *DB, accountID, userID, branchID uuid.UUID) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, branch_id) DO NOTHING`, accountID, userID, branchID); err != nil {
		t.Fatalf("link user_branch: %v", err)
	}
}

func unlinkUserBranch(t *testing.T, db *DB, userID uuid.UUID) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`DELETE FROM user_branch WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("unlink user_branch: %v", err)
	}
}
