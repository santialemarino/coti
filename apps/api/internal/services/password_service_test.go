package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// The credential lifecycle's security properties are decisions, not SQL, so they are tested
// against in-memory fakes. What reaches the database is covered by the integration suites.

const (
	testCurrentPassword = "Current-horse-battery1"
	testNewPassword     = "Brand-new-staple-42"
	testRawResetToken   = "raw-reset-token"
)

type fakePasswordUsers struct {
	user            *domain.AppUser
	accountIsActive bool
	getByEmailErr   error
	writtenHash     string
	epochBumped     int
	// hashMovedTo replaces the stored hash after the read, standing in for a concurrent write.
	hashMovedTo string
}

func (f *fakePasswordUsers) GetAuthSubjectByID(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.AuthSubject, error) {
	if f.user == nil {
		return nil, domain.ErrNotFound
	}
	copied := *f.user
	if f.hashMovedTo != "" {
		f.user.PasswordHash = f.hashMovedTo
	}
	return &domain.AuthSubject{AppUser: copied, AccountIsActive: f.accountIsActive}, nil
}

func (f *fakePasswordUsers) GetAuthSubjectByEmailCrossAccount(context.Context, repository.Querier, string) (*domain.AuthSubject, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	copied := *f.user
	return &domain.AuthSubject{AppUser: copied, AccountIsActive: f.accountIsActive}, nil
}

func (f *fakePasswordUsers) UpdatePassword(_ context.Context, _ repository.Querier, _, _ uuid.UUID, hash string) error {
	f.writtenHash = hash
	return nil
}

func (f *fakePasswordUsers) UpdatePasswordIfCurrent(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, currentHash, hash string,
) error {
	if f.user == nil || f.user.PasswordHash != currentHash {
		return domain.ErrConflict
	}
	f.writtenHash = hash
	return nil
}

func (f *fakePasswordUsers) BumpSessionEpoch(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (int, error) {
	f.epochBumped++
	return f.user.SessionEpoch + f.epochBumped, nil
}

type fakeAuthTokens struct {
	stored       *domain.AuthToken
	created      []domain.AuthToken
	consumed     []uuid.UUID
	invalidated  []uuid.UUID
	consumeErr   error
	getByHashErr error
}

func (f *fakeAuthTokens) GetByHashCrossAccount(context.Context, repository.Querier, string) (*domain.AuthToken, error) {
	if f.getByHashErr != nil {
		return nil, f.getByHashErr
	}
	if f.stored == nil {
		return nil, domain.ErrNotFound
	}
	copied := *f.stored
	return &copied, nil
}

func (f *fakeAuthTokens) Create(_ context.Context, _ repository.Querier, t domain.AuthToken) error {
	f.created = append(f.created, t)
	return nil
}

func (f *fakeAuthTokens) Consume(_ context.Context, _ repository.Querier, _, id uuid.UUID) error {
	if f.consumeErr != nil {
		return f.consumeErr
	}
	f.consumed = append(f.consumed, id)
	return nil
}

func (f *fakeAuthTokens) InvalidateActive(
	_ context.Context, _ repository.Querier, _, userID uuid.UUID, _ domain.AuthTokenType,
) error {
	f.invalidated = append(f.invalidated, userID)
	return nil
}

type fakeSessionRevoker struct {
	revoked []uuid.UUID
}

func (f *fakeSessionRevoker) RevokeAllForUser(_ context.Context, _ repository.Querier, _, userID uuid.UUID) error {
	f.revoked = append(f.revoked, userID)
	return nil
}

type fakeMail struct {
	sent    []OutboundMail
	sendErr error
}

func (f *fakeMail) Send(_ context.Context, out OutboundMail) error {
	f.sent = append(f.sent, out)
	return f.sendErr
}

type fakeIssuer struct {
	issuedFor []domain.AppUser
}

func (f *fakeIssuer) IssueForUser(_ context.Context, user domain.AppUser) (*domain.TokenPair, error) {
	f.issuedFor = append(f.issuedFor, user)
	return &domain.TokenPair{AccessToken: "access", RefreshToken: "refresh"}, nil
}

type passwordFixture struct {
	svc      *PasswordService
	db       *fakeDB
	users    *fakePasswordUsers
	tokens   *fakeAuthTokens
	sessions *fakeSessionRevoker
	mail     *fakeMail
	issuer   *fakeIssuer
}

func newPasswordFixture(t *testing.T, user *domain.AppUser) *passwordFixture {
	t.Helper()
	f := &passwordFixture{
		db:       &fakeDB{},
		users:    &fakePasswordUsers{user: user, accountIsActive: true},
		tokens:   &fakeAuthTokens{},
		sessions: &fakeSessionRevoker{},
		mail:     &fakeMail{},
		issuer:   &fakeIssuer{},
	}
	cfg := testAuthConfig()
	cfg.PasswordResetTTL = time.Hour
	f.svc = NewPasswordService(f.db, f.users, f.tokens, f.sessions, f.mail, f.issuer,
		slog.New(slog.NewTextHandler(io.Discard, nil)), cfg,
		config.WebConfig{BackofficeURL: "https://backoffice.example"},
		func() time.Time { return fixedNow })
	f.svc.links.newSecret = func() (string, error) { return testRawResetToken, nil }
	return f
}

func passwordUser(t *testing.T) *domain.AppUser {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testCurrentPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	return &domain.AppUser{
		ID: testUserID, AccountID: testAccountID, Name: "Ana", Email: "ana@corralon.test",
		PasswordHash: string(hash), Role: domain.UserRoleAdmin, IsActive: true, SessionEpoch: 3,
	}
}

func TestPasswordService_ChangeOwn_WritesHashAndEndsOtherSessions(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))

	pair, err := f.svc.ChangeOwn(context.Background(), testTenant(), testCurrentPassword, testNewPassword)
	if err != nil {
		t.Fatalf("ChangeOwn: %v", err)
	}
	if pair == nil || pair.AccessToken == "" {
		t.Fatal("ChangeOwn returned no token pair; the caller would be logged out by their own change")
	}
	if bcrypt.CompareHashAndPassword([]byte(f.users.writtenHash), []byte(testNewPassword)) != nil {
		t.Fatal("the stored hash does not verify against the new password")
	}
	if f.users.epochBumped != 1 {
		t.Fatalf("session epoch bumped %d times, want 1", f.users.epochBumped)
	}
	if len(f.sessions.revoked) != 1 || f.sessions.revoked[0] != testUserID {
		t.Fatalf("refresh tokens revoked for %v, want exactly [%v]", f.sessions.revoked, testUserID)
	}
}

func TestPasswordService_ChangeOwn_IssuesThePairWithTheNewEpoch(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	f := newPasswordFixture(t, user)

	if _, err := f.svc.ChangeOwn(context.Background(), testTenant(), testCurrentPassword, testNewPassword); err != nil {
		t.Fatalf("ChangeOwn: %v", err)
	}
	if len(f.issuer.issuedFor) != 1 {
		t.Fatalf("issued %d pairs, want 1", len(f.issuer.issuedFor))
	}
	// The bump moved the epoch to 4; a pair signed with 3 is rejected on its first use.
	if got := f.issuer.issuedFor[0].SessionEpoch; got != user.SessionEpoch+1 {
		t.Fatalf("pair issued with session epoch %d, want %d", got, user.SessionEpoch+1)
	}
}

func TestPasswordService_ChangeOwn_WrongCurrentPasswordChangesNothing(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))

	_, err := f.svc.ChangeOwn(context.Background(), testTenant(), "not-the-current-one", testNewPassword)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("ChangeOwn with a wrong current password = %v, want ErrUnauthenticated", err)
	}
	if f.users.writtenHash != "" {
		t.Fatal("the password was written despite the wrong current password")
	}
	if f.users.epochBumped != 0 || len(f.sessions.revoked) != 0 {
		t.Fatal("sessions were ended despite the wrong current password")
	}
}

func TestPasswordService_ChangeOwn_RejectsAPasswordBelowTheFloor(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))

	_, err := f.svc.ChangeOwn(context.Background(), testTenant(), testCurrentPassword, "short")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("ChangeOwn with a 5-character password = %v, want ErrInvalidInput", err)
	}
	if f.users.writtenHash != "" {
		t.Fatal("a password below the configured floor was written")
	}
}

func TestPasswordService_Forgot_UnknownAddressAnswersTheSameAndSendsNothing(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))
	f.users.getByEmailErr = domain.ErrNotFound

	if err := f.svc.Forgot(context.Background(), "nobody@corralon.test"); err != nil {
		t.Fatalf("Forgot for an unknown address = %v, want nil: the answer must not differ", err)
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("a recovery mail was sent to an address that is not registered")
	}
	if len(f.tokens.created) != 0 {
		t.Fatal("a recovery token was minted for an address that is not registered")
	}
}

func TestPasswordService_Forgot_DeactivatedUserAnswersTheSameAndSendsNothing(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	user.IsActive = false
	f := newPasswordFixture(t, user)

	if err := f.svc.Forgot(context.Background(), user.Email); err != nil {
		t.Fatalf("Forgot for a deactivated user = %v, want nil", err)
	}
	if len(f.mail.sent) != 0 || len(f.tokens.created) != 0 {
		t.Fatal("a deactivated user was sent a way back in")
	}
}

func TestPasswordService_Forgot_MintsAHashedTokenAndMailsTheLink(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	f := newPasswordFixture(t, user)

	if err := f.svc.Forgot(context.Background(), "  ANA@Corralon.Test "); err != nil {
		t.Fatalf("Forgot: %v", err)
	}
	if len(f.tokens.created) != 1 {
		t.Fatalf("minted %d tokens, want 1", len(f.tokens.created))
	}
	created := f.tokens.created[0]
	if created.TokenHash == testRawResetToken {
		t.Fatal("the raw token was stored; only its hash may be")
	}
	if created.TokenHash != hashToken(testRawResetToken) {
		t.Fatal("the stored hash is not the SHA-256 of the token that was mailed")
	}
	if created.Type != domain.AuthTokenTypePasswordReset {
		t.Fatalf("token type = %q, want %q", created.Type, domain.AuthTokenTypePasswordReset)
	}
	if want := fixedNow.Add(time.Hour); !created.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", created.ExpiresAt, want)
	}
	if len(f.tokens.invalidated) != 1 {
		t.Fatal("the user's outstanding links were not retired before a new one was minted")
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(f.mail.sent))
	}
	sent := f.mail.sent[0]
	if sent.To != user.Email {
		t.Fatalf("mail addressed to %q, want %q", sent.To, user.Email)
	}
	if !strings.Contains(sent.ActionURL, "https://backoffice.example/reset-password?token=") ||
		!strings.Contains(sent.ActionURL, testRawResetToken) {
		t.Fatalf("action url = %q, want the backoffice reset route carrying the raw token", sent.ActionURL)
	}
}

func TestPasswordService_Forgot_UndeliverableMailStillAnswersOK(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))
	f.mail.sendErr = errors.New("provider rejected the message")

	if err := f.svc.Forgot(context.Background(), "ana@corralon.test"); err != nil {
		t.Fatalf("Forgot with a bouncing transport = %v, want nil: a bounce must not surface", err)
	}
}

func storedResetToken(expiresAt time.Time) *domain.AuthToken {
	return &domain.AuthToken{
		ID:        uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		AccountID: testAccountID,
		UserID:    testUserID,
		Type:      domain.AuthTokenTypePasswordReset,
		TokenHash: hashToken(testRawResetToken),
		ExpiresAt: expiresAt,
	}
}

func TestPasswordService_Reset_RedeemsTheLinkAndEndsEverySession(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))
	f.tokens.stored = storedResetToken(fixedNow.Add(time.Hour))

	if err := f.svc.Reset(context.Background(), testRawResetToken, testNewPassword); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(f.users.writtenHash), []byte(testNewPassword)) != nil {
		t.Fatal("the stored hash does not verify against the new password")
	}
	if len(f.tokens.consumed) != 1 {
		t.Fatal("the link was not redeemed, so it would work a second time")
	}
	if f.users.epochBumped != 1 {
		t.Fatalf("session epoch bumped %d times, want 1", f.users.epochBumped)
	}
	if len(f.sessions.revoked) != 1 {
		t.Fatal("refresh tokens survived the reset, so another device stays logged in")
	}
}

func TestPasswordService_Reset_RejectsATokenThatIsNotUsable(t *testing.T) {
	t.Parallel()
	consumed := fixedNow.Add(-time.Minute)
	cases := []struct {
		name  string
		token *domain.AuthToken
		err   error
	}{
		{"unknown", nil, nil},
		{"expired", storedResetToken(fixedNow.Add(-time.Second)), nil},
		{"already redeemed", func() *domain.AuthToken {
			t := storedResetToken(fixedNow.Add(time.Hour))
			t.ConsumedAt = &consumed
			return t
		}(), nil},
		{"wrong type", func() *domain.AuthToken {
			t := storedResetToken(fixedNow.Add(time.Hour))
			t.Type = domain.AuthTokenTypeEmailVerification
			return t
		}(), nil},
		{"lost the redemption race", storedResetToken(fixedNow.Add(time.Hour)), domain.ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPasswordFixture(t, passwordUser(t))
			f.tokens.stored = tc.token
			f.tokens.consumeErr = tc.err

			err := f.svc.Reset(context.Background(), testRawResetToken, testNewPassword)
			if !errors.Is(err, domain.ErrUnauthenticated) {
				t.Fatalf("Reset with a %s token = %v, want ErrUnauthenticated", tc.name, err)
			}
			if f.users.writtenHash != "" {
				t.Fatalf("the password was changed by a %s token", tc.name)
			}
		})
	}
}

func TestPasswordService_Reset_RejectsAPasswordBelowTheFloor(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))
	f.tokens.stored = storedResetToken(fixedNow.Add(time.Hour))

	err := f.svc.Reset(context.Background(), testRawResetToken, "short")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Reset with a 5-character password = %v, want ErrInvalidInput", err)
	}
	if f.users.writtenHash != "" {
		t.Fatal("a password below the configured floor was written")
	}
}

func TestPasswordService_Reset_RejectsADeactivatedUser(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	user.IsActive = false
	f := newPasswordFixture(t, user)
	f.tokens.stored = storedResetToken(fixedNow.Add(time.Hour))

	err := f.svc.Reset(context.Background(), testRawResetToken, testNewPassword)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Reset for a deactivated user = %v, want ErrUnauthenticated", err)
	}
	if f.users.writtenHash != "" {
		t.Fatal("a deactivated user got a working password")
	}
}

func TestPasswordService_AdminReset_ReadsTheTargetInsideTheCallersAccount(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, passwordUser(t))
	target := uuid.MustParse("44444444-4444-4444-8444-444444444444")

	if err := f.svc.AdminReset(context.Background(), testTenant(), target); err != nil {
		t.Fatalf("AdminReset: %v", err)
	}
	// The account scope on the lookup is the whole containment: a user of another account is
	// simply absent, so there is nothing to send a link to.
	for _, scope := range f.db.scopes {
		if scope != testAccountID {
			t.Fatalf("a transaction ran scoped to %v, want only %v", scope, testAccountID)
		}
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(f.mail.sent))
	}
}

func TestPasswordService_AdminReset_AUserOfAnotherAccountIsNotFound(t *testing.T) {
	t.Parallel()
	f := newPasswordFixture(t, nil)

	err := f.svc.AdminReset(context.Background(), testTenant(),
		uuid.MustParse("55555555-5555-4555-8555-555555555555"))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AdminReset on a user outside the account = %v, want ErrNotFound", err)
	}
	if len(f.mail.sent) != 0 || len(f.tokens.created) != 0 {
		t.Fatal("a link was issued for a user outside the administrator's account")
	}
}

func TestPasswordService_AdminReset_RefusesADeactivatedUser(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	user.IsActive = false
	f := newPasswordFixture(t, user)

	err := f.svc.AdminReset(context.Background(), testTenant(), testUserID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("AdminReset on a deactivated user = %v, want ErrInvalidInput", err)
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("a deactivated user was sent a way back in")
	}
}

// A recovery link redeemed while the change was hashing moves the stored credential. The
// change has to lose that race: overwriting would hand the account back to whoever knew the
// password the recovery was meant to retire.
func TestPasswordService_ChangeOwn_LosesToARecoveryThatLandedFirst(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	f := newPasswordFixture(t, user)

	// Stand in for the reset committing between the read and the write.
	f.users.hashMovedTo = "$2a$04$someoneelsereplacedthecredentialalready000000000000000"

	_, err := f.svc.ChangeOwn(context.Background(), testTenant(), testCurrentPassword, testNewPassword)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("ChangeOwn racing a recovery = %v, want ErrUnauthenticated", err)
	}
	if f.users.writtenHash != "" {
		t.Fatal("the change overwrote a credential a recovery had already replaced")
	}
	if f.users.epochBumped != 0 || len(f.sessions.revoked) != 0 {
		t.Fatal("the losing change still ended the sessions the recovery had just established")
	}
}
