package dto

import "time"

// ARCASetupResponse exposes the public setup material: the request, the installed certificate
// and their metadata. The private key never leaves the server.
type ARCASetupResponse struct {
	Enabled              bool       `json:"enabled"`
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
