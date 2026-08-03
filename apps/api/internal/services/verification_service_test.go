package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	return f.subject(), nil
}

func (f *fakeVerificationUsers) GetAuthSubjectByEmailCrossAccount(
	context.Context, repository.Querier, string,
) (*domain.AuthSubject, error) {
	if f.getByEmailErr != nil {
		return nil, f.getByEmailErr
	}
	return f.subject(), nil
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
