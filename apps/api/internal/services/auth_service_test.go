package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The auth flow's security properties are all in this file, so they are tested against
// in-memory fakes rather than a database: what matters is the decision, not the SQL.

var (
	testAccountID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testUserID    = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	fixedNow      = time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
)

const testPassword = "correct-horse-battery"

func testAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		JWTSecret:          "0123456789abcdef0123456789abcdef",
		AccessTTL:          15 * time.Minute,
		RefreshTTL:         12 * time.Hour,
		RefreshRememberTTL: 30 * 24 * time.Hour,
		RefreshReuseGrace:  30 * time.Second,
		MaxFailedAttempts:  5,
		LockoutDuration:    15 * time.Minute,
	}
}

// fakeDB satisfies tenantScoper. It records the accounts it was scoped to so a test can
// assert the tenant boundary was respected.
type fakeDB struct {
	scopes []uuid.UUID
	txErr  error
}

func (f *fakeDB) InTenantTx(_ context.Context, tenant domain.Tenant, fn func(repository.Querier) error) error {
	if tenant.AccountID == uuid.Nil {
		return domain.ErrNoTenantContext
	}
	f.scopes = append(f.scopes, tenant.AccountID)
	if f.txErr != nil {
		return f.txErr
	}
	return fn(nil)
}

func (f *fakeDB) CrossAccount() repository.Querier { return nil }

type fakeUsers struct {
	user            *domain.AppUser
	getByEmailErr   error
	failedAttempts  int
	successRecorded bool
	epochBumped     bool
}

func (f *fakeUsers) GetByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.AppUser, error) {
	if f.user == nil {
		return nil, domain.ErrNotFound
	}
	copied := *f.user
	return &copied, nil
}

func (f *fakeUsers) GetByEmailCrossAccount(context.Context, repository.Querier, string) (*domain.AppUser, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	copied := *f.user
	return &copied, nil
}

func (f *fakeUsers) RegisterFailedAttempt(context.Context, repository.Querier, uuid.UUID, uuid.UUID, int, time.Duration) (int, error) {
	f.failedAttempts++
	return f.failedAttempts, nil
}

func (f *fakeUsers) RegisterSuccessfulLogin(context.Context, repository.Querier, uuid.UUID, uuid.UUID) error {
	f.successRecorded = true
	return nil
}

func (f *fakeUsers) BumpSessionEpoch(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (int, error) {
	f.epochBumped = true
	return f.user.SessionEpoch + 1, nil
}

type fakeTokens struct {
	stored        *domain.RefreshToken
	created       []domain.RefreshToken
	consumedIDs   []uuid.UUID
	revokedFamily []uuid.UUID
	getByHashErr  error
}

func (f *fakeTokens) GetByHashCrossAccount(context.Context, repository.Querier, string) (*domain.RefreshToken, error) {
	if f.getByHashErr != nil {
		return nil, f.getByHashErr
	}
	if f.stored == nil {
		return nil, domain.ErrNotFound
	}
	copied := *f.stored
	return &copied, nil
}

func (f *fakeTokens) Create(_ context.Context, _ repository.Querier, t domain.RefreshToken) error {
	f.created = append(f.created, t)
	return nil
}

func (f *fakeTokens) Consume(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID) error {
	f.consumedIDs = append(f.consumedIDs, id)
	return nil
}

func (f *fakeTokens) RevokeFamily(_ context.Context, _ repository.Querier, _ uuid.UUID, familyID uuid.UUID) error {
	f.revokedFamily = append(f.revokedFamily, familyID)
	return nil
}

type harness struct {
	svc    *AuthService
	db     *fakeDB
	users  *fakeUsers
	tokens *fakeTokens
}

func newHarness(t *testing.T, user *domain.AppUser) *harness {
	t.Helper()
	cfg := testAuthConfig()
	db := &fakeDB{}
	users := &fakeUsers{user: user}
	tokens := &fakeTokens{}
	now := func() time.Time { return fixedNow }

	svc := NewAuthService(db, users, tokens, NewTokenService(cfg.JWTSecret, cfg.AccessTTL, now), cfg, now)
	return &harness{svc: svc, db: db, users: users, tokens: tokens}
}

func activeUser(t *testing.T) *domain.AppUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return &domain.AppUser{
		ID: testUserID, AccountID: testAccountID, Email: "vendedor@corralon.test",
		PasswordHash: string(hash), Role: domain.UserRoleSeller, IsActive: true, SessionEpoch: 3,
	}
}

func TestLogin_Succeeds(t *testing.T) {
	h := newHarness(t, activeUser(t))

	pair, err := h.svc.Login(context.Background(), domain.Credentials{
		Email: "vendedor@corralon.test", Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Login() = %v, want no error", err)
	}

	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Login() returned an empty token")
	}
	if pair.AccessExpiresAt != fixedNow.Add(15*time.Minute) {
		t.Errorf("AccessExpiresAt = %v, want %v", pair.AccessExpiresAt, fixedNow.Add(15*time.Minute))
	}
	if !h.users.successRecorded {
		t.Error("a successful login must clear the failure counter")
	}
	if len(h.tokens.created) != 1 {
		t.Fatalf("refresh tokens created = %d, want 1", len(h.tokens.created))
	}
	// The raw value must never be what lands in the database.
	if h.tokens.created[0].TokenHash == pair.RefreshToken {
		t.Error("the stored value equals the raw token; only its hash may be persisted")
	}
	if h.tokens.created[0].ExpiresAt != fixedNow.Add(12*time.Hour) {
		t.Errorf("refresh expiry = %v, want the non-remember TTL", h.tokens.created[0].ExpiresAt)
	}
}

func TestLogin_RememberMeUsesTheLongerTTL(t *testing.T) {
	h := newHarness(t, activeUser(t))

	if _, err := h.svc.Login(context.Background(), domain.Credentials{
		Email: "vendedor@corralon.test", Password: testPassword, RememberMe: true,
	}); err != nil {
		t.Fatalf("Login() = %v, want no error", err)
	}
	if got, want := h.tokens.created[0].ExpiresAt, fixedNow.Add(30*24*time.Hour); got != want {
		t.Errorf("refresh expiry = %v, want %v", got, want)
	}
}

// A bad password, an unknown email, and a disabled user must be indistinguishable to the
// caller, or the endpoint becomes an account-enumeration oracle.
func TestLogin_FailuresAreIndistinguishable(t *testing.T) {
	disabled := activeUser(t)
	disabled.IsActive = false

	cases := []struct {
		name  string
		setup func(*harness)
		creds domain.Credentials
	}{
		{
			name:  "unknown email",
			setup: func(h *harness) { h.users.getByEmailErr = domain.ErrNotFound },
			creds: domain.Credentials{Email: "nobody@corralon.test", Password: testPassword},
		},
		{
			name:  "wrong password",
			setup: func(*harness) {},
			creds: domain.Credentials{Email: "vendedor@corralon.test", Password: "wrong-password"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, activeUser(t))
			tc.setup(h)

			_, err := h.svc.Login(context.Background(), tc.creds)
			if !errors.Is(err, domain.ErrUnauthenticated) {
				t.Errorf("Login() = %v, want %v", err, domain.ErrUnauthenticated)
			}
		})
	}

	t.Run("disabled user", func(t *testing.T) {
		h := newHarness(t, disabled)
		_, err := h.svc.Login(context.Background(), domain.Credentials{
			Email: "vendedor@corralon.test", Password: testPassword,
		})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("Login() = %v, want %v", err, domain.ErrUnauthenticated)
		}
	})
}

func TestLogin_WrongPasswordCountsTheAttempt(t *testing.T) {
	h := newHarness(t, activeUser(t))

	_, _ = h.svc.Login(context.Background(), domain.Credentials{
		Email: "vendedor@corralon.test", Password: "wrong-password",
	})

	if h.users.failedAttempts != 1 {
		t.Errorf("failed attempts recorded = %d, want 1", h.users.failedAttempts)
	}
	// The counter update is a write on a tenant-scoped table, so it must be scoped.
	if len(h.db.scopes) != 1 || h.db.scopes[0] != testAccountID {
		t.Errorf("tenant scopes = %v, want one entry for %v", h.db.scopes, testAccountID)
	}
}

// A locked account reports the lockout rather than a generic failure: the client needs
// to tell "wrong password" from "stop retrying".
func TestLogin_LockedAccountIsReportedAndSkipsThePasswordCheck(t *testing.T) {
	user := activeUser(t)
	until := fixedNow.Add(5 * time.Minute)
	user.LockedUntil = &until
	h := newHarness(t, user)

	_, err := h.svc.Login(context.Background(), domain.Credentials{
		Email: "vendedor@corralon.test", Password: testPassword,
	})
	if !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("Login() = %v, want %v", err, domain.ErrLocked)
	}
	if len(h.tokens.created) != 0 {
		t.Error("a locked account must not receive tokens")
	}
}

func TestLogin_LockoutExpires(t *testing.T) {
	user := activeUser(t)
	past := fixedNow.Add(-time.Minute)
	user.LockedUntil = &past
	h := newHarness(t, user)

	if _, err := h.svc.Login(context.Background(), domain.Credentials{
		Email: "vendedor@corralon.test", Password: testPassword,
	}); err != nil {
		t.Errorf("Login() = %v, want no error once the lockout has passed", err)
	}
}

func storedToken(consumedAt *time.Time) *domain.RefreshToken {
	return &domain.RefreshToken{
		ID:         uuid.New(),
		AccountID:  testAccountID,
		UserID:     testUserID,
		FamilyID:   uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		TokenHash:  hashToken("raw-token"),
		ExpiresAt:  fixedNow.Add(time.Hour),
		ConsumedAt: consumedAt,
		CreatedAt:  fixedNow.Add(-time.Minute),
	}
}

func TestRefresh_RotatesWithinTheSameFamily(t *testing.T) {
	h := newHarness(t, activeUser(t))
	h.tokens.stored = storedToken(nil)

	pair, err := h.svc.Refresh(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("Refresh() = %v, want no error", err)
	}

	if len(h.tokens.consumedIDs) != 1 {
		t.Errorf("consumed tokens = %d, want 1", len(h.tokens.consumedIDs))
	}
	if len(h.tokens.created) != 1 {
		t.Fatalf("tokens created = %d, want 1", len(h.tokens.created))
	}
	if h.tokens.created[0].FamilyID != h.tokens.stored.FamilyID {
		t.Error("the successor must stay in the same family")
	}
	if pair.RefreshToken == "raw-token" {
		t.Error("Refresh() returned the same raw token; it must rotate")
	}
	if len(h.tokens.revokedFamily) != 0 {
		t.Error("a legitimate rotation must not revoke the family")
	}
}

// Two tabs refreshing at once replay a just-consumed token. That is a race, not theft,
// so it yields a fresh rotation instead of destroying the session.
func TestRefresh_ReplayInsideTheGraceWindowIsBenign(t *testing.T) {
	h := newHarness(t, activeUser(t))
	consumed := fixedNow.Add(-10 * time.Second)
	h.tokens.stored = storedToken(&consumed)

	if _, err := h.svc.Refresh(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Refresh() = %v, want no error inside the grace window", err)
	}
	if len(h.tokens.revokedFamily) != 0 {
		t.Error("a replay inside the grace window must not revoke the family")
	}
	if len(h.tokens.created) != 1 {
		t.Error("a replay inside the grace window must still hand out a rotation")
	}
}

// Past the window, the same replay is treated as a stolen token and takes the whole
// family down, logging attacker and victim out together.
func TestRefresh_ReplayPastTheGraceWindowRevokesTheFamily(t *testing.T) {
	h := newHarness(t, activeUser(t))
	consumed := fixedNow.Add(-5 * time.Minute)
	h.tokens.stored = storedToken(&consumed)

	_, err := h.svc.Refresh(context.Background(), "raw-token")
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Refresh() = %v, want %v", err, domain.ErrUnauthenticated)
	}
	if len(h.tokens.revokedFamily) != 1 || h.tokens.revokedFamily[0] != h.tokens.stored.FamilyID {
		t.Errorf("revoked families = %v, want the presented token's family", h.tokens.revokedFamily)
	}
	if len(h.tokens.created) != 0 {
		t.Error("a suspected theft must not hand out a new token")
	}
}

func TestRefresh_Rejects(t *testing.T) {
	revoked := fixedNow.Add(-time.Minute)

	cases := []struct {
		name   string
		mutate func(*domain.RefreshToken)
	}{
		{"revoked", func(t *domain.RefreshToken) { t.RevokedAt = &revoked }},
		{"expired", func(t *domain.RefreshToken) { t.ExpiresAt = fixedNow.Add(-time.Second) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, activeUser(t))
			h.tokens.stored = storedToken(nil)
			tc.mutate(h.tokens.stored)

			_, err := h.svc.Refresh(context.Background(), "raw-token")
			if !errors.Is(err, domain.ErrUnauthenticated) {
				t.Errorf("Refresh() = %v, want %v", err, domain.ErrUnauthenticated)
			}
		})
	}

	t.Run("unknown", func(t *testing.T) {
		h := newHarness(t, activeUser(t))
		_, err := h.svc.Refresh(context.Background(), "never-issued")
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("Refresh() = %v, want %v", err, domain.ErrUnauthenticated)
		}
	})
}

// A rotation must not silently downgrade a "remember me" session to the short TTL.
func TestRefresh_PreservesTheRememberMeLifetime(t *testing.T) {
	h := newHarness(t, activeUser(t))
	h.tokens.stored = storedToken(nil)
	h.tokens.stored.CreatedAt = fixedNow.Add(-time.Hour)
	h.tokens.stored.ExpiresAt = h.tokens.stored.CreatedAt.Add(30 * 24 * time.Hour)

	if _, err := h.svc.Refresh(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Refresh() = %v, want no error", err)
	}
	if got, want := h.tokens.created[0].ExpiresAt, fixedNow.Add(30*24*time.Hour); got != want {
		t.Errorf("rotated expiry = %v, want the remember-me TTL %v", got, want)
	}
}

func TestLogout_BumpsTheEpochAndRevokesTheFamily(t *testing.T) {
	h := newHarness(t, activeUser(t))
	h.tokens.stored = storedToken(nil)
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID, Role: domain.UserRoleSeller}

	if err := h.svc.Logout(context.Background(), tenant, "raw-token"); err != nil {
		t.Fatalf("Logout() = %v, want no error", err)
	}
	if !h.users.epochBumped {
		t.Error("logout must bump the session epoch to invalidate outstanding access tokens")
	}
	if len(h.tokens.revokedFamily) != 1 {
		t.Errorf("revoked families = %d, want 1", len(h.tokens.revokedFamily))
	}
}

// Losing the refresh token must not trap a session open.
func TestLogout_WorksWithoutARefreshToken(t *testing.T) {
	h := newHarness(t, activeUser(t))
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	if err := h.svc.Logout(context.Background(), tenant, ""); err != nil {
		t.Fatalf("Logout() = %v, want no error", err)
	}
	if !h.users.epochBumped {
		t.Error("logout without a refresh token must still bump the session epoch")
	}
}

// A refresh token belonging to somebody else must not let the caller revoke that
// person's family.
func TestLogout_IgnoresAForeignRefreshToken(t *testing.T) {
	h := newHarness(t, activeUser(t))
	h.tokens.stored = storedToken(nil)
	h.tokens.stored.UserID = uuid.New()
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	if err := h.svc.Logout(context.Background(), tenant, "raw-token"); err != nil {
		t.Fatalf("Logout() = %v, want no error", err)
	}
	if len(h.tokens.revokedFamily) != 0 {
		t.Error("a foreign refresh family must not be revoked")
	}
	if !h.users.epochBumped {
		t.Error("the caller's own session must still end")
	}
}

func TestVerifySession(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*domain.AppUser)
		epoch   int
		wantErr error
	}{
		{"matching epoch", func(*domain.AppUser) {}, 3, nil},
		{"stale epoch after logout", func(*domain.AppUser) {}, 2, domain.ErrUnauthenticated},
		{"deactivated user", func(u *domain.AppUser) { u.IsActive = false }, 3, domain.ErrUnauthenticated},
		{"locked user", func(u *domain.AppUser) {
			until := fixedNow.Add(time.Minute)
			u.LockedUntil = &until
		}, 3, domain.ErrLocked},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := activeUser(t)
			tc.mutate(user)
			h := newHarness(t, user)

			err := h.svc.VerifySession(context.Background(), testAccountID, testUserID, tc.epoch)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("VerifySession() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifySession_UnknownUser(t *testing.T) {
	h := newHarness(t, nil)

	err := h.svc.VerifySession(context.Background(), testAccountID, testUserID, 1)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("VerifySession() = %v, want %v", err, domain.ErrUnauthenticated)
	}
}
