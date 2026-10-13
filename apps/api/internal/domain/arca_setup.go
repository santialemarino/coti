package domain

import (
	"time"

	"github.com/google/uuid"
)

// ARCASetup is the credential an account connects to ARCA with: its CUIT, the certificate request,
// the sealed private key and, once ARCA issues it, the certificate.
type ARCASetup struct {
	ID                   uuid.UUID
	TaxID                string
	CSR                  string
	SealedKey            string
	Certificate          string
	CertificateExpiresAt *time.Time
}

// ARCATicket holds the temporary credentials returned by WSAA.
type ARCATicket struct {
	Token     string
	Sign      string
	ExpiresAt time.Time
}

// ARCAConnector creates an account's key and certificate request and checks the certificate ARCA
// issues for them.
type ARCAConnector interface {
	Generate(taxID string) (key, csr string, err error)
	ValidateCertificate(key, certificate, taxID string) (time.Time, error)
}
