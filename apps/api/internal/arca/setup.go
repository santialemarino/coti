// Package arca implements the fiscal service's homologation setup protocol.
package arca

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/smallstep/pkcs7"
)

const wsfeNamespace = "http://ar.gov.afip.dif.FEV1/"
const wsaaNamespace = "http://wsaa.view.sua.dvadac.desein.afip.gov"
const wsaaHomologation = "https://wsaahomo.afip.gov.ar/ws/services/LoginCms"
const wsfeHomologation = "https://wswhomo.afip.gov.ar/wsfev1/service.asmx"
const responseLimit = 1024 * 1024

// SetupConnector creates credentials and checks access without issuing a voucher.
type SetupConnector struct {
	client  *http.Client
	now     func() time.Time
	wsaaURL string
	wsfeURL string
}

// NewSetupConnector builds a connector restricted to homologation.
func NewSetupConnector(timeout time.Duration) *SetupConnector {
	return &SetupConnector{client: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now, wsaaURL: wsaaHomologation, wsfeURL: wsfeHomologation}
}

// Generate creates a private key and a PKCS#10 request for the authorized CUIT.
func (c *SetupConnector) Generate(taxID string) (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "Coti homologation", Organization: []string{"Coti"}, Country: []string{"AR"}, SerialNumber: "CUIT " + taxID},
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

type ticketRequest struct {
	XMLName xml.Name     `xml:"loginTicketRequest"`
	Version string       `xml:"version,attr"`
	Header  ticketHeader `xml:"header"`
	Service string       `xml:"service"`
}
type ticketHeader struct {
	UniqueID       int64  `xml:"uniqueId"`
	GenerationTime string `xml:"generationTime"`
	ExpirationTime string `xml:"expirationTime"`
}

func (c *SetupConnector) login(ctx context.Context, keyPEM, certificate string) (domain.ARCATicket, string) {
	key, cert, err := readCredentials(keyPEM, certificate)
	if err != nil {
		return domain.ARCATicket{}, "CERTIFICATE_INVALID"
	}
	now := c.now()
	tra, err := xml.Marshal(ticketRequest{Version: "1.0", Header: ticketHeader{now.Unix(), now.Add(-5 * time.Minute).Format(time.RFC3339), now.Add(5 * time.Minute).Format(time.RFC3339)}, Service: "wsfe"})
	if err != nil {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	signed, err := pkcs7.NewSignedData(append([]byte(xml.Header), tra...))
	if err != nil {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	signed.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err = signed.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
		return domain.ARCATicket{}, "CERTIFICATE_INVALID"
	}
	der, err := signed.Finish()
	if err != nil {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	body := `<loginCms xmlns="` + wsaaNamespace + `"><in0>` + base64.StdEncoding.EncodeToString(der) + `</in0></loginCms>`
	raw, failure := c.call(ctx, c.wsaaURL, "", body)
	if failure != "" {
		return domain.ARCATicket{}, failure
	}
	var envelope struct {
		Body struct {
			Return string `xml:"loginCmsResponse>loginCmsReturn"`
		} `xml:"Body"`
	}
	if xml.Unmarshal(raw, &envelope) != nil {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	var response struct {
		Expires string `xml:"header>expirationTime"`
		Token   string `xml:"credentials>token"`
		Sign    string `xml:"credentials>sign"`
	}
	if xml.Unmarshal([]byte(envelope.Body.Return), &response) != nil {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	expires, err := time.Parse(time.RFC3339, response.Expires)
	if err != nil || !expires.After(now) || response.Token == "" || response.Sign == "" {
		return domain.ARCATicket{}, "UNAVAILABLE"
	}
	return domain.ARCATicket{Token: response.Token, Sign: response.Sign, ExpiresAt: expires}, ""
}

// Verify authenticates and reads the last authorized test invoice for the point of sale.
func (c *SetupConnector) Verify(ctx context.Context, key, certificate, taxID string, pointOfSale int, ticket domain.ARCATicket) (domain.ARCAConnectionResult, error) {
	if _, err := c.ValidateCertificate(key, certificate, taxID); err != nil {
		return domain.ARCAConnectionResult{Failure: "CERTIFICATE_INVALID"}, nil
	}
	if !ticket.ExpiresAt.After(c.now().Add(time.Minute)) {
		var failure string
		ticket, failure = c.login(ctx, key, certificate)
		if failure != "" {
			return domain.ARCAConnectionResult{Failure: failure}, nil
		}
	}
	var auth bytes.Buffer
	_ = xml.EscapeText(&auth, []byte(ticket.Token))
	token := auth.String()
	auth.Reset()
	_ = xml.EscapeText(&auth, []byte(ticket.Sign))
	body := fmt.Sprintf(`<FECompUltimoAutorizado xmlns="%s"><Auth><Token>%s</Token><Sign>%s</Sign><Cuit>%s</Cuit></Auth><PtoVta>%d</PtoVta><CbteTipo>6</CbteTipo></FECompUltimoAutorizado>`, wsfeNamespace, token, auth.String(), taxID, pointOfSale)
	raw, failure := c.call(ctx, c.wsfeURL, wsfeNamespace+"FECompUltimoAutorizado", body)
	result := domain.ARCAConnectionResult{Ticket: ticket, Failure: failure}
	if failure != "" {
		return result, nil
	}
	var envelope struct {
		Body struct {
			Result struct {
				Number *int64 `xml:"CbteNro"`
				Errors []struct {
					Code int `xml:"Code"`
				} `xml:"Errors>Err"`
			} `xml:"FECompUltimoAutorizadoResponse>FECompUltimoAutorizadoResult"`
		} `xml:"Body"`
	}
	if xml.Unmarshal(raw, &envelope) != nil {
		result.Failure = "UNAVAILABLE"
		return result, nil
	}
	for _, e := range envelope.Body.Result.Errors {
		switch e.Code {
		case 10005, 11002:
			result.Failure = "POINT_OF_SALE_REJECTED"
		default:
			result.Failure = "WSFE_REJECTED"
		}
		return result, nil
	}
	if envelope.Body.Result.Number == nil {
		result.Failure = "UNAVAILABLE"
		return result, nil
	}
	result.LastNumber = *envelope.Body.Result.Number
	return result, nil
}

func (c *SetupConnector) call(ctx context.Context, endpoint, action, body string) ([]byte, string) {
	envelope := `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` + body + `</soap:Body></soap:Envelope>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(envelope))
	if err != nil {
		return nil, "UNAVAILABLE"
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+action+`"`)
	response, err := c.client.Do(req)
	if err != nil {
		return nil, "UNAVAILABLE"
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil || len(raw) > responseLimit {
		return nil, "UNAVAILABLE"
	}
	var fault struct {
		Body struct {
			Fault *struct {
				Code string `xml:"faultcode"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if xml.Unmarshal(raw, &fault) == nil && fault.Body.Fault != nil {
		code := fault.Body.Fault.Code
		if strings.Contains(code, "alreadyAuthenticated") {
			return nil, "TICKET_ACTIVE"
		}
		if strings.Contains(code, "cms.") || strings.Contains(code, "notAuthorized") {
			return nil, "WSAA_REJECTED"
		}
		return nil, "UNAVAILABLE"
	}
	if response.StatusCode != http.StatusOK {
		return nil, "UNAVAILABLE"
	}
	return raw, ""
}
