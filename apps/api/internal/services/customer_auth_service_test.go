package services

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type customerAuthFake struct {
	site         domain.CustomerSite
	identity     *domain.CustomerIdentity
	session      *domain.CustomerSession
	failed       int
	succeeded    bool
	revoked      bool
	lockedLookup func()
}

func (f *customerAuthFake) GetSiteCrossAccount(_ context.Context, _ repository.Querier, host string) (*domain.CustomerSite, error) {
	if host != f.site.Hostname {
		return nil, domain.ErrNotFound
	}
	return &f.site, nil
}

func (f *customerAuthFake) GetIdentityByTaxID(_ context.Context, _ repository.Querier, accountID uuid.UUID, taxID string) (*domain.CustomerIdentity, error) {
	if f.identity == nil || accountID != f.identity.AccountID || taxID != f.identity.TaxID {
		return nil, domain.ErrNotFound
	}
	copy := *f.identity
	return &copy, nil
}

func (f *customerAuthFake) GetIdentityForUpdate(_ context.Context, _ repository.Querier, accountID, id uuid.UUID) (*domain.CustomerIdentity, error) {
	if f.lockedLookup != nil {
		f.lockedLookup()
	}
	if f.identity == nil || f.identity.AccountID != accountID || f.identity.ID != id {
		return nil, domain.ErrNotFound
	}
	return f.identity, nil
}

func (f *customerAuthFake) GetSession(_ context.Context, _ repository.Querier, accountID uuid.UUID, hash string, now time.Time) (*domain.CustomerPrincipal, error) {
	if f.session == nil || f.session.AccountID != accountID || f.session.TokenHash != hash || !f.session.ExpiresAt.After(now) || f.session.RevokedAt != nil || f.identity.DisabledAt != nil {
		return nil, domain.ErrUnauthenticated
	}
	return &domain.CustomerPrincipal{CustomerID: f.identity.ID, AccountID: accountID, TaxID: f.identity.TaxID, SessionID: f.session.ID, SessionExpiresAt: f.session.ExpiresAt}, nil
}

func (f *customerAuthFake) CreateIdentity(_ context.Context, _ repository.Querier, identity domain.CustomerIdentity) (*domain.CustomerIdentity, error) {
	if f.identity != nil {
		return nil, domain.ErrConflict
	}
	f.identity = &identity
	return f.identity, nil
}

func (f *customerAuthFake) CreateSession(_ context.Context, _ repository.Querier, session domain.CustomerSession) error {
	f.session = &session
	return nil
}

func (f *customerAuthFake) RecordFailedAttempt(_ context.Context, _ repository.Querier, accountID, id uuid.UUID, _ int, _, _ time.Time) error {
	if accountID != f.identity.AccountID || id != f.identity.ID {
		return domain.ErrForbidden
	}
	f.failed++
	return nil
}

func (f *customerAuthFake) RecordSuccessfulLogin(_ context.Context, _ repository.Querier, _, _ uuid.UUID) error {
	f.succeeded = true
	return nil
}

func (f *customerAuthFake) RevokeSession(_ context.Context, _ repository.Querier, accountID, customerID, sessionID uuid.UUID, now time.Time) error {
	if accountID != f.session.AccountID || customerID != f.session.CustomerID || sessionID != f.session.ID {
		return domain.ErrForbidden
	}
	f.revoked = true
	f.session.RevokedAt = &now
	return nil
}

func customerAuthFixture() (*CustomerAuthService, *customerAuthFake, *fakeDB) {
	f := &customerAuthFake{site: domain.CustomerSite{AccountID: uuid.New(), Hostname: "supplier.coti.test"}}
	db := &fakeDB{}
	cfg := config.CustomerAuthConfig{SessionTTL: 30 * 24 * time.Hour, MaxFailedAttempts: 5, LockoutDuration: 15 * time.Minute, PasswordMinLength: 12}
	svc := NewCustomerAuthService(db, f, cfg, func() time.Time { return fixedNow }).WithClientProfiles(fakeCustomerProfileRegistrar{})
	return svc, f, db
}

func registerCustomer(t *testing.T, svc *CustomerAuthService) *domain.CustomerLogin {
	t.Helper()
	login, err := svc.Register(context.Background(), "supplier.coti.test", domain.CustomerRegistration{Name: "Juan Perez", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "customer@example.test", Primary: true}}, TaxID: "20-12345678-6", Password: "Customer-pass123!"})
	if err != nil {
		t.Fatal(err)
	}
	return login
}

func TestCustomerAuthRegistrationAndSessionLifecycle(t *testing.T) {
	svc, store, db := customerAuthFixture()
	login := registerCustomer(t, svc)
	if store.identity.TaxID != "20123456786" || store.identity.ID == uuid.Nil || store.identity.AccountID != store.site.AccountID {
		t.Fatalf("invalid customer identity: %+v", store.identity)
	}
	if bcrypt.CompareHashAndPassword([]byte(store.identity.PasswordHash), []byte("Customer-pass123!")) != nil {
		t.Fatal("password was not hashed")
	}
	if store.session.TokenHash == login.SessionToken || store.session.TokenHash != hashToken(login.SessionToken) {
		t.Fatal("session must persist only the opaque secret's hash")
	}
	if !login.Principal.SessionExpiresAt.Equal(fixedNow.Add(30 * 24 * time.Hour)) {
		t.Fatal("incorrect absolute expiration")
	}
	principal, err := svc.Authenticate(context.Background(), "SUPPLIER.COTI.TEST.", login.SessionToken)
	if err != nil || principal.CustomerID != store.identity.ID {
		t.Fatalf("authenticate = %+v, %v", principal, err)
	}
	for _, accountID := range db.scopes {
		if accountID != store.site.AccountID {
			t.Fatal("wrong tenant scope")
		}
	}
	if err := svc.Logout(context.Background(), *principal); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), store.site.Hostname, login.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked session accepted: %v", err)
	}
}

func TestCustomerAuthRejectsWrongSupplierAndInvalidSessions(t *testing.T) {
	svc, store, _ := customerAuthFixture()
	login := registerCustomer(t, svc)
	store.site.AccountID = uuid.New()
	if _, err := svc.Authenticate(context.Background(), store.site.Hostname, login.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("session accepted in another supplier: %v", err)
	}
	store.site.AccountID = store.identity.AccountID
	for _, secret := range []string{"", "staff.jwt.value", base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		if _, err := svc.Authenticate(context.Background(), store.site.Hostname, secret); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("invalid session accepted: %v", err)
		}
	}
	store.session.ExpiresAt = fixedNow
	if _, err := svc.Authenticate(context.Background(), store.site.Hostname, login.SessionToken); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expired session accepted: %v", err)
	}
}

func TestCustomerAuthLoginFailuresAndConcurrentCredentialChange(t *testing.T) {
	svc, store, _ := customerAuthFixture()
	registerCustomer(t, svc)
	store.session = nil
	credentials := domain.CustomerCredentials{TaxID: store.identity.TaxID, Password: "wrong"}
	if _, err := svc.Login(context.Background(), store.site.Hostname, credentials); !errors.Is(err, domain.ErrUnauthenticated) || store.failed != 1 || store.session != nil {
		t.Fatalf("bad password not rejected: %v", err)
	}
	until := fixedNow.Add(time.Minute)
	store.identity.LockedUntil = &until
	credentials.Password = "Customer-pass123!"
	if _, err := svc.Login(context.Background(), store.site.Hostname, credentials); !errors.Is(err, domain.ErrLocked) || store.session != nil {
		t.Fatalf("locked credential accepted: %v", err)
	}
	store.identity.LockedUntil = nil
	store.lockedLookup = func() { store.identity.PasswordHash = "changed-concurrently" }
	if _, err := svc.Login(context.Background(), store.site.Hostname, credentials); !errors.Is(err, domain.ErrUnauthenticated) || store.session != nil {
		t.Fatalf("session issued after credential change: %v", err)
	}
}

func TestCustomerAuthRegistrationRejectsInvalidAndDuplicateCredentials(t *testing.T) {
	svc, store, _ := customerAuthFixture()
	for _, in := range []domain.CustomerRegistration{{TaxID: "00000000000", Password: "Customer-pass123!"}, {TaxID: "20123456786", Password: "short"}} {
		if _, err := svc.Register(context.Background(), store.site.Hostname, in); !errors.Is(err, domain.ErrInvalidInput) || store.identity != nil {
			t.Fatalf("invalid registration persisted: %v", err)
		}
	}
	registerCustomer(t, svc)
	if _, err := svc.Register(context.Background(), store.site.Hostname, domain.CustomerRegistration{Name: "Juan Perez", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "customer@example.test", Primary: true}}, TaxID: store.identity.TaxID, Password: "Customer-pass123!"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate registration: %v", err)
	}
}

type fakeCustomerProfileRegistrar struct{}

func (fakeCustomerProfileRegistrar) RegisterProfile(context.Context, repository.Querier, domain.CustomerPrincipal, domain.ClientContactUpdate) error {
	return nil
}
