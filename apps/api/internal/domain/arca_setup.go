package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ARCASetup holds one account's homologation identity and one branch's test point of sale.
type ARCASetup struct {
	ID                   uuid.UUID
	TaxID                string
	CSR                  string
	SealedKey            string
	Certificate          string
	SealedTicket         string
	CertificateExpiresAt *time.Time
	PointOfSale          int
	VerifiedAt           *time.Time
}

// ARCATicket holds the temporary credentials returned by WSAA.
type ARCATicket struct {
	Token     string
	Sign      string
	ExpiresAt time.Time
}

// ARCAConnectionResult reports a read-only connection check.
type ARCAConnectionResult struct {
	Ticket     ARCATicket
	LastNumber int64
	Failure    string
}

// ARCAConnector owns certificate creation and read-only homologation calls.
type ARCAConnector interface {
	Generate(taxID string) (key, csr string, err error)
	ValidateCertificate(key, certificate, taxID string) (time.Time, error)
	Verify(ctx context.Context, key, certificate, taxID string, pointOfSale int, ticket ARCATicket) (ARCAConnectionResult, error)
}
