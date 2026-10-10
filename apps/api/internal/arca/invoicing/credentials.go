package arca

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CertificateInfo describes an ARCA certificate as the account sees it.
type CertificateInfo struct {
	CUIT     string
	Subject  string
	NotAfter time.Time
}

// signer is a parsed certificate and the key that matches it.
type signer struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
	cuit string
}

// cacheKey identifies the certificate a ticket was issued to.
func (s *signer) cacheKey() string {
	sum := sha256.Sum256(s.cert.Raw)
	return fmt.Sprintf("%x", sum)
}

var oidSerialNumber = asn1.ObjectIdentifier{2, 5, 4, 5}

// ParseCredentials validates a certificate and key pair and reads the CUIT the certificate
// represents.
func ParseCredentials(certPEM, keyPEM []byte) (*CertificateInfo, error) {
	s, err := parseSigner(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &CertificateInfo{CUIT: s.cuit, Subject: s.cert.Subject.String(), NotAfter: s.cert.NotAfter}, nil
}

func parseSigner(certPEM, keyPEM []byte) (*signer, error) {
	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, invalidCredentials(err)
	}
	key, err := parsePrivateKey(keyPEM)
	if err != nil {
		return nil, invalidCredentials(err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, invalidCredentials(errors.New("certificate key is not RSA"))
	}
	if !pub.Equal(&key.PublicKey) {
		return nil, invalidCredentials(errors.New("private key does not match the certificate"))
	}
	cuit, err := certificateCUIT(cert)
	if err != nil {
		return nil, invalidCredentials(err)
	}
	return &signer{cert: cert, key: key, cuit: cuit}, nil
}

func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate is not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	return cert, nil
}

func parsePrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return key, nil
}

// certificateCUIT reads the subject serialNumber ARCA writes as "CUIT 20123456789".
func certificateCUIT(cert *x509.Certificate) (string, error) {
	for _, name := range cert.Subject.Names {
		if !name.Type.Equal(oidSerialNumber) {
			continue
		}
		value, _ := name.Value.(string)
		digits, found := strings.CutPrefix(strings.TrimSpace(value), "CUIT ")
		digits = strings.TrimSpace(digits)
		if found && len(digits) == 11 && strings.Trim(digits, "0123456789") == "" {
			return digits, nil
		}
	}
	return "", errors.New("certificate subject carries no CUIT serialNumber")
}

func invalidCredentials(err error) error {
	return fmt.Errorf("arca credentials: %v: %w", err, domain.WithCode(domain.CodeARCACredentials, domain.ErrInvalidInput))
}
