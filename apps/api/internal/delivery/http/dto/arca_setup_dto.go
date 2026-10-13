package dto

import "time"

// ARCASetupResponse exposes the public setup material: the request, the installed certificate
// and their metadata. The private key and the access ticket never leave the server.
type ARCASetupResponse struct {
	Enabled              bool       `json:"enabled"`
	TaxID                string     `json:"tax_id"`
	CSR                  string     `json:"csr"`
	HasCertificate       bool       `json:"has_certificate"`
	Certificate          string     `json:"certificate"`
	CertificateExpiresAt *time.Time `json:"certificate_expires_at"`
	PointOfSale          int        `json:"point_of_sale"`
	VerifiedAt           *time.Time `json:"verified_at"`
}

// UploadARCACertificateRequest carries the public PEM certificate from WSASS.
type UploadARCACertificateRequest struct {
	Certificate string `json:"certificate" binding:"required,max=16384"`
}

// VerifyARCASetupRequest selects the active branch's test point of sale.
type VerifyARCASetupRequest struct {
	PointOfSale int `json:"point_of_sale" binding:"required,min=1,max=99998"`
}

// ARCAConnectionResponse reports access without returning temporary credentials.
type ARCAConnectionResponse struct {
	Verified   bool   `json:"verified"`
	Failure    string `json:"failure"`
	LastNumber int64  `json:"last_number"`
}
