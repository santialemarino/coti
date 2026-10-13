package arca

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func setupCredentials(t *testing.T, c *SetupConnector, taxID string) (string, string) {
	t.Helper()
	keyPEM, csrPEM, err := c.Generate(taxID)
	if err != nil {
		t.Fatal(err)
	}
	k, _ := pem.Decode([]byte(keyPEM))
	key, err := x509.ParsePKCS1PrivateKey(k.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pem.Decode([]byte(csrPEM))
	csr, err := x509.ParseCertificateRequest(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: csr.Subject, NotBefore: c.now().Add(-time.Hour), NotAfter: c.now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return keyPEM, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestSetupConnector_ValidateCertificate(t *testing.T) {
	t.Parallel()
	c := NewSetupConnector()
	key, cert := setupCredentials(t, c, "20329642330")
	otherKey, _ := setupCredentials(t, c, "20329642330")
	for _, tc := range []struct {
		name, key, cert, cuit string
		valid                 bool
	}{
		{"matching", key, cert, "20329642330", true},
		{"wrong key", otherKey, cert, "20329642330", false},
		{"wrong CUIT", key, cert, "30710955057", false},
		{"not a certificate", key, key, "20329642330", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.ValidateCertificate(tc.key, tc.cert, tc.cuit)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := c.ValidateCertificate(key, cert, "20329642330"); err == nil {
		t.Fatal("accepted expired certificate")
	}
}
