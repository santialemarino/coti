package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CustomerIdentity is a supplier-scoped credential, independent of commercial client profiles.
type CustomerIdentity struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	TaxID          string
	PasswordHash   string
	GoogleSubject  *string
	Name           *string
	FailedAttempts int
	LockedUntil    *time.Time
	DisabledAt     *time.Time
	CreatedAt      time.Time
}

// CustomerPrincipal is an authenticated customer's scope, never a staff tenant or a client ID.
type CustomerPrincipal struct {
	CustomerID       uuid.UUID
	AccountID        uuid.UUID
	TaxID            string
	Name             *string
	SessionID        uuid.UUID
	SessionExpiresAt time.Time
}

// CustomerSession is an opaque credential stored only as a hash with an absolute expiration.
type CustomerSession struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	CustomerID uuid.UUID
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

// CustomerRegistration creates credentials without claiming an existing commercial profile.
type CustomerRegistration struct {
	Name     string
	Contacts []ClientContact
	TaxID    string
	Password string
}

// CustomerCredentials is a CUIT and password login scoped to the supplier's hostname.
type CustomerCredentials struct {
	TaxID    string
	Password string
}

// CustomerLogin returns the one-time raw session value to the trusted webapp server.
type CustomerLogin struct {
	SessionToken string
	Principal    CustomerPrincipal
}

// CustomerSite identifies the supplier before establishing a customer transaction scope.
type CustomerSite struct {
	LogoURL      *string
	AccountID    uuid.UUID
	Hostname     string
	SupplierName string
}

// CustomerGoogleIdentity is the verified stable subject returned by Google's OpenID provider.
type CustomerGoogleIdentity struct {
	Subject string
	Email   string
	Name    string
	// EmailVerified carries Google's email_verified claim; only a verified address may become a contact.
	EmailVerified bool
	// HostedDomain carries the hd claim, the Google Workspace domain the account belongs to.
	HostedDomain string
}

// CustomerGoogleProvider exchanges a single-use authorization code and verifies its ID token.
type CustomerGoogleProvider interface {
	Exchange(context.Context, string, string, string, string) (*CustomerGoogleIdentity, error)
}

// CustomerChallenge is a hashed single-use capability scoped to a supplier and purpose.
type CustomerChallenge struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	CustomerID *uuid.UUID
	Purpose    CustomerChallengePurpose
	TokenHash  string
	Payload    json.RawMessage
	ExpiresAt  time.Time
}

// CustomerGoogleResult either starts a known customer's session or requires a first CUIT/CUIL.
type CustomerGoogleResult struct {
	Login       *CustomerLogin
	SignupToken string
	// SignupEmail is the Google address the customer may choose as a contact; empty when Google did not verify it.
	SignupEmail string
	// SignupEmailProven is true when Google proves current control of SignupEmail, so choosing it
	// verifies it; otherwise it joins pending a code.
	SignupEmailProven bool
}

// CustomerGoogleCompletion is a first Google registration: its own contacts, and the Google address
// only when the customer chose it.
type CustomerGoogleCompletion struct {
	Token          string
	TaxID          string
	Name           string
	Contacts       []ClientContact
	UseGoogleEmail bool
}

// CustomerAuthOptions is the public supplier identity and customer credential configuration.
type CustomerAuthOptions struct {
	ClientID          string
	RedirectURI       string
	PasswordMinLength int
	SupplierName      string
	LogoURL           *string
}

// CustomerChallengePurpose scopes one-use capabilities to a single credential operation.
type CustomerChallengePurpose string

const CustomerChallengeGoogleSignup CustomerChallengePurpose = "GOOGLE_SIGNUP"
