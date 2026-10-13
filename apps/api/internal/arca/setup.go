// Package arca creates the credentials an account connects to ARCA with.
package arca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SetupConnector creates an account's key and certificate request and checks the certificate ARCA
// issues for it.
type SetupConnector struct {
	now func() time.Time
}

// NewSetupConnector builds a SetupConnector.
func NewSetupConnector() *SetupConnector {
	return &SetupConnector{now: time.Now}
}

// Generate creates a private key and a PKCS#10 request for the authorized CUIT.
func (c *SetupConnector) Generate(taxID string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "Coti", Organization: []string{"Coti"}, Country: []string{"AR"}, SerialNumber: "CUIT " + taxID},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}, key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})), nil
}

func readCredentials(keyPEM, certificate string) (*rsa.PrivateKey, *x509.Certificate, error) {
	k, _ := pem.Decode([]byte(keyPEM))
	cert, _ := pem.Decode([]byte(certificate))
	if k == nil || cert == nil || cert.Type != "CERTIFICATE" {
		return nil, nil, domain.ErrInvalidInput
	}
	key, err := x509.ParsePKCS1PrivateKey(k.Bytes)
	if err != nil {
		return nil, nil, domain.ErrInvalidInput
	}
	x, err := x509.ParseCertificate(cert.Bytes)
	if err != nil {
		return nil, nil, domain.ErrInvalidInput
	}
	public, ok := x.PublicKey.(*rsa.PublicKey)
	if !ok || public.E != key.E || public.N.Cmp(key.N) != 0 {
		return nil, nil, domain.ErrInvalidInput
	}
	return key, x, nil
}

// ValidateCertificate checks key ownership, CUIT and validity before persistence.
func (c *SetupConnector) ValidateCertificate(key, certificate, taxID string) (time.Time, error) {
	_, cert, err := readCredentials(key, certificate)
	if err != nil {
		return time.Time{}, err
	}
	if cert.Subject.SerialNumber != "CUIT "+taxID || c.now().Before(cert.NotBefore) || !c.now().Before(cert.NotAfter) {
		return time.Time{}, domain.ErrInvalidInput
	}
	return cert.NotAfter, nil
}
