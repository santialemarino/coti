// Package arca issues electronic invoices through ARCA's WSAA and WSFEv1 web services.
package arca

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// Environment names ARCA's two deployments.
const (
	EnvironmentHomologation = "homologation"
	EnvironmentProduction   = "production"
)

const (
	soapNamespace = "http://schemas.xmlsoap.org/soap/envelope/"

	homologationWSAA = "https://wsaahomo.afip.gov.ar/ws/services/LoginCms"
	homologationWSFE = "https://wswhomo.afip.gov.ar/wsfev1/service.asmx"
	productionWSAA   = "https://wsaa.afip.gov.ar/ws/services/LoginCms"
	productionWSFE   = "https://servicios1.afip.gov.ar/wsfev1/service.asmx"
)

// maxResponseBytes bounds what is read from one SOAP response; ARCA's are a few kilobytes.
const maxResponseBytes = 1 << 20

// Settings configures an Issuer.
type Settings struct {
	Environment string // EnvironmentHomologation or EnvironmentProduction; anything else is homologation.
	Timeout     time.Duration
	Now         func() time.Time
	WSAAURL     string // overrides the environment's WSAA endpoint.
	WSFEURL     string // overrides the environment's WSFEv1 endpoint.
	Log         *slog.Logger
}

// Issuer implements domain.InvoiceIssuer against ARCA.
type Issuer struct {
	client  *http.Client
	cache   TicketCache
	now     func() time.Time
	wsaaURL string
	wsfeURL string
	log     *slog.Logger
	mu      sync.Mutex
	logins  map[string]*sync.Mutex // one per certificate, so a slow login holds back only its own account.
}

var _ domain.InvoiceIssuer = (*Issuer)(nil)

// NewIssuer builds an Issuer for the configured environment, caching tickets in cache.
func NewIssuer(settings Settings, cache TicketCache) *Issuer {
	wsaa, wsfe := homologationWSAA, homologationWSFE
	if settings.Environment == EnvironmentProduction {
		wsaa, wsfe = productionWSAA, productionWSFE
	}
	if settings.WSAAURL != "" {
		wsaa = settings.WSAAURL
	}
	if settings.WSFEURL != "" {
		wsfe = settings.WSFEURL
	}
	now := settings.Now
	if now == nil {
		now = time.Now
	}
	log := settings.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Issuer{
		client:  &http.Client{Timeout: settings.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		cache:   cache,
		now:     now,
		wsaaURL: wsaa,
		wsfeURL: wsfe,
		log:     log,
		logins:  map[string]*sync.Mutex{},
	}
}

// Issue authorizes one invoice, numbering it as the next after ARCA's last authorized one.
func (i *Issuer) Issue(ctx context.Context, creds domain.ARCACredentials, req domain.InvoiceRequest) (*domain.InvoiceAuthorization, error) {
	s, err := parseSigner(creds.CertificatePEM, creds.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	t, err := i.ticket(ctx, s, creds.Tickets)
	if err != nil {
		return nil, err
	}
	a := feAuth{Token: t.Token, Sign: t.Sign, Cuit: req.IssuerCUIT}
	for attempt := 0; ; attempt++ {
		last, err := i.lastAuthorized(ctx, a, req.Type, req.PointOfSale)
		if err != nil {
			return nil, i.dropRejectedTicket(ctx, s, creds.Tickets, err)
		}
		number := last + 1
		if req.Claim != nil {
			if err := req.Claim(ctx, number); err != nil {
				return nil, err
			}
		}
		resp, raw, err := i.solicitar(ctx, a, req, number)
		if err != nil {
			return i.recoverIssued(ctx, a, req, number, raw, err)
		}
		// 10016: another system took the number between our read and our request.
		if attempt == 0 && resp.hasCode(codeNotNextNumber) {
			continue
		}
		auth, err := resp.authorization(number, raw)
		if errors.Is(err, errTicketRejected) {
			return nil, i.dropRejectedTicket(ctx, s, creds.Tickets, err)
		}
		var rejected *domain.InvoiceRejectedError
		if err != nil && !errors.As(err, &rejected) {
			return i.recoverIssued(ctx, a, req, number, raw, err)
		}
		return auth, i.dropRejectedTicket(ctx, s, creds.Tickets, err)
	}
}

// Authorized reads back the invoice ARCA holds under number for a type at a point of sale.
func (i *Issuer) Authorized(ctx context.Context, creds domain.ARCACredentials, issuerCUIT string, invoiceType domain.InvoiceType, pointOfSale int, number int64) (*domain.AuthorizedInvoice, error) {
	s, err := parseSigner(creds.CertificatePEM, creds.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	t, err := i.ticket(ctx, s, creds.Tickets)
	if err != nil {
		return nil, err
	}
	got, _, err := i.consult(ctx, feAuth{Token: t.Token, Sign: t.Sign, Cuit: issuerCUIT}, invoiceType, pointOfSale, number)
	if err != nil || got == nil {
		return nil, i.dropRejectedTicket(ctx, s, creds.Tickets, err)
	}
	return got.authorized(number)
}

// soapFault is a SOAP 1.1 Fault; ARCA answers one for infrastructure and WSAA login failures.
type soapFault struct {
	Code   string
	String string
}

func (f *soapFault) Error() string { return "SOAP fault " + f.Code + ": " + f.String }

func (f *soapFault) Unwrap() error { return domain.ErrInvoicingUnavailable }

// transportError is a request that failed in flight, so ARCA may or may not have processed it.
type transportError struct{ err error }

func (e *transportError) Error() string { return "arca transport: " + e.err.Error() }

func (e *transportError) Unwrap() []error { return []error{e.err, domain.ErrInvoicingUnavailable} }

type faultEnvelope struct {
	Body struct {
		Fault *struct {
			Code   string `xml:"faultcode"`
			String string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// envelope wraps body in a SOAP 1.1 envelope, declaring extraNS on it.
func envelope(extraNS string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<soap:Envelope xmlns:soap="` + soapNamespace + `"`)
	if extraNS != "" {
		b.WriteString(" " + extraNS)
	}
	b.WriteString("><soap:Body>")
	b.Write(body)
	b.WriteString("</soap:Body></soap:Envelope>")
	return b.Bytes()
}

// call posts body as a SOAP request and returns the response envelope.
func (i *Issuer) call(ctx context.Context, url, action, extraNS string, body []byte) ([]byte, error) {
	return i.post(ctx, url, action, envelope(extraNS, body))
}

func (i *Issuer) post(ctx context.Context, url, action string, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, unavailable("build request", err)
	}
	httpReq.Header.Set("Content-Type", "text/xml; charset=utf-8")
	httpReq.Header.Set("SOAPAction", `"`+action+`"`)
	res, err := i.client.Do(httpReq)
	if err != nil {
		return nil, &transportError{err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return nil, &transportError{err: err}
	}
	if len(raw) > maxResponseBytes {
		return nil, &transportError{err: errors.New("response exceeds limit")}
	}
	var fault faultEnvelope
	if xml.Unmarshal(raw, &fault) == nil && fault.Body.Fault != nil {
		return nil, &soapFault{Code: fault.Body.Fault.Code, String: fault.Body.Fault.String}
	}
	if res.StatusCode != http.StatusOK {
		return nil, unavailable("call", fmt.Errorf("HTTP %d", res.StatusCode))
	}
	return raw, nil
}

func unavailable(what string, err error) error {
	return fmt.Errorf("arca: %s: %v: %w", what, err, domain.ErrInvoicingUnavailable)
}
