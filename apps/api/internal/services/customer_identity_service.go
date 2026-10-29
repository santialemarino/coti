package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type customerCredentialStore interface {
	GetHostnameByDeliveryTokenCrossAccount(context.Context, repository.Querier, string) (string, error)
	GetIdentityByGoogleSubject(context.Context, repository.Querier, uuid.UUID, string) (*domain.CustomerIdentity, error)
	ConsumeChallenge(context.Context, repository.Querier, uuid.UUID, string, domain.CustomerChallengePurpose, time.Time) (*domain.CustomerChallenge, error)
	CreateGoogleIdentity(context.Context, repository.Querier, domain.CustomerIdentity) (*domain.CustomerIdentity, error)
	CreateChallenge(context.Context, repository.Querier, domain.CustomerChallenge) error
	LinkGoogle(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error
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
	Subject       string `json:"subject,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
}

type customerContactProver interface {
	ProveRegisteredContact(context.Context, repository.Querier, domain.CustomerPrincipal, domain.ChannelType, string) error
}

// WithIdentityProviders enables Google sign-in and credential recovery through explicit adapter wiring.
func (s *CustomerAuthService) WithIdentityProviders(store customerCredentialStore, google domain.CustomerGoogleProvider) *CustomerAuthService {
	s.credentials = store
	s.google = google
	return s
}

// WithContactProof lets a Google-verified address the customer chose count as a verified contact.
func (s *CustomerAuthService) WithContactProof(prover customerContactProver) *CustomerAuthService {
	s.prover = prover
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
		payload, _ := json.Marshal(customerChallengePayload{Subject: google.Subject, Email: google.Email, EmailVerified: google.EmailVerified, Name: google.Name})
		err = s.credentials.CreateChallenge(ctx, q, domain.CustomerChallenge{ID: uuid.New(), AccountID: site.AccountID, Purpose: "GOOGLE_SIGNUP", TokenHash: hashToken(secret), Payload: payload, ExpiresAt: s.now().Add(s.cfg.SignupTTL)})
		result.SignupToken = secret
		if email, ok := googleContactEmail(google.Email, google.EmailVerified); ok {
			result.SignupEmail = email
		}
		return err
	})
	return result, err
}

// CompleteGoogle binds a new CUIT/CUIL or explicitly links Google to an already authenticated customer.
// A new registration needs a name and at least one contact; the Google address is one only when the
// customer chose it, and then it is verified because Google verified it.
func (s *CustomerAuthService) CompleteGoogle(ctx context.Context, hostname string, in domain.CustomerGoogleCompletion, principal *domain.CustomerPrincipal) (*domain.CustomerLogin, error) {
	if s.credentials == nil {
		return nil, domain.ErrNotConfigured
	}
	if !validCustomerSecret(in.Token) {
		return nil, domain.ErrUnauthenticated
	}
	taxID := domain.NormalizeTaxID(in.TaxID)
	if !domain.ValidCUIT(taxID) {
		return nil, domain.ErrInvalidInput
	}
	contacts, err := domain.NormalizeClientContacts(in.Contacts)
	if err != nil {
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
		token, err := s.credentials.ConsumeChallenge(ctx, q, site.AccountID, hashToken(in.Token), "GOOGLE_SIGNUP", s.now())
		if err != nil {
			return err
		}
		var payload customerChallengePayload
		if err = json.Unmarshal(token.Payload, &payload); err != nil {
			return err
		}
		if principal != nil {
			identity, err := s.store.GetIdentityForUpdate(ctx, q, site.AccountID, principal.CustomerID)
			if err != nil {
				return err
			}
			if identity.DisabledAt != nil {
				return domain.ErrUnauthenticated
			}
			if err = s.credentials.LinkGoogle(ctx, q, site.AccountID, principal.CustomerID, payload.Subject); err != nil {
				return err
			}
			login, err = s.createSession(ctx, q, *identity, secret)
			return err
		}
		if name := strings.TrimSpace(in.Name); name != "" {
			payload.Name = name
		}
		if s.profiles == nil || payload.Name == "" || len(payload.Name) > 255 {
			return domain.ErrInvalidInput
		}
		google, chosen := "", false
		if in.UseGoogleEmail {
			if google, chosen = googleContactEmail(payload.Email, payload.EmailVerified); !chosen {
				return domain.ErrInvalidInput
			}
			contacts = withGoogleContact(contacts, google)
		}
		if len(contacts) == 0 {
			return domain.ErrInvalidInput
		}
		identity, err := s.credentials.CreateGoogleIdentity(ctx, q, domain.CustomerIdentity{ID: uuid.New(), AccountID: site.AccountID, TaxID: taxID, GoogleSubject: &payload.Subject, Name: &payload.Name})
		if err != nil {
			return err
		}
		registered := domain.CustomerPrincipal{CustomerID: identity.ID, AccountID: site.AccountID, TaxID: taxID}
		if err := s.profiles.RegisterProfile(ctx, q, registered, domain.ClientContactUpdate{Name: payload.Name, Contacts: contacts}); err != nil {
			return err
		}
		if chosen {
			if s.prover == nil {
				return domain.ErrNotConfigured
			}
			if err := s.prover.ProveRegisteredContact(ctx, q, registered, domain.ChannelTypeEmail, google); err != nil {
				return err
			}
		}
		login, err = s.createSession(ctx, q, *identity, secret)
		return err
	})
	return login, err
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

// googleContactEmail returns the normalized Google address only when Google verified it.
func googleContactEmail(email string, verified bool) (string, bool) {
	if !verified || !validCustomerEmail(email) {
		return "", false
	}
	normalized, err := domain.NormalizeClientContact(domain.ChannelTypeEmail, email)
	return normalized, err == nil
}

// withGoogleContact adds the chosen Google address, primary only when no other email is.
func withGoogleContact(contacts []domain.ClientContact, email string) []domain.ClientContact {
	primary := true
	for _, contact := range contacts {
		if contact.Kind == domain.ChannelTypeEmail {
			if contact.Value == email {
				return contacts
			}
			primary = false
		}
	}
	return append(contacts, domain.ClientContact{Kind: domain.ChannelTypeEmail, Value: email, Primary: primary})
}
