package domain

import (
	"time"

	"github.com/google/uuid"
)

// ARCAEnvironment is the ARCA deployment a connection or an invoice belongs to. HOMOLOGATION is ARCA's
// test deployment: what it authorizes has no fiscal validity.
type ARCAEnvironment string

const (
	ARCAEnvironmentHomologation ARCAEnvironment = "HOMOLOGATION"
	ARCAEnvironmentProduction   ARCAEnvironment = "PRODUCTION"
)

// Valid reports whether e is one of the declared environments.
func (e ARCAEnvironment) Valid() bool {
	return e == ARCAEnvironmentHomologation || e == ARCAEnvironmentProduction
}

// ARCASetup is the credential an account connects to ARCA with: its CUIT, the certificate request,
// the sealed private key and, once ARCA issues it, the certificate.
type ARCASetup struct {
	ID                   uuid.UUID
	Environment          ARCAEnvironment // the deployment the certificate is requested for.
	TaxID                string
	CSR                  string
	SealedKey            string
	Certificate          string
	CertificateExpiresAt *time.Time
}

// UsableIn reports whether the setup can authorize invoices in env; one made for the other
// environment has to be connected again.
func (s ARCASetup) UsableIn(env ARCAEnvironment) bool { return s.Environment == env }

// ARCATicket holds the temporary credentials returned by WSAA for one environment.
type ARCATicket struct {
	Environment ARCAEnvironment
	Token       string
	Sign        string
	ExpiresAt   time.Time
}

// ARCAConnector creates an account's key and certificate request and checks the certificate ARCA
// issues for them.
type ARCAConnector interface {
	Generate(taxID string) (key, csr string, err error)
	ValidateCertificate(key, certificate, taxID string) (time.Time, error)
}
