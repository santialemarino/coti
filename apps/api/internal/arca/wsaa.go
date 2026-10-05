package arca

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smallstep/pkcs7"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const (
	wsaaNamespace = "http://wsaa.view.sua.dvadac.desein.afip.gov"
	wsfeService   = "wsfe"
	// ticketRenewMargin keeps a ticket from expiring between the cache read and ARCA's check.
	ticketRenewMargin = 10 * time.Minute
)

// Ticket is a WSAA access ticket for the wsfe service.
type Ticket struct {
	Token     string
	Sign      string
	ExpiresAt time.Time
}

// TicketCache keeps WSAA tickets by certificate, since WSAA refuses a new one while one is live.
type TicketCache interface {
	Get(key string) (Ticket, bool)
	Put(key string, t Ticket)
}

// MemoryTicketCache is a process-local TicketCache.
type MemoryTicketCache struct {
	mu      sync.Mutex
	tickets map[string]Ticket
}

// NewMemoryTicketCache builds an empty MemoryTicketCache.
func NewMemoryTicketCache() *MemoryTicketCache {
	return &MemoryTicketCache{tickets: map[string]Ticket{}}
}

// Get returns the ticket stored for key.
func (c *MemoryTicketCache) Get(key string) (Ticket, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tickets[key]
	return t, ok
}

// Put stores the ticket for key.
func (c *MemoryTicketCache) Put(key string, t Ticket) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tickets[key] = t
}

type loginTicketRequest struct {
	XMLName xml.Name `xml:"loginTicketRequest"`
	Version string   `xml:"version,attr"`
	Header  struct {
		UniqueID       uint32 `xml:"uniqueId"`
		GenerationTime string `xml:"generationTime"`
		ExpirationTime string `xml:"expirationTime"`
	} `xml:"header"`
	Service string `xml:"service"`
}

type loginTicketResponse struct {
	Header struct {
		ExpirationTime string `xml:"expirationTime"`
	} `xml:"header"`
	Credentials struct {
		Token string `xml:"token"`
		Sign  string `xml:"sign"`
	} `xml:"credentials"`
}

type loginCmsRequest struct {
	XMLName xml.Name `xml:"wsaa:loginCms"`
	In0     string   `xml:"wsaa:in0"`
}

type loginCmsEnvelope struct {
	Body struct {
		Return string `xml:"loginCmsResponse>loginCmsReturn"`
	} `xml:"Body"`
}

// ticket returns a live ticket for the certificate, logging in only when none is cached.
func (i *Issuer) ticket(ctx context.Context, s *signer) (Ticket, error) {
	key := s.cacheKey()
	// Serialized so two concurrent first calls do not both log in and trip alreadyAuthenticated.
	i.loginMu.Lock()
	defer i.loginMu.Unlock()
	if t, ok := i.cache.Get(key); ok && i.now().Add(ticketRenewMargin).Before(t.ExpiresAt) {
		return t, nil
	}
	t, err := i.login(ctx, s)
	if err != nil {
		return Ticket{}, err
	}
	i.cache.Put(key, t)
	return t, nil
}

func (i *Issuer) login(ctx context.Context, s *signer) (Ticket, error) {
	tra, err := i.buildTRA()
	if err != nil {
		return Ticket{}, err
	}
	cms, err := signTRA(tra, s)
	if err != nil {
		return Ticket{}, err
	}
	body, err := xml.Marshal(loginCmsRequest{In0: base64.StdEncoding.EncodeToString(cms)})
	if err != nil {
		return Ticket{}, fmt.Errorf("arca: encode loginCms: %w", err)
	}
	raw, err := i.call(ctx, i.wsaaURL, "", `xmlns:wsaa="`+wsaaNamespace+`"`, body)
	if err != nil {
		var fault *soapFault
		if errors.As(err, &fault) && strings.HasSuffix(fault.Code, "coe.alreadyAuthenticated") {
			return Ticket{}, fmt.Errorf("arca: WSAA still holds an active ticket for this certificate; retry after it expires: %w", domain.ErrInvoicingUnavailable)
		}
		return Ticket{}, fmt.Errorf("arca: WSAA login: %w", err)
	}
	var env loginCmsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return Ticket{}, unavailable("decode loginCms response", err)
	}
	var resp loginTicketResponse
	if err := xml.Unmarshal([]byte(env.Body.Return), &resp); err != nil {
		return Ticket{}, unavailable("decode login ticket", err)
	}
	expires, err := time.Parse(time.RFC3339, strings.TrimSpace(resp.Header.ExpirationTime))
	if err != nil || resp.Credentials.Token == "" || resp.Credentials.Sign == "" {
		return Ticket{}, unavailable("decode login ticket", fmt.Errorf("incomplete ticket (expiration %q)", resp.Header.ExpirationTime))
	}
	return Ticket{Token: resp.Credentials.Token, Sign: resp.Credentials.Sign, ExpiresAt: expires}, nil
}

// buildTRA renders the loginTicketRequest WSAA spec 1.2.2 asks for.
func (i *Issuer) buildTRA() ([]byte, error) {
	now := i.now().In(argentina)
	req := loginTicketRequest{Version: "1.0", Service: wsfeService}
	req.Header.UniqueID = uint32(now.Unix())
	req.Header.GenerationTime = now.Add(-10 * time.Minute).Format(time.RFC3339)
	req.Header.ExpirationTime = now.Add(10 * time.Minute).Format(time.RFC3339)
	out, err := xml.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("arca: encode TRA: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// signTRA wraps the TRA in a CMS SignedData with the content attached, as LoginCms requires.
func signTRA(tra []byte, s *signer) ([]byte, error) {
	sd, err := pkcs7.NewSignedData(tra)
	if err != nil {
		return nil, fmt.Errorf("arca: sign TRA: %w", err)
	}
	// SHA-256: WSAA accepts it and SHA-1 signatures are deprecated; the library defaults to SHA-1.
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	if err := sd.AddSigner(s.cert, s.key, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, fmt.Errorf("arca: sign TRA: %w", err)
	}
	der, err := sd.Finish()
	if err != nil {
		return nil, fmt.Errorf("arca: sign TRA: %w", err)
	}
	return der, nil
}
