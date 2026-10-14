package dto

import "time"

// ARCASetupResponse exposes the public setup material: the request, the installed certificate
// and their metadata. The private key never leaves the server.
type ARCASetupResponse struct {
	Enabled bool `json:"enabled"`
	// Environment is where this server authorizes invoices: HOMOLOGATION (test mode) or PRODUCTION.
	Environment string `json:"environment" enums:"HOMOLOGATION,PRODUCTION"`
	// SetupEnvironment is the environment the connection was made for; null without one.
	SetupEnvironment *string `json:"setup_environment" enums:"HOMOLOGATION,PRODUCTION"`
	// ReconnectRequired is set when the connection was made for the other environment and has to
	// be started over before it can invoice.
	ReconnectRequired    bool       `json:"reconnect_required"`
	TaxID                string     `json:"tax_id"`
	CSR                  string     `json:"csr"`
	HasCertificate       bool       `json:"has_certificate"`
	Certificate          string     `json:"certificate"`
	CertificateExpiresAt *time.Time `json:"certificate_expires_at"`
}

// UploadARCACertificateRequest carries the public PEM certificate from WSASS.
type UploadARCACertificateRequest struct {
	Certificate string `json:"certificate" binding:"required,max=16384"`
}
