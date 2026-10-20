package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type customerCredentialStore interface {
	GetHostnameByDeliveryTokenCrossAccount(context.Context, repository.Querier, string) (string, error)
	GetIdentityByGoogleSubject(context.Context, repository.Querier, uuid.UUID, string) (*domain.CustomerIdentity, error)
	GetVerifiedRecoveryIdentity(context.Context, repository.Querier, uuid.UUID, string, string) (*domain.CustomerIdentity, error)
	ConsumeChallenge(context.Context, repository.Querier, uuid.UUID, string, domain.CustomerChallengePurpose, time.Time) (*domain.CustomerChallenge, error)
	CreateGoogleIdentity(context.Context, repository.Querier, domain.CustomerIdentity) (*domain.CustomerIdentity, error)
	CreateChallenge(context.Context, repository.Querier, domain.CustomerChallenge) error
	LinkGoogle(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error
	VerifyRecoveryEmail(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string, time.Time) error
	ResetCustomerPassword(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string, time.Time) error
}

// GetDeliveryHostname routes legacy links without reading any private request or quotation data.
func (s *CustomerAuthService) GetDeliveryHostname(ctx context.Context, token string) (string, error) {
	if s.credentials == nil {
		return "", domain.ErrNotConfigured
	}
	if len(token) > 255 || token == "" {
		return "", domain.ErrNotFound
	}
	return s.credentials.GetHostnameByDeliveryTokenCrossAccount(ctx, s.db.CrossAccount(), token)
}

type customerChallengePayload struct {
	Subject string `json:"subject,omitempty"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
}

// WithIdentityProviders enables Google and recovery mail through explicit adapter wiring.
func (s *CustomerAuthService) WithIdentityProviders(store customerCredentialStore, google domain.CustomerGoogleProvider, mail mailSender) *CustomerAuthService {
	s.credentials = store
	s.google = google
	s.mail = mail
	return s
}

// GoogleOptions returns public provider configuration bound to an existing supplier hostname.
func (s *CustomerAuthService) GoogleOptions(ctx context.Context, hostname string) (*domain.CustomerAuthOptions, error) {
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	return &domain.CustomerAuthOptions{ClientID: s.cfg.GoogleClientID, RedirectURI: s.customerOrigin(*site) + "/api/customer/google/callback", PasswordMinLength: s.cfg.PasswordMinLength, SupplierName: site.SupplierName, LogoURL: site.LogoURL}, nil
}

// GoogleExchange authenticates a linked Google subject or issues a short-lived first-registration capability.
func (s *CustomerAuthService) GoogleExchange(ctx context.Context, hostname, code, verifier, nonce string) (*domain.CustomerGoogleResult, error) {
	if s.google == nil || s.credentials == nil {
		return nil, domain.ErrNotConfigured
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if code == "" || len(code) > 4096 || len(verifier) < 43 || len(verifier) > 128 || !validCustomerSecret(nonce) {
		return nil, domain.ErrInvalidInput
	}
	google, err := s.google.Exchange(ctx, code, verifier, s.customerOrigin(*site)+"/api/customer/google/callback", nonce)
	if err != nil {
		return nil, err
	}
	secret, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	result := &domain.CustomerGoogleResult{}
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		identity, err := s.credentials.GetIdentityByGoogleSubject(ctx, q, site.AccountID, google.Subject)
		if err == nil {
			current, err := s.store.GetIdentityForUpdate(ctx, q, site.AccountID, identity.ID)
			if err != nil {
				return err
			}
			if current.DisabledAt != nil {
				return domain.ErrUnauthenticated
			}
			result.Login, err = s.createSession(ctx, q, *current, secret)
			return err
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		payload, _ := json.Marshal(customerChallengePayload{Subject: google.Subject, Email: google.Email, Name: google.Name})
		err = s.credentials.CreateChallenge(ctx, q, domain.CustomerChallenge{ID: uuid.New(), AccountID: site.AccountID, Purpose: "GOOGLE_SIGNUP", TokenHash: hashToken(secret), Payload: payload, ExpiresAt: s.now().Add(s.cfg.SignupTTL)})
		result.SignupToken = secret
		return err
	})
	return result, err
}

// CompleteGoogle binds a new CUIT/CUIL or explicitly links Google to an already authenticated customer.
func (s *CustomerAuthService) CompleteGoogle(ctx context.Context, hostname, raw, taxID string, principal *domain.CustomerPrincipal) (*domain.CustomerLogin, error) {
	if s.credentials == nil {
		return nil, domain.ErrNotConfigured
	}
	if !validCustomerSecret(raw) {
		return nil, domain.ErrUnauthenticated
	}
	taxID = domain.NormalizeTaxID(taxID)
	if !domain.ValidCUIT(taxID) {
		return nil, domain.ErrInvalidInput
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if principal != nil && (principal.AccountID != site.AccountID || principal.TaxID != taxID) {
		return nil, domain.ErrForbidden
	}
	secret, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	var login *domain.CustomerLogin
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		token, err := s.credentials.ConsumeChallenge(ctx, q, site.AccountID, hashToken(raw), "GOOGLE_SIGNUP", s.now())
		if err != nil {
			return err
		}
		var payload customerChallengePayload
		if err = json.Unmarshal(token.Payload, &payload); err != nil {
			return err
		}
		var identity *domain.CustomerIdentity
		if principal == nil {
			var name *string
			if len(payload.Name) <= 255 && payload.Name != "" {
				name = &payload.Name
			}
			identity, err = s.credentials.CreateGoogleIdentity(ctx, q, domain.CustomerIdentity{ID: uuid.New(), AccountID: site.AccountID, TaxID: taxID, GoogleSubject: &payload.Subject, Name: name})
		} else {
			identity, err = s.store.GetIdentityForUpdate(ctx, q, site.AccountID, principal.CustomerID)
			if err != nil {
				return err
			}
			if identity.DisabledAt != nil {
				return domain.ErrUnauthenticated
			}
			err = s.credentials.LinkGoogle(ctx, q, site.AccountID, principal.CustomerID, payload.Subject)
		}
		if err != nil {
			return err
		}
		login, err = s.createSession(ctx, q, *identity, secret)
		return err
	})
	return login, err
}

// ForgotCustomerPassword sends only to the matching verified recovery address and conceals account existence.
func (s *CustomerAuthService) ForgotCustomerPassword(ctx context.Context, hostname, taxID, email string) error {
	if s.credentials == nil || s.mail == nil {
		return domain.ErrNotConfigured
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return err
	}
	taxID = domain.NormalizeTaxID(taxID)
	email = domain.NormalizeEmail(email)
	if !domain.ValidCUIT(taxID) || !validCustomerEmail(email) {
		return nil
	}
	var identity *domain.CustomerIdentity
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		var err error
		identity, err = s.credentials.GetVerifiedRecoveryIdentity(ctx, q, site.AccountID, taxID, email)
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.sendCustomerChallenge(ctx, *site, *identity, "RESET_PASSWORD", email)
}

// VerifyCustomerEmail enables recovery only after the one-use link confirms the saved address.
func (s *CustomerAuthService) VerifyCustomerEmail(ctx context.Context, hostname, raw string) error {
	return s.redeemCustomerChallenge(ctx, hostname, raw, "VERIFY_EMAIL", func(q repository.Querier, token domain.CustomerChallenge, payload customerChallengePayload) error {
		if token.CustomerID == nil {
			return domain.ErrUnauthenticated
		}
		return s.credentials.VerifyRecoveryEmail(ctx, q, token.AccountID, *token.CustomerID, payload.Email, s.now())
	})
}

// ResetCustomerPassword consumes one recovery capability and revokes every previous session.
func (s *CustomerAuthService) ResetCustomerPassword(ctx context.Context, hostname, raw, password string) error {
	if err := (domain.PasswordPolicy{MinLength: s.cfg.PasswordMinLength}).Validate(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.redeemCustomerChallenge(ctx, hostname, raw, "RESET_PASSWORD", func(q repository.Querier, token domain.CustomerChallenge, _ customerChallengePayload) error {
		if token.CustomerID == nil {
			return domain.ErrUnauthenticated
		}
		return s.credentials.ResetCustomerPassword(ctx, q, token.AccountID, *token.CustomerID, hash, s.now())
	})
}

func (s *CustomerAuthService) redeemCustomerChallenge(ctx context.Context, hostname, raw string, purpose domain.CustomerChallengePurpose, apply func(repository.Querier, domain.CustomerChallenge, customerChallengePayload) error) error {
	if s.credentials == nil {
		return domain.ErrNotConfigured
	}
	if !validCustomerSecret(raw) {
		return domain.ErrUnauthenticated
	}
	site, err := s.resolveSite(ctx, hostname)
	if err != nil {
		return err
	}
	return s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		token, err := s.credentials.ConsumeChallenge(ctx, q, site.AccountID, hashToken(raw), purpose, s.now())
		if err != nil {
			return err
		}
		var payload customerChallengePayload
		if err = json.Unmarshal(token.Payload, &payload); err != nil {
			return err
		}
		return apply(q, *token, payload)
	})
}

func (s *CustomerAuthService) sendCustomerChallenge(ctx context.Context, site domain.CustomerSite, identity domain.CustomerIdentity, purpose domain.CustomerChallengePurpose, email string) error {
	secret, err := s.newSecret()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(customerChallengePayload{Email: email})
	ttl, path, event := s.cfg.RecoveryTTL, "/reset-password", domain.NotificationEventPasswordReset
	subject, heading, label := "Restablecé tu contraseña", "Restablecé tu contraseña", "Elegir contraseña"
	if purpose == "VERIFY_EMAIL" {
		ttl, path, event = s.cfg.VerificationTTL, "/verify-email", domain.NotificationEventEmailVerification
		subject, heading, label = "Confirmá tu email de recuperación", "Confirmá tu email de recuperación", "Confirmar email"
	}
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: site.AccountID}, func(q repository.Querier) error {
		return s.credentials.CreateChallenge(ctx, q, domain.CustomerChallenge{ID: uuid.New(), AccountID: site.AccountID, CustomerID: &identity.ID, Purpose: purpose, TokenHash: hashToken(secret), Payload: payload, ExpiresAt: s.now().Add(ttl)})
	})
	if err != nil {
		return err
	}
	// Delivery outcomes never disclose whether a recovery address matches an account.
	_ = s.mail.Send(ctx, OutboundMail{AccountID: site.AccountID, Event: event, To: email, Subject: subject, Heading: heading, ActionLabel: label, ActionURL: s.customerOrigin(site) + path + "?token=" + url.QueryEscape(secret)})
	return nil
}

func (s *CustomerAuthService) customerOrigin(site domain.CustomerSite) string {
	if s.cfg.GoogleLocalOrigin != "" {
		return strings.TrimRight(s.cfg.GoogleLocalOrigin, "/")
	}
	return "https://" + site.Hostname
}

func validCustomerEmail(value string) bool {
	a, err := mail.ParseAddress(value)
	return err == nil && a.Address == value && len(value) <= 255
}
