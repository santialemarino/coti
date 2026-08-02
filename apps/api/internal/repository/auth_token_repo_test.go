//go:build integration

package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Single use and the account boundary are both properties of the UPDATE's WHERE clause, so
// neither can be checked against a fake.

func newResetToken(accountID, userID uuid.UUID, hash string, expiresAt time.Time) domain.AuthToken {
	return domain.AuthToken{
		AccountID: accountID,
		UserID:    userID,
		Type:      domain.AuthTokenTypePasswordReset,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
}

// hashOf pads a label out to the CHAR(64) the column stores, so a test can name its tokens.
func hashOf(label string) string {
	return label + strings.Repeat("0", 64-len(label))
}

func seedResetToken(t *testing.T, db *DB, accountID, userID uuid.UUID, hash string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	repo := NewAuthTokenRepository()
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		return repo.Create(ctx, q, newResetToken(accountID, userID, hash, time.Now().Add(time.Hour)))
	}); err != nil {
		t.Fatalf("seed auth token: %v", err)
	}

	var id uuid.UUID
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT id FROM auth_token WHERE token_hash = $1`, hash).Scan(&id); err != nil {
		t.Fatalf("read seeded auth token: %v", err)
	}
	return id
}

func TestAuthTokenRepository_ConsumeIsSingleUse(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón Single Use")
	user := seedUser(t, db, account, "ADMIN")
	tokenID := seedResetToken(t, db, account, user, hashOf("singleuse"))
	tenant := domain.Tenant{AccountID: account}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.Consume(ctx, q, account, tokenID)
	}); err != nil {
		t.Fatalf("first Consume() = %v, want no error", err)
	}

	err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.Consume(ctx, q, account, tokenID)
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second Consume() = %v, want %v: the link would work twice", err, domain.ErrConflict)
	}
}

// Two redemptions of one link racing each other: the predicate on consumed_at is the only
// thing serializing them, and exactly one has to win.
func TestAuthTokenRepository_ConcurrentConsumeLetsOneWinner(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón Race")
	user := seedUser(t, db, account, "ADMIN")
	tokenID := seedResetToken(t, db, account, user, hashOf("race"))
	tenant := domain.Tenant{AccountID: account}

	var wg sync.WaitGroup
	results := make([]error, 2)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = db.InTenantTx(ctx, tenant, func(q Querier) error {
				return repo.Consume(ctx, q, account, tokenID)
			})
		}(i)
	}
	close(start)
	wg.Wait()

	var won int
	for _, err := range results {
		switch {
		case err == nil:
			won++
		case errors.Is(err, domain.ErrConflict):
		default:
			t.Fatalf("Consume() = %v, want nil or %v", err, domain.ErrConflict)
		}
	}
	if won != 1 {
		t.Fatalf("%d of 2 concurrent redemptions succeeded, want exactly 1", won)
	}
}

func TestAuthTokenRepository_InvalidateActiveRetiresTheOutstandingLinks(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón Invalidate")
	user := seedUser(t, db, account, "ADMIN")
	other := seedUser(t, db, account, "SELLER")
	firstHash := hashOf("first")
	seedResetToken(t, db, account, user, firstHash)
	otherHash := hashOf("other")
	seedResetToken(t, db, account, other, otherHash)
	tenant := domain.Tenant{AccountID: account}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.InvalidateActive(ctx, q, account, user, domain.AuthTokenTypePasswordReset)
	}); err != nil {
		t.Fatalf("InvalidateActive() = %v, want no error", err)
	}

	first, err := getToken(t, db, tenant, firstHash)
	if err != nil {
		t.Fatalf("read the retired token: %v", err)
	}
	if first.IsUsable(time.Now()) {
		t.Fatal("the previous link is still usable after a new one was requested")
	}

	// Another user's outstanding link is not collateral damage.
	untouched, err := getToken(t, db, tenant, otherHash)
	if err != nil {
		t.Fatalf("read the other user's token: %v", err)
	}
	if !untouched.IsUsable(time.Now()) {
		t.Fatal("InvalidateActive retired a link belonging to a different user")
	}
}

func TestAuthTokenRepository_AnotherAccountsTokenIsInvisible(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	accountA := seedAccount(t, db, "Corralón A tokens")
	accountB := seedAccount(t, db, "Corralón B tokens")
	userB := seedUser(t, db, accountB, "ADMIN")
	hashB := hashOf("accountb")
	tokenB := seedResetToken(t, db, accountB, userB, hashB)
	tenantA := domain.Tenant{AccountID: accountA}

	err := db.InTenantTx(ctx, tenantA, func(q Querier) error {
		_, getErr := repo.GetByHashCrossAccount(ctx, q, hashB)
		return getErr
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("reading another account's token inside a tenant scope = %v, want %v",
			err, domain.ErrNotFound)
	}

	err = db.InTenantTx(ctx, tenantA, func(q Querier) error {
		return repo.Consume(ctx, q, accountA, tokenB)
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Consume(other account's token) = %v, want %v", err, domain.ErrConflict)
	}

	// The lookup the reset flow actually uses runs on the owner pool, because the bearer
	// presents a link and nothing else. It has to find the row.
	found, err := repo.GetByHashCrossAccount(ctx, db.CrossAccount(), hashB)
	if err != nil {
		t.Fatalf("GetByHashCrossAccount on the owner pool = %v, want the token", err)
	}
	if found.AccountID != accountB {
		t.Fatalf("token resolved to account %v, want %v", found.AccountID, accountB)
	}
	if found.ConsumedAt != nil {
		t.Fatal("the foreign account's consume attempt redeemed the token")
	}
}

func getToken(t *testing.T, db *DB, tenant domain.Tenant, hash string) (*domain.AuthToken, error) {
	t.Helper()
	repo := NewAuthTokenRepository()
	var token *domain.AuthToken
	err := db.InTenantTx(context.Background(), tenant, func(q Querier) error {
		var getErr error
		token, getErr = repo.GetByHashCrossAccount(context.Background(), q, hash)
		return getErr
	})
	return token, err
}
