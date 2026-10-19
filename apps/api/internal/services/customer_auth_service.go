package services

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type customerAuthStore interface {
	GetSiteCrossAccount(context.Context, repository.Querier, string) (*domain.CustomerSite, error)
	GetIdentityByTaxID(context.Context, repository.Querier, uuid.UUID, string) (*domain.CustomerIdentity, error)
	GetIdentityForUpdate(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.CustomerIdentity, error)
	GetSession(context.Context, repository.Querier, uuid.UUID, string, time.Time) (*domain.CustomerPrincipal, error)
	CreateIdentity(context.Context, repository.Querier, domain.CustomerIdentity) (*domain.CustomerIdentity, error)
	CreateSession(context.Context, repository.Querier, domain.CustomerSession) error
	RecordFailedAttempt(context.Context, repository.Querier, uuid.UUID, uuid.UUID, int, time.Time, time.Time) error
	RecordSuccessfulLogin(context.Context, repository.Querier, uuid.UUID, uuid.UUID) error
	RevokeSession(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
}

// CustomerAuthService authenticates supplier-scoped customers separately from staff and client profiles.
type CustomerAuthService struct {
	db          tenantScoper
	store       customerAuthStore
	credentials customerCredentialStore
	google      domain.CustomerGoogleProvider
	mail        mailSender
	cfg         config.CustomerAuthConfig
	now         func() time.Time
	newSecret   func() (string, error)
}

// NewCustomerAuthService builds the customer credential lifecycle over existing cryptographic helpers.
func NewCustomerAuthService(db tenantScoper, store customerAuthStore, cfg config.CustomerAuthConfig, now func() time.Time) *CustomerAuthService {
	if now == nil {
		now = time.Now
	}
	return &CustomerAuthService{db: db, store: store, cfg: cfg, now: now, newSecret: newTokenSecret}
}

// Authenticate checks the opaque session against its supplier, expiration and revocation state.
func (s *CustomerAuthService) Authenticate(ctx context.Context, hostname, raw string) (*domain.CustomerPrincipal, error) {
	if !validCustomerSecret(raw) {
		return nil, domain.ErrUnauthenticated
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	var principal *domain.CustomerPrincipal
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		var err error
		principal, err = s.store.GetSession(ctx, q, site.AccountID, hashToken(raw), s.now())
		return err
	})
	return principal, err
}

// Register creates supplier-scoped credentials and their first session.
func (s *CustomerAuthService) Register(ctx context.Context, hostname string, in domain.CustomerRegistration) (*domain.CustomerLogin, error) {
	in.TaxID = domain.NormalizeTaxID(in.TaxID)
	in.RecoveryEmail = domain.NormalizeEmail(in.RecoveryEmail)
	if in.RecoveryEmail != "" && !validCustomerEmail(in.RecoveryEmail) {
		return nil, domain.ErrInvalidInput
	}
	if !domain.ValidCUIT(in.TaxID) {
		return nil, domain.ErrInvalidInput
	}
	if err := (domain.PasswordPolicy{MinLength: s.cfg.PasswordMinLength}).Validate(in.Password); err != nil {
		return nil, err
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	secret, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	var login *domain.CustomerLogin
	var recovery *string
	if in.RecoveryEmail != "" {
		recovery = &in.RecoveryEmail
	}
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		identity, err := s.store.CreateIdentity(ctx, q, domain.CustomerIdentity{ID: uuid.New(), AccountID: site.AccountID, TaxID: in.TaxID, PasswordHash: hash, RecoveryEmail: recovery})
		if err != nil {
			return err
		}
		login, err = s.createSession(ctx, q, *identity, secret)
		return err
	})
	if err == nil && recovery != nil && s.credentials != nil && s.mail != nil {
		_ = s.sendCustomerChallenge(ctx, *site, domain.CustomerIdentity{ID: login.Principal.CustomerID, AccountID: site.AccountID}, "VERIFY_EMAIL", *recovery)
	}
	return login, err
}

// Login verifies CUIT and password and rechecks the credential under lock before issuing a session.
func (s *CustomerAuthService) Login(ctx context.Context, hostname string, in domain.CustomerCredentials) (*domain.CustomerLogin, error) {
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	in.TaxID = domain.NormalizeTaxID(in.TaxID)
	if !domain.ValidCUIT(in.TaxID) {
		return nil, domain.ErrUnauthenticated
	}
	tenant := domain.Tenant{AccountID: site.AccountID}
	var identity *domain.CustomerIdentity
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		identity, err = s.store.GetIdentityByTaxID(ctx, q, site.AccountID, in.TaxID)
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Password))
		return nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if identity.LockedUntil != nil && identity.LockedUntil.After(s.now()) {
		return nil, domain.ErrLocked
	}
	if bcrypt.CompareHashAndPassword([]byte(identity.PasswordHash), []byte(in.Password)) != nil {
		err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			return s.store.RecordFailedAttempt(ctx, q, site.AccountID, identity.ID, s.cfg.MaxFailedAttempts, s.now(), s.now().Add(s.cfg.LockoutDuration))
		})
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrUnauthenticated
	}
	secret, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	var login *domain.CustomerLogin
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		current, err := s.store.GetIdentityForUpdate(ctx, q, site.AccountID, identity.ID)
		if err != nil {
			return err
		}
		if current.DisabledAt != nil || current.PasswordHash != identity.PasswordHash {
			return domain.ErrUnauthenticated
		}
		if current.LockedUntil != nil && current.LockedUntil.After(s.now()) {
			return domain.ErrLocked
		}
		if err := s.store.RecordSuccessfulLogin(ctx, q, site.AccountID, current.ID); err != nil {
			return err
		}
		login, err = s.createSession(ctx, q, *current, secret)
		return err
	})
	return login, err
}

// Logout revokes the authenticated session in persistent storage.
func (s *CustomerAuthService) Logout(ctx context.Context, principal domain.CustomerPrincipal) error {
	return s.db.InTenantTx(ctx, domain.Tenant{AccountID: principal.AccountID}, func(q repository.Querier) error {
		return s.store.RevokeSession(ctx, q, principal.AccountID, principal.CustomerID, principal.SessionID, s.now())
	})
}

func (s *CustomerAuthService) resolveSite(ctx context.Context, hostname string) (*domain.CustomerSite, error) {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	if hostname == "" || len(hostname) > 253 || strings.ContainsAny(hostname, "/:@ \\?#") {
		return nil, domain.ErrNotFound
	}
	return s.store.GetSiteCrossAccount(ctx, s.db.CrossAccount(), hostname)
}

func (s *CustomerAuthService) createSession(ctx context.Context, q repository.Querier, identity domain.CustomerIdentity, secret string) (*domain.CustomerLogin, error) {
	session := domain.CustomerSession{ID: uuid.New(), AccountID: identity.AccountID, CustomerID: identity.ID, TokenHash: hashToken(secret), ExpiresAt: s.now().Add(s.cfg.SessionTTL)}
	if err := s.store.CreateSession(ctx, q, session); err != nil {
		return nil, err
	}
	return &domain.CustomerLogin{SessionToken: secret, Principal: domain.CustomerPrincipal{
		CustomerID: identity.ID, AccountID: identity.AccountID, TaxID: identity.TaxID,
		Name: identity.Name, SessionID: session.ID, SessionExpiresAt: session.ExpiresAt}}, nil
}

func validCustomerSecret(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == tokenSecretBytes
}
