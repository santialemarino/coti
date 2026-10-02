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

// seedTokenAt writes a token straight through the owner pool, so a test can fix its type,
// creation time and state.
func seedTokenAt(
	t *testing.T, db *DB, accountID, userID uuid.UUID, tokenType domain.AuthTokenType,
	hash string, createdAt time.Time, consumed bool,
) {
	t.Helper()
	var consumedAt *time.Time
	if consumed {
		consumedAt = &createdAt
	}
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO auth_token (account_id, user_id, type, token_hash, expires_at, consumed_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		accountID, userID, tokenType, hash, createdAt.Add(time.Hour), consumedAt, createdAt); err != nil {
		t.Fatalf("seed %s token: %v", tokenType, err)
	}
}

func TestAuthTokenRepository_InvalidateAllForUserRetiresEveryType(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón Invalidate All")
	user := seedUser(t, db, account, "SELLER")
	other := seedUser(t, db, account, "SELLER")
	now := time.Now()
	hashes := map[domain.AuthTokenType]string{
		domain.AuthTokenTypePasswordReset:     hashOf("allreset"),
		domain.AuthTokenTypeEmailVerification: hashOf("allverify"),
		domain.AuthTokenTypeInvite:            hashOf("allinvite"),
	}
	for tokenType, hash := range hashes {
		seedTokenAt(t, db, account, user, tokenType, hash, now, false)
	}
	otherHash := hashOf("allother")
	seedTokenAt(t, db, account, other, domain.AuthTokenTypeInvite, otherHash, now, false)
	tenant := domain.Tenant{AccountID: account}

	if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
		return repo.InvalidateAllForUser(ctx, q, account, user)
	}); err != nil {
		t.Fatalf("InvalidateAllForUser() = %v, want no error", err)
	}

	for tokenType, hash := range hashes {
		token, err := getToken(t, db, tenant, hash)
		if err != nil {
			t.Fatalf("read the %s token: %v", tokenType, err)
		}
		if token.IsUsable(now) {
			t.Errorf("the %s link is still usable", tokenType)
		}
	}
	untouched, err := getToken(t, db, tenant, otherHash)
	if err != nil {
		t.Fatalf("read the other user's token: %v", err)
	}
	if !untouched.IsUsable(now) {
		t.Fatal("InvalidateAllForUser retired a link belonging to a different user")
	}
}

func TestAuthTokenRepository_LatestInvitesByUsersReadsTheNewestInviteOnly(t *testing.T) {
	db := testDB(t)
	repo := NewAuthTokenRepository()
	ctx := context.Background()

	account := seedAccount(t, db, "Corralón Latest Invite")
	invited := seedUser(t, db, account, "SELLER")
	neverInvited := seedUser(t, db, account, "SELLER")
	base := time.Now().Add(-time.Hour)
	// The older invite was retired by a resend; the newer one is the one that counts. A recovery
	// link newer still must not be read as an invite.
	seedTokenAt(t, db, account, invited, domain.AuthTokenTypeInvite, hashOf("oldinvite"), base, true)
	seedTokenAt(t, db, account, invited, domain.AuthTokenTypeInvite, hashOf("newinvite"),
		base.Add(time.Minute), false)
	seedTokenAt(t, db, account, invited, domain.AuthTokenTypePasswordReset, hashOf("newerreset"),
		base.Add(2*time.Minute), true)
	seedTokenAt(t, db, account, neverInvited, domain.AuthTokenTypePasswordReset, hashOf("onlyreset"),
		base, false)

	var latest map[uuid.UUID]domain.AuthToken
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
		var err error
		latest, err = repo.LatestInvitesByUsers(ctx, q, account, []uuid.UUID{invited, neverInvited})
		return err
	}); err != nil {
		t.Fatalf("LatestInvitesByUsers() = %v, want no error", err)
	}

	got, ok := latest[invited]
	if !ok {
		t.Fatal("the invited user has no latest invite")
	}
	if got.TokenHash != hashOf("newinvite") {
		t.Errorf("latest invite = %s, want the newest invite", got.TokenHash)
	}
	if got.ConsumedAt != nil {
		t.Error("the latest invite reads as consumed, want the outstanding one")
	}
	if _, ok := latest[neverInvited]; ok {
		t.Error("a user with only a recovery link has an invite entry")
	}
}
