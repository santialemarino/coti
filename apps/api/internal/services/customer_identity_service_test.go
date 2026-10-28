package services

import (
	"context"
	"errors"
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

func customerIdentityFixture() (*CustomerAuthService, *customerCredentialsFake) {
	svc, base, _ := customerAuthFixture()
	store := &customerCredentialsFake{customerAuthFake: base, tokens: map[string]domain.CustomerChallenge{}, used: map[string]bool{}}
	svc.cfg.SignupTTL = 10 * time.Minute
	svc.WithIdentityProviders(store, customerGoogleFake{})
	return svc, store
}

func TestCustomerGoogleFirstRegistrationRequiresCUITAndDoesNotMergeByEmail(t *testing.T) {
	svc, store := customerIdentityFixture()
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
	if err != nil || login.Principal.TaxID != "20123456786" || store.identity.GoogleSubject == nil {
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
	svc, store := customerIdentityFixture()
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

// recoveryFake redeems one code for one customer and counts every attempt it was asked to judge.
type recoveryFake struct {
	customerID uuid.UUID
	code       string
	attempts   int
}

func (f *recoveryFake) ListRecoveryContacts(context.Context, repository.Querier, domain.CustomerPrincipal) ([]domain.RecoveryContact, error) {
	return []domain.RecoveryContact{}, nil
}
func (f *recoveryFake) RequestRecovery(context.Context, domain.CustomerPrincipal, uuid.UUID) (*domain.ContactChallenge, error) {
	return &domain.ContactChallenge{ID: uuid.New()}, nil
}
func (f *recoveryFake) RedeemRecovery(_ context.Context, _ repository.Querier, _, _ uuid.UUID, code string) (uuid.UUID, bool, error) {
	f.attempts++
	if code != f.code {
		return uuid.Nil, false, nil
	}
	return f.customerID, true, nil
}

func TestCustomerResetWithRecoveryCodeChecksThePolicyFirstAndSignsIn(t *testing.T) {
	svc, store := customerIdentityFixture()
	ctx := context.Background()
	registered := registerCustomer(t, svc)
	recovery := &recoveryFake{customerID: registered.Principal.CustomerID, code: "123456"}
	svc.WithContactRecovery(recovery)
	if _, err := svc.ResetWithRecoveryCode(ctx, store.site.Hostname, uuid.New(), "123456", "short"); err == nil || recovery.attempts != 0 {
		t.Fatalf("weak password spent an attempt: %v, attempts %d", err, recovery.attempts)
	}
	if _, err := svc.ResetWithRecoveryCode(ctx, store.site.Hostname, uuid.New(), "654321", "Replacement-pass123!"); !errors.Is(err, domain.ErrUnauthenticated) || store.reset {
		t.Fatalf("wrong code reset the password: %v", err)
	}
	login, err := svc.ResetWithRecoveryCode(ctx, store.site.Hostname, uuid.New(), "123456", "Replacement-pass123!")
	if err != nil || login.Principal.CustomerID != registered.Principal.CustomerID || !store.reset || !store.revoked || bcrypt.CompareHashAndPassword([]byte(store.identity.PasswordHash), []byte("Replacement-pass123!")) != nil {
		t.Fatalf("recovery did not replace the hash, revoke sessions and sign in: %+v %v", login, err)
	}
}
