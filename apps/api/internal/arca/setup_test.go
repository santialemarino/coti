package arca

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/smallstep/pkcs7"
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
	c := NewSetupConnector(time.Second)
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

func TestSetupConnector_VerifySignsAndUsesReadOnlySOAP(t *testing.T) {
	t.Parallel()
	c := NewSetupConnector(time.Second)
	key, cert := setupCredentials(t, c, "20329642330")
	logins, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "text/xml; charset=utf-8" {
			t.Error("wrong HTTP request")
		}
		if r.URL.Path == "/wsaa" {
			logins++
			var req struct {
				In0 string `xml:"Body>loginCms>in0"`
			}
			if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			der, err := base64.StdEncoding.DecodeString(req.In0)
			if err != nil {
				t.Error(err)
			}
			cms, err := pkcs7.Parse(der)
			if err != nil {
				t.Error(err)
				return
			}
			if err := cms.Verify(); err != nil {
				t.Error(err)
			}
			if !strings.Contains(string(cms.Content), "<service>wsfe</service>") {
				t.Error("wrong WSAA service")
			}
			fmt.Fprintf(w, `<Envelope><Body><loginCmsResponse><loginCmsReturn>&lt;loginTicketResponse&gt;&lt;header&gt;&lt;expirationTime&gt;%s&lt;/expirationTime&gt;&lt;/header&gt;&lt;credentials&gt;&lt;token&gt;test-token&lt;/token&gt;&lt;sign&gt;test-sign&lt;/sign&gt;&lt;/credentials&gt;&lt;/loginTicketResponse&gt;</loginCmsReturn></loginCmsResponse></Body></Envelope>`, time.Now().Add(time.Hour).Format(time.RFC3339))
			return
		}
		reads++
		if r.Header.Get("SOAPAction") != `"`+wsfeNamespace+`FECompUltimoAutorizado"` {
			t.Error("unexpected operation; setup must not authorize invoices")
		}
		var req struct {
			Auth  struct{ Token, Sign, Cuit string } `xml:"Body>FECompUltimoAutorizado>Auth"`
			Point int                                `xml:"Body>FECompUltimoAutorizado>PtoVta"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Auth.Token != "test-token" || req.Auth.Sign != "test-sign" || req.Auth.Cuit != "20329642330" || req.Point != 3 {
			t.Errorf("unexpected request %+v", req)
		}
		fmt.Fprint(w, `<Envelope><Body><FECompUltimoAutorizadoResponse><FECompUltimoAutorizadoResult><CbteNro>17</CbteNro></FECompUltimoAutorizadoResult></FECompUltimoAutorizadoResponse></Body></Envelope>`)
	}))
	defer server.Close()
	c.wsaaURL, c.wsfeURL = server.URL+"/wsaa", server.URL+"/wsfe"
	result, err := c.Verify(context.Background(), key, cert, "20329642330", 3, domain.ARCATicket{})
	if err != nil || result.Failure != "" || result.LastNumber != 17 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	_, err = c.Verify(context.Background(), key, cert, "20329642330", 3, result.Ticket)
	if err != nil || logins != 1 || reads != 2 {
		t.Fatalf("logins=%d reads=%d error=%v", logins, reads, err)
	}
}

func TestSetupConnector_VerifyRejectsFiscalErrorsAndMalformedResponses(t *testing.T) {
	t.Parallel()
	c := NewSetupConnector(time.Second)
	key, cert := setupCredentials(t, c, "20329642330")
	for _, tc := range []struct{ name, body, want string }{
		{"point rejected", `<Envelope><Body><FECompUltimoAutorizadoResponse><FECompUltimoAutorizadoResult><CbteNro>0</CbteNro><Errors><Err><Code>11002</Code></Err></Errors></FECompUltimoAutorizadoResult></FECompUltimoAutorizadoResponse></Body></Envelope>`, "POINT_OF_SALE_REJECTED"},
		{"empty success", `<Envelope><Body/></Envelope>`, "UNAVAILABLE"},
		{"malformed", `<broken`, "UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c.wsfeURL = server.URL
			result, err := c.Verify(context.Background(), key, cert, "20329642330", 1, domain.ARCATicket{Token: "t", Sign: "s", ExpiresAt: time.Now().Add(time.Hour)})
			if err != nil || result.Failure != tc.want {
				t.Fatalf("failure=%s error=%v", result.Failure, err)
			}
		})
	}
}
