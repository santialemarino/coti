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

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const testRawVerificationToken = "raw-verification-token"

type fakeVerificationUsers struct {
	user            *domain.AppUser
	accountIsActive bool
	getByEmailErr   error
	markedVerified  []uuid.UUID
	updateEmailErr  error
	// hashMovesTo replaces the stored hash after the read, standing in for a concurrent
	// credential write landing inside the bcrypt window.
	hashMovesTo string
}

func (f *fakeVerificationUsers) subject() *domain.AuthSubject {
	copied := *f.user
	return &domain.AuthSubject{AppUser: copied, AccountIsActive: f.accountIsActive}
}

func (f *fakeVerificationUsers) GetAuthSubjectByID(
	context.Context, repository.Querier, uuid.UUID, uuid.UUID,
) (*domain.AuthSubject, error) {
	if f.user == nil {
		return nil, domain.ErrNotFound
	}
	subject := f.subject()
	if f.hashMovesTo != "" {
		f.user.PasswordHash = f.hashMovesTo
	}
	return subject, nil
}

func (f *fakeVerificationUsers) GetAuthSubjectByEmailCrossAccount(
	context.Context, repository.Querier, string,
) (*domain.AuthSubject, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	return f.subject(), nil
}

/*
 * UpdateEmail stands in for the write: it drops the confirmation the way the SQL does, and it
 * matches no row when the hash it was handed is not the stored one, which is how the real
 * predicate reports a password that moved under it.
 */
func (f *fakeVerificationUsers) UpdateEmail(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, email, currentHash string,
) (*domain.AppUser, error) {
	if f.updateEmailErr != nil {
		return nil, f.updateEmailErr
	}
	if f.user.PasswordHash != currentHash {
		return nil, domain.ErrNotFound
	}
	f.user.Email = email
	f.user.EmailVerifiedAt = nil
	updated := *f.user
	return &updated, nil
}

func (f *fakeVerificationUsers) MarkEmailVerified(
	_ context.Context, _ repository.Querier, _, id uuid.UUID,
) error {
	f.markedVerified = append(f.markedVerified, id)
	return nil
}

type verificationFixture struct {
	svc    *VerificationService
	users  *fakeVerificationUsers
	tokens *fakeAuthTokens
	mail   *fakeMail
}

func newVerificationFixture(t *testing.T, user *domain.AppUser) *verificationFixture {
	t.Helper()
	f := &verificationFixture{
		users:  &fakeVerificationUsers{user: user, accountIsActive: true},
		tokens: &fakeAuthTokens{},
		mail:   &fakeMail{},
	}
	cfg := testAuthConfig()
	cfg.VerificationTTL = 48 * time.Hour
	f.svc = NewVerificationService(&fakeDB{}, f.users, f.tokens, f.mail,
		slog.New(slog.NewTextHandler(io.Discard, nil)), cfg,
		config.WebConfig{BackofficeURL: "https://backoffice.example"},
		func() time.Time { return fixedNow })
	f.svc.links.newSecret = func() (string, error) { return testRawVerificationToken, nil }
	return f
}

func storedVerificationToken(expiresAt time.Time) *domain.AuthToken {
	return &domain.AuthToken{
		ID:        uuid.MustParse("66666666-6666-4666-8666-666666666666"),
		AccountID: testAccountID,
		UserID:    testUserID,
		Type:      domain.AuthTokenTypeEmailVerification,
		TokenHash: hashToken(testRawVerificationToken),
		ExpiresAt: expiresAt,
	}
}

func TestVerificationService_Send_MailsAHashedTokenAndTheBackofficeLink(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	f := newVerificationFixture(t, user)

	if err := f.svc.Send(context.Background(), *user); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(f.tokens.created) != 1 {
		t.Fatalf("minted %d tokens, want 1", len(f.tokens.created))
	}
	created := f.tokens.created[0]
	if created.Type != domain.AuthTokenTypeEmailVerification {
		t.Fatalf("token type = %q, want %q", created.Type, domain.AuthTokenTypeEmailVerification)
	}
	if created.TokenHash != hashToken(testRawVerificationToken) {
		t.Fatal("the stored hash is not the SHA-256 of the token that was mailed")
	}
	if want := fixedNow.Add(48 * time.Hour); !created.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", created.ExpiresAt, want)
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(f.mail.sent))
	}
	// A recovery link and a verification link must not land on the same screen.
	if got := f.mail.sent[0].ActionURL; got != "https://backoffice.example/verify-email?token="+testRawVerificationToken {
		t.Fatalf("action url = %q, want the verify-email route", got)
	}
}

func TestVerificationService_Confirm_StampsTheUserOnce(t *testing.T) {
	t.Parallel()
	f := newVerificationFixture(t, passwordUser(t))
	f.tokens.stored = storedVerificationToken(fixedNow.Add(time.Hour))

	if err := f.svc.Confirm(context.Background(), testRawVerificationToken); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if len(f.users.markedVerified) != 1 {
		t.Fatalf("marked %d users verified, want 1", len(f.users.markedVerified))
	}
	if len(f.tokens.consumed) != 1 {
		t.Fatal("the link was not redeemed, so it would work a second time")
	}
}

func TestVerificationService_Confirm_RejectsATokenThatIsNotUsable(t *testing.T) {
	t.Parallel()
	consumed := fixedNow.Add(-time.Minute)
	cases := []struct {
		name  string
		token *domain.AuthToken
	}{
		{"unknown", nil},
		{"expired", storedVerificationToken(fixedNow.Add(-time.Second))},
		{"already redeemed", func() *domain.AuthToken {
			tk := storedVerificationToken(fixedNow.Add(time.Hour))
			tk.ConsumedAt = &consumed
			return tk
		}()},
		{"a password-reset token", func() *domain.AuthToken {
			tk := storedVerificationToken(fixedNow.Add(time.Hour))
			tk.Type = domain.AuthTokenTypePasswordReset
			return tk
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newVerificationFixture(t, passwordUser(t))
			f.tokens.stored = tc.token

			err := f.svc.Confirm(context.Background(), testRawVerificationToken)
			if !errors.Is(err, domain.ErrUnauthenticated) {
				t.Fatalf("Confirm with a %s token = %v, want ErrUnauthenticated", tc.name, err)
			}
			if len(f.users.markedVerified) != 0 {
				t.Fatalf("a %s token verified the address anyway", tc.name)
			}
		})
	}
}

// Clicking a link twice is something a user does by accident, and the second click has no
// way to look different to them, so it is a success rather than an error.
func TestVerificationService_Confirm_AnAlreadyVerifiedAddressSucceeds(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	verifiedAt := fixedNow.Add(-time.Hour)
	user.EmailVerifiedAt = &verifiedAt
	f := newVerificationFixture(t, user)
	f.tokens.stored = storedVerificationToken(fixedNow.Add(time.Hour))

	if err := f.svc.Confirm(context.Background(), testRawVerificationToken); err != nil {
		t.Fatalf("Confirm on an already-verified address = %v, want no error", err)
	}
	if len(f.users.markedVerified) != 0 {
		t.Fatal("the verification timestamp was moved by a second confirmation")
	}
}

func TestVerificationService_Confirm_RejectsADeactivatedAccount(t *testing.T) {
	t.Parallel()
	f := newVerificationFixture(t, passwordUser(t))
	f.users.accountIsActive = false
	f.tokens.stored = storedVerificationToken(fixedNow.Add(time.Hour))

	err := f.svc.Confirm(context.Background(), testRawVerificationToken)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Confirm for a deactivated account = %v, want ErrUnauthenticated", err)
	}
	if len(f.users.markedVerified) != 0 {
		t.Fatal("a deactivated account's address was verified")
	}
}

func TestVerificationService_Resend_AnswersTheSameForEveryAddress(t *testing.T) {
	t.Parallel()
	verifiedAt := fixedNow.Add(-time.Hour)
	cases := []struct {
		name    string
		prepare func(*verificationFixture)
	}{
		{"unregistered", func(f *verificationFixture) { f.users.getByEmailErr = domain.ErrNotFound }},
		{"already verified", func(f *verificationFixture) { f.users.user.EmailVerifiedAt = &verifiedAt }},
		{"deactivated account", func(f *verificationFixture) { f.users.accountIsActive = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newVerificationFixture(t, passwordUser(t))
			tc.prepare(f)

			if err := f.svc.Resend(context.Background(), "quien-sea@corralon.test"); err != nil {
				t.Fatalf("Resend for %s = %v, want nil: the answer must not differ", tc.name, err)
			}
			if len(f.mail.sent) != 0 {
				t.Fatalf("a link was mailed for %s", tc.name)
			}
		})
	}
}

func TestVerificationService_Resend_RetiresThePreviousLink(t *testing.T) {
	t.Parallel()
	f := newVerificationFixture(t, passwordUser(t))

	if err := f.svc.Resend(context.Background(), "ana@corralon.test"); err != nil {
		t.Fatalf("Resend: %v", err)
	}
	if len(f.tokens.invalidated) != 1 {
		t.Fatal("the outstanding link was not retired before a new one was minted")
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d mails, want 1", len(f.mail.sent))
	}
}

const testNewAddress = "ana.nueva@corralon.test"

func TestVerificationService_ChangeOwnEmail_WritesTheAddressAndMailsTheNewOne(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	verifiedAt := fixedNow.Add(-time.Hour)
	user.EmailVerifiedAt = &verifiedAt
	f := newVerificationFixture(t, user)

	if err := f.svc.ChangeOwnEmail(context.Background(), testTenant(),
		testCurrentPassword, "  Ana.Nueva@Corralon.TEST "); err != nil {
		t.Fatalf("ChangeOwnEmail: %v", err)
	}
	// Folded and trimmed before the write, so the address stored is the one the global unique
	// index compares.
	if f.users.user.Email != testNewAddress {
		t.Fatalf("stored address = %q, want %q", f.users.user.Email, testNewAddress)
	}
	if f.users.user.EmailVerifiedAt != nil {
		t.Fatal("the confirmation survived a change of address")
	}
	if len(f.tokens.created) != 1 || f.tokens.created[0].Type != domain.AuthTokenTypeEmailVerification {
		t.Fatalf("minted %v, want one email-verification link", f.tokens.created)
	}
	// Two messages: the link to the new address, and the warning to the one it left.
	if len(f.mail.sent) != 2 {
		t.Fatalf("sent %d mails, want 2", len(f.mail.sent))
	}
	if got := f.mail.sent[1].To; got != testNewAddress {
		t.Fatalf("the link went to %q, want the new address %q", got, testNewAddress)
	}
}

// The old mailbox is the only place a takeover shows up, since the account itself is now
// reachable only by whoever holds the new address.
func TestVerificationService_ChangeOwnEmail_WarnsTheAddressItLeft(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	previous := user.Email
	f := newVerificationFixture(t, user)

	if err := f.svc.ChangeOwnEmail(context.Background(), testTenant(),
		testCurrentPassword, testNewAddress); err != nil {
		t.Fatalf("ChangeOwnEmail: %v", err)
	}
	warning := f.mail.sent[0]
	if warning.To != previous {
		t.Fatalf("the warning went to %q, want the previous address %q", warning.To, previous)
	}
	if warning.Event != domain.NotificationEventEmailChanged {
		t.Fatalf("warning event = %q, want %q", warning.Event, domain.NotificationEventEmailChanged)
	}
	// It has to name the address that replaced it, or the reader cannot tell whose change it was.
	if !strings.Contains(strings.Join(warning.Paragraphs, " "), testNewAddress) {
		t.Fatalf("the warning does not name the new address: %v", warning.Paragraphs)
	}
	if warning.ActionURL != "" {
		t.Fatal("the warning carries a link, so a stranger's mailbox was handed one")
	}
}

// A recovery link already sitting in the old mailbox would let whoever reads it take the
// account back, so changing the address retires it.
func TestVerificationService_ChangeOwnEmail_RetiresOutstandingRecoveryLinks(t *testing.T) {
	t.Parallel()
	f := newVerificationFixture(t, passwordUser(t))

	if err := f.svc.ChangeOwnEmail(context.Background(), testTenant(),
		testCurrentPassword, testNewAddress); err != nil {
		t.Fatalf("ChangeOwnEmail: %v", err)
	}
	var retiredResets int
	for _, link := range f.tokens.invalidated {
		if link.tokenType == domain.AuthTokenTypePasswordReset && link.userID == testUserID {
			retiredResets++
		}
	}
	if retiredResets != 1 {
		t.Fatalf("retired %d recovery links for the caller, want 1 (all: %v)",
			retiredResets, f.tokens.invalidated)
	}
}

func TestVerificationService_ChangeOwnEmail_RefusesAWrongPassword(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	previous := user.Email
	f := newVerificationFixture(t, user)

	err := f.svc.ChangeOwnEmail(context.Background(), testTenant(), "no-es-la-clave", testNewAddress)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("ChangeOwnEmail with a wrong password = %v, want ErrUnauthenticated", err)
	}
	if f.users.user.Email != previous {
		t.Fatal("a wrong password moved the address anyway")
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("a wrong password still mailed somebody")
	}
}

// Answered as a taken address rather than as its own rule, so the response cannot be used to
// find out whether a stranger holds one.
func TestVerificationService_ChangeOwnEmail_TheCallersOwnAddressIsAConflict(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	f := newVerificationFixture(t, user)

	err := f.svc.ChangeOwnEmail(context.Background(), testTenant(), testCurrentPassword,
		strings.ToUpper(user.Email))
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ChangeOwnEmail to the caller's own address = %v, want ErrConflict", err)
	}
	if got := domain.CodeOf(err); got != domain.CodeEmailTaken {
		t.Fatalf("code = %q, want %q", got, domain.CodeEmailTaken)
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("an address that did not change still mailed a link")
	}
}

/*
 * A recovery link redeemed inside the bcrypt window moves the password, and the address change
 * must not be spendable on the one it just replaced — otherwise whoever held the old password
 * gets the recovery mailbox, which is worse than what they lost.
 */
func TestVerificationService_ChangeOwnEmail_RefusesAPasswordThatMovedMeanwhile(t *testing.T) {
	t.Parallel()
	user := passwordUser(t)
	previous := user.Email
	f := newVerificationFixture(t, user)
	// Read, then rotated: the compare passes and the write no longer matches.
	f.users.hashMovesTo = "$2a$10$otro-hash-que-nadie-comparo"

	err := f.svc.ChangeOwnEmail(context.Background(), testTenant(), testCurrentPassword, testNewAddress)
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("ChangeOwnEmail with a rotated password = %v, want ErrUnauthenticated", err)
	}
	if f.users.user.Email != previous {
		t.Fatal("the address moved on a password that was no longer the account's")
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("a refused change still mailed somebody")
	}
}

// The write is what has to fail the request; the two mails are best-effort and come after it.
func TestVerificationService_ChangeOwnEmail_AConflictingAddressMailsNobody(t *testing.T) {
	t.Parallel()
	f := newVerificationFixture(t, passwordUser(t))
	f.users.updateEmailErr = domain.WithCode(domain.CodeEmailTaken, domain.ErrConflict)

	err := f.svc.ChangeOwnEmail(context.Background(), testTenant(), testCurrentPassword, testNewAddress)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ChangeOwnEmail onto a taken address = %v, want ErrConflict", err)
	}
	if len(f.mail.sent) != 0 {
		t.Fatal("a refused change still mailed somebody")
	}
}
