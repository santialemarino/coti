package services

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type customerCredentialsFake struct {
	*customerAuthFake
	tokens map[string]domain.CustomerChallenge
	used   map[string]bool
	reset  bool
}

func (f *customerCredentialsFake) GetIdentityByGoogleSubject(_ context.Context, _ repository.Querier, accountID uuid.UUID, subject string) (*domain.CustomerIdentity, error) {
	if f.identity == nil || f.identity.AccountID != accountID || f.identity.GoogleSubject == nil || *f.identity.GoogleSubject != subject {
		return nil, domain.ErrNotFound
	}
	return f.identity, nil
}
func (f *customerCredentialsFake) GetVerifiedRecoveryIdentity(_ context.Context, _ repository.Querier, accountID uuid.UUID, taxID, email string) (*domain.CustomerIdentity, error) {
	if f.identity == nil || f.identity.AccountID != accountID || f.identity.TaxID != taxID || f.identity.RecoveryEmail == nil || *f.identity.RecoveryEmail != email || f.identity.RecoveryEmailVerifiedAt == nil {
		return nil, domain.ErrNotFound
	}
	return f.identity, nil
}
func (f *customerCredentialsFake) ConsumeChallenge(_ context.Context, _ repository.Querier, accountID uuid.UUID, hash string, purpose domain.CustomerChallengePurpose, now time.Time) (*domain.CustomerChallenge, error) {
	token, ok := f.tokens[hash]
	if !ok || token.AccountID != accountID || token.Purpose != purpose || !token.ExpiresAt.After(now) || f.used[hash] {
		return nil, domain.ErrUnauthenticated
	}
	f.used[hash] = true
	return &token, nil
}
func (f *customerCredentialsFake) CreateGoogleIdentity(ctx context.Context, q repository.Querier, in domain.CustomerIdentity) (*domain.CustomerIdentity, error) {
	return f.CreateIdentity(ctx, q, in)
}
func (f *customerCredentialsFake) CreateChallenge(_ context.Context, _ repository.Querier, in domain.CustomerChallenge) error {
	f.tokens[in.TokenHash] = in
	return nil
}
func (f *customerCredentialsFake) LinkGoogle(_ context.Context, _ repository.Querier, accountID, customerID uuid.UUID, subject string) error {
	if accountID != f.identity.AccountID || customerID != f.identity.ID {
		return domain.ErrForbidden
	}
	f.identity.GoogleSubject = &subject
	return nil
}
func (f *customerCredentialsFake) VerifyRecoveryEmail(_ context.Context, _ repository.Querier, _, _ uuid.UUID, email string, now time.Time) error {
	if f.identity.RecoveryEmail == nil || *f.identity.RecoveryEmail != email {
		return domain.ErrUnauthenticated
	}
	f.identity.RecoveryEmailVerifiedAt = &now
	return nil
}
func (f *customerCredentialsFake) ResetCustomerPassword(_ context.Context, _ repository.Querier, _, _ uuid.UUID, hash string, _ time.Time) error {
	f.identity.PasswordHash = hash
	f.reset = true
	f.revoked = true
	return nil
}

type customerGoogleFake struct{}

func (f *customerCredentialsFake) GetHostnameByDeliveryTokenCrossAccount(context.Context, repository.Querier, string) (string, error) {
	return "", domain.ErrNotFound
}

func (customerGoogleFake) Exchange(context.Context, string, string, string, string) (*domain.CustomerGoogleIdentity, error) {
	return &domain.CustomerGoogleIdentity{Subject: "stable-google-subject", Email: "customer@example.test", Name: "Customer"}, nil
}

func customerIdentityFixture() (*CustomerAuthService, *customerCredentialsFake, *fakeMail) {
	svc, base, _ := customerAuthFixture()
	store := &customerCredentialsFake{customerAuthFake: base, tokens: map[string]domain.CustomerChallenge{}, used: map[string]bool{}}
	mail := &fakeMail{}
	svc.cfg.SignupTTL = 10 * time.Minute
	svc.cfg.RecoveryTTL = time.Hour
	svc.cfg.VerificationTTL = 24 * time.Hour
	svc.WithIdentityProviders(store, customerGoogleFake{}, mail)
	return svc, store, mail
}

func TestCustomerGoogleFirstRegistrationRequiresCUITAndDoesNotMergeByEmail(t *testing.T) {
	svc, store, _ := customerIdentityFixture()
	ctx := context.Background()
	secret, err := newTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.GoogleExchange(ctx, store.site.Hostname, "single-use-code", secret, secret)
	if err != nil || result.Login != nil || result.SignupToken == "" || store.identity != nil {
		t.Fatalf("first Google sign-in: %+v %v", result, err)
	}
	login, err := svc.CompleteGoogle(ctx, store.site.Hostname, result.SignupToken, "20123456786", nil)
	if err != nil || login.Principal.TaxID != "20123456786" || store.identity.GoogleSubject == nil || store.identity.RecoveryEmail != nil {
		t.Fatalf("Google registration: %+v %v", login, err)
	}
	if _, err = svc.CompleteGoogle(ctx, store.site.Hostname, result.SignupToken, "20123456786", nil); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("Google signup capability reused: %v", err)
	}
	known, err := svc.GoogleExchange(ctx, store.site.Hostname, "next-code", secret, secret)
	if err != nil || known.Login == nil || known.SignupToken != "" || known.Login.Principal.CustomerID != store.identity.ID {
		t.Fatalf("known Google subject: %+v %v", known, err)
	}
}

func TestCustomerGoogleCannotClaimExistingCUITWithoutAuthenticatedLink(t *testing.T) {
	svc, store, _ := customerIdentityFixture()
	ctx := context.Background()
	registered := registerCustomer(t, svc)
	secret, err := newTokenSecret()
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.GoogleExchange(ctx, store.site.Hostname, "code", secret, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.CompleteGoogle(ctx, store.site.Hostname, result.SignupToken, registered.Principal.TaxID, nil); !errors.Is(err, domain.ErrConflict) || store.identity.GoogleSubject != nil {
		t.Fatalf("unverified CUIT claimed existing credentials: %v", err)
	}
	result, err = svc.GoogleExchange(ctx, store.site.Hostname, "another-code", secret, secret)
	if err != nil {
		t.Fatal(err)
	}
	wrong := registered.Principal
	wrong.AccountID = uuid.New()
	if _, err = svc.CompleteGoogle(ctx, store.site.Hostname, result.SignupToken, registered.Principal.TaxID, &wrong); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("cross-account Google link: %v", err)
	}
	if _, err = svc.CompleteGoogle(ctx, store.site.Hostname, result.SignupToken, registered.Principal.TaxID, &registered.Principal); err != nil || store.identity.GoogleSubject == nil {
		t.Fatalf("authenticated Google link: %v", err)
	}
}

func TestCustomerRecoveryRequiresStoredVerifiedEmailAndSingleUseMatchingPurpose(t *testing.T) {
	svc, store, mail := customerIdentityFixture()
	ctx := context.Background()
	_, err := svc.Register(ctx, store.site.Hostname, domain.CustomerRegistration{Name: "Juan Perez", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "customer@example.test", Primary: true}}, TaxID: "20123456786", Password: "Customer-pass123!", RecoveryEmail: "customer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 1 {
		t.Fatal("missing verification mail")
	}
	verificationURL, err := url.Parse(mail.sent[0].ActionURL)
	if err != nil {
		t.Fatal(err)
	}
	verification := verificationURL.Query().Get("token")
	if err = svc.ForgotCustomerPassword(ctx, store.site.Hostname, "20123456786", "customer@example.test"); err != nil || len(mail.sent) != 1 {
		t.Fatal("unverified address used for recovery")
	}
	if err = svc.ResetCustomerPassword(ctx, store.site.Hostname, verification, "Replacement-pass123!"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("verification capability reset password: %v", err)
	}
	if err = svc.VerifyCustomerEmail(ctx, store.site.Hostname, verification); err != nil {
		t.Fatal(err)
	}
	if err = svc.VerifyCustomerEmail(ctx, store.site.Hostname, verification); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("verification link reused: %v", err)
	}
	if err = svc.ForgotCustomerPassword(ctx, store.site.Hostname, "20123456786", "attacker@example.test"); err != nil || len(mail.sent) != 1 {
		t.Fatal("caller-selected recovery email received token")
	}
	if err = svc.ForgotCustomerPassword(ctx, store.site.Hostname, "20123456786", "customer@example.test"); err != nil || len(mail.sent) != 2 {
		t.Fatalf("verified recovery: %v", err)
	}
	u, err := url.Parse(mail.sent[1].ActionURL)
	if err != nil {
		t.Fatal(err)
	}
	reset := u.Query().Get("token")
	if err = svc.ResetCustomerPassword(ctx, store.site.Hostname, reset, "Replacement-pass123!"); err != nil || !store.reset || !store.revoked || bcrypt.CompareHashAndPassword([]byte(store.identity.PasswordHash), []byte("Replacement-pass123!")) != nil {
		t.Fatalf("reset did not replace hash and revoke sessions: %v", err)
	}
	if err = svc.ResetCustomerPassword(ctx, store.site.Hostname, reset, "Another-pass123!"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("reset link reused: %v", err)
	}
}

func (f *customerCredentialsFake) SetRecoveryEmail(_ context.Context, _ repository.Querier, accountID, customerID uuid.UUID, email *string) error {
	if f.identity == nil || f.identity.AccountID != accountID || f.identity.ID != customerID {
		return domain.ErrUnauthenticated
	}
	f.identity.RecoveryEmail = email
	f.identity.RecoveryEmailVerifiedAt = nil
	f.tokens = map[string]domain.CustomerChallenge{}
	return nil
}

func (f *customerCredentialsFake) GetChallengeCustomer(_ context.Context, _ repository.Querier, accountID uuid.UUID, hash string, purpose domain.CustomerChallengePurpose, now time.Time) (uuid.UUID, error) {
	token, ok := f.tokens[hash]
	if !ok || token.AccountID != accountID || token.Purpose != purpose || !token.ExpiresAt.After(now) || f.used[hash] || token.CustomerID == nil {
		return uuid.Nil, domain.ErrUnauthenticated
	}
	return *token.CustomerID, nil
}
