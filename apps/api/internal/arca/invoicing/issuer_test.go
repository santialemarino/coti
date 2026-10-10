package arca

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/smallstep/pkcs7"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var fixedNow = time.Date(2026, 10, 5, 15, 0, 0, 0, domain.Argentina)

// testCredentials builds a self-signed certificate whose subject names the CUIT the way ARCA does.
func testCredentials(t *testing.T, serial string) (certPEM, keyPEM []byte, key *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	subject := pkix.Name{CommonName: "coti-test", Organization: []string{"Coti"}}
	if serial != "" {
		subject.ExtraNames = []pkix.AttributeTypeAndValue{{Type: asn1.ObjectIdentifier{2, 5, 4, 5}, Value: serial}}
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		NotBefore:    fixedNow.Add(-time.Hour),
		NotAfter:     fixedNow.Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, key
}

const soapOpen = `<?xml version="1.0" encoding="utf-8"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`
const soapClose = `</soap:Body></soap:Envelope>`

type fakeWSAA struct {
	mu      sync.Mutex
	logins  int
	cms     []string
	already bool
	fault   string // when set, every login answers a fault with this code.
}

func (f *fakeWSAA) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logins++
	if m := regexp.MustCompile(`<wsaa:in0>([^<]*)</wsaa:in0>`).FindSubmatch(body); m != nil {
		f.cms = append(f.cms, string(m[1]))
	}
	if f.fault != "" {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, soapOpen+`<soap:Fault><faultcode xmlns:ns1="http://xml.apache.org/axis/">ns1:`+f.fault+`</faultcode><faultstring>Motivo de ARCA</faultstring></soap:Fault>`+soapClose)
		return
	}
	if f.already {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, soapOpen+`<soap:Fault><faultcode xmlns:ns1="http://xml.apache.org/axis/">ns1:coe.alreadyAuthenticated</faultcode><faultstring>El CEE ya posee un TA valido para el acceso al WSN solicitado</faultstring></soap:Fault>`+soapClose)
		return
	}
	ticket := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><loginTicketResponse version="1.0"><header><source>CN=wsaahomo</source><destination>SERIALNUMBER=CUIT 20123456789</destination><uniqueId>1</uniqueId><generationTime>2026-10-05T14:50:00.000-03:00</generationTime><expirationTime>2026-10-06T02:50:00.000-03:00</expirationTime></header><credentials><token>TOKEN+secret/abc=</token><sign>SIGN+secret/xyz=</sign></credentials></loginTicketResponse>`
	fmt.Fprint(w, soapOpen+`<loginCmsResponse xmlns="http://wsaa.view.sua.dvadac.desein.afip.gov"><loginCmsReturn>`+html.EscapeString(ticket)+`</loginCmsReturn></loginCmsResponse>`+soapClose)
}

// fakeWSFE answers each operation from a script, recording what it was sent.
type fakeWSFE struct {
	mu          sync.Mutex
	last        []int64  // successive FECompUltimoAutorizado answers, -1 resets the connection; the final one repeats.
	solicitar   []string // successive FECAESolicitarResult bodies; "DROP" resets the connection.
	consult     string   // the FECompConsultarResult body; "DROP" resets the connection.
	requests    map[string][]string
	lastErrors  string
	onSolicitar func() // runs as FECAESolicitar arrives, before it is answered.
}

func (f *fakeWSFE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	op := strings.TrimPrefix(strings.Trim(r.Header.Get("SOAPAction"), `"`), wsfeNamespace)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requests == nil {
		f.requests = map[string][]string{}
	}
	f.requests[op] = append(f.requests[op], string(body))
	switch op {
	case "FECompUltimoAutorizado":
		n := f.last[0]
		if len(f.last) > 1 {
			f.last = f.last[1:]
		}
		if n < 0 {
			drop(w)
			return
		}
		fmt.Fprintf(w, soapOpen+`<FECompUltimoAutorizadoResponse xmlns="http://ar.gov.afip.dif.FEV1/"><FECompUltimoAutorizadoResult><PtoVta>3</PtoVta><CbteTipo>1</CbteTipo><CbteNro>%d</CbteNro>%s</FECompUltimoAutorizadoResult></FECompUltimoAutorizadoResponse>`+soapClose, n, f.lastErrors)
	case "FECAESolicitar":
		if f.onSolicitar != nil {
			f.onSolicitar()
		}
		next := f.solicitar[0]
		f.solicitar = f.solicitar[1:]
		if next == "DROP" {
			drop(w)
			return
		}
		fmt.Fprint(w, soapOpen+`<FECAESolicitarResponse xmlns="http://ar.gov.afip.dif.FEV1/"><FECAESolicitarResult>`+next+`</FECAESolicitarResult></FECAESolicitarResponse>`+soapClose)
	case "FECompConsultar":
		if f.consult == "DROP" {
			drop(w)
			return
		}
		fmt.Fprint(w, soapOpen+`<FECompConsultarResponse xmlns="http://ar.gov.afip.dif.FEV1/"><FECompConsultarResult>`+f.consult+`</FECompConsultarResult></FECompConsultarResponse>`+soapClose)
	default:
		http.Error(w, "unknown action "+op, http.StatusBadRequest)
	}
}

// drop resets the connection mid-exchange, as a timeout or a network cut would.
func drop(w http.ResponseWriter) {
	conn, _, _ := w.(http.Hijacker).Hijack()
	_ = conn.Close()
}

func (f *fakeWSFE) sent(op string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests[op]...)
}

func approved(number int64) string {
	return fmt.Sprintf(`<FeCabResp><Cuit>20123456789</Cuit><PtoVta>3</PtoVta><CbteTipo>1</CbteTipo><FchProceso>20261005150000</FchProceso><CantReg>1</CantReg><Resultado>A</Resultado><Reproceso>N</Reproceso></FeCabResp><FeDetResp><FECAEDetResponse><Concepto>1</Concepto><DocTipo>80</DocTipo><DocNro>30712345678</DocNro><CbteDesde>%d</CbteDesde><CbteHasta>%d</CbteHasta><CbteFch>20261005</CbteFch><Resultado>A</Resultado><Observaciones><Obs><Code>10217</Code><Msg>Observación informativa</Msg></Obs></Observaciones><CAE>76401234567890</CAE><CAEFchVto>20261015</CAEFchVto></FECAEDetResponse></FeDetResp>`, number, number)
}

func rejected(obs ...string) string {
	return `<FeCabResp><Resultado>R</Resultado></FeCabResp><FeDetResp><FECAEDetResponse><Resultado>R</Resultado><Observaciones>` + strings.Join(obs, "") + `</Observaciones><CAE></CAE><CAEFchVto></CAEFchVto></FECAEDetResponse></FeDetResp>`
}

func obs(code int, msg string) string {
	return fmt.Sprintf(`<Obs><Code>%d</Code><Msg>%s</Msg></Obs>`, code, msg)
}

type harness struct {
	issuer *Issuer
	wsaa   *fakeWSAA
	wsfe   *fakeWSFE
	creds  domain.ARCACredentials
	key    *rsa.PrivateKey
}

func newHarness(t *testing.T, wsfe *fakeWSFE) *harness {
	t.Helper()
	wsaa := &fakeWSAA{}
	wsaaSrv := httptest.NewServer(wsaa)
	t.Cleanup(wsaaSrv.Close)
	wsfeSrv := httptest.NewServer(wsfe)
	t.Cleanup(wsfeSrv.Close)
	certPEM, keyPEM, key := testCredentials(t, "CUIT 20123456789")
	issuer := NewIssuer(Settings{
		Environment: EnvironmentHomologation,
		Timeout:     5 * time.Second,
		Now:         func() time.Time { return fixedNow },
		WSAAURL:     wsaaSrv.URL,
		WSFEURL:     wsfeSrv.URL,
	}, NewMemoryTicketCache())
	return &harness{issuer: issuer, wsaa: wsaa, wsfe: wsfe, creds: domain.ARCACredentials{CertificatePEM: certPEM, PrivateKeyPEM: keyPEM}, key: key}
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func invoiceA() domain.InvoiceRequest {
	return domain.InvoiceRequest{
		IssuerCUIT:  "20123456789",
		Type:        domain.InvoiceTypeA,
		PointOfSale: 3,
		Date:        time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), // a calendar day, as the service sends it.
		Receiver: domain.InvoiceReceiver{
			Name: "Obra SA", DocType: domain.ReceiverDocCUIT, DocNumber: "30712345678",
			IVACondition: domain.IVAConditionRegistered,
		},
		Amounts: domain.InvoiceAmounts{
			Net: dec("1100"), Exempt: dec("50"), VAT: dec("220.5"), Total: dec("1370.5"),
			ByRate: []domain.VATAmount{
				{Rate: domain.VATRateTwentyOne, Base: dec("1000"), Amount: dec("210")},
				{Rate: domain.VATRateTenFive, Base: dec("100"), Amount: dec("10.5")},
				{Rate: domain.VATRateExempt, Base: dec("50"), Amount: dec("0")},
			},
		},
	}
}

func invoiceC() domain.InvoiceRequest {
	req := invoiceA()
	req.Type = domain.InvoiceTypeC
	req.Receiver = domain.InvoiceReceiver{Name: "Consumidor Final", DocType: domain.ReceiverDocNone, IVACondition: domain.IVAConditionFinalConsumer}
	req.Amounts = domain.InvoiceAmounts{
		Net: dec("1210"), Total: dec("1210"), VAT: dec("0"), Exempt: dec("0"),
		ByRate: []domain.VATAmount{{Rate: domain.VATRateTwentyOne, Base: dec("1000"), Amount: dec("210")}},
	}
	return req
}

type sentDet struct {
	DocTipo                int    `xml:"DocTipo"`
	DocNro                 string `xml:"DocNro"`
	CbteDesde              int64  `xml:"CbteDesde"`
	CbteHasta              int64  `xml:"CbteHasta"`
	CbteFch                string `xml:"CbteFch"`
	ImpTotal               string `xml:"ImpTotal"`
	ImpTotConc             string `xml:"ImpTotConc"`
	ImpNeto                string `xml:"ImpNeto"`
	ImpOpEx                string `xml:"ImpOpEx"`
	ImpTrib                string `xml:"ImpTrib"`
	ImpIVA                 string `xml:"ImpIVA"`
	MonID                  string `xml:"MonId"`
	CondicionIVAReceptorID int    `xml:"CondicionIVAReceptorId"`
	Iva                    *struct {
		AlicIva []alicIva `xml:"AlicIva"`
	} `xml:"Iva"`
}

type sentSolicitar struct {
	Token    string  `xml:"Body>FECAESolicitar>Auth>Token"`
	Sign     string  `xml:"Body>FECAESolicitar>Auth>Sign"`
	Cuit     string  `xml:"Body>FECAESolicitar>Auth>Cuit"`
	CbteTipo int     `xml:"Body>FECAESolicitar>FeCAEReq>FeCabReq>CbteTipo"`
	Det      sentDet `xml:"Body>FECAESolicitar>FeCAEReq>FeDetReq>FECAEDetRequest"`
}

func decodeSent(t *testing.T, raw string) sentSolicitar {
	t.Helper()
	var s sentSolicitar
	if err := xml.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("decode sent request: %v\n%s", err, raw)
	}
	return s
}

func TestIssuer_Issue_SignsAttachedTRAWithSHA256(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil {
		t.Fatal(err)
	}
	if len(h.wsaa.cms) != 1 {
		t.Fatalf("captured %d CMS, want 1", len(h.wsaa.cms))
	}
	der, err := base64.StdEncoding.DecodeString(h.wsaa.cms[0])
	if err != nil {
		t.Fatal(err)
	}
	p7, err := pkcs7.Parse(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := p7.Verify(); err != nil {
		t.Fatalf("CMS does not verify: %v", err)
	}
	if !p7.GetOnlySigner().PublicKey.(*rsa.PublicKey).Equal(&h.key.PublicKey) {
		t.Fatal("CMS signed by another certificate")
	}
	if !p7.Signers[0].DigestAlgorithm.Algorithm.Equal(pkcs7.OIDDigestAlgorithmSHA256) {
		t.Fatalf("digest %v, want SHA-256", p7.Signers[0].DigestAlgorithm.Algorithm)
	}
	var tra loginTicketRequest
	if err := xml.Unmarshal(p7.Content, &tra); err != nil {
		t.Fatalf("attached content is not a TRA: %v\n%s", err, p7.Content)
	}
	if tra.Version != "1.0" || tra.Service != "wsfe" {
		t.Fatalf("TRA version %q service %q", tra.Version, tra.Service)
	}
	if tra.Header.GenerationTime != "2026-10-05T14:50:00-03:00" || tra.Header.ExpirationTime != "2026-10-05T15:10:00-03:00" {
		t.Fatalf("TRA window %s → %s", tra.Header.GenerationTime, tra.Header.ExpirationTime)
	}
}

func TestIssuer_Issue_ReusesCachedTicket(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41, 42}, solicitar: []string{approved(42), approved(43)}})
	for range 2 {
		if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil {
			t.Fatal(err)
		}
	}
	if h.wsaa.logins != 1 {
		t.Fatalf("LoginCms called %d times, want 1", h.wsaa.logins)
	}
	if got := decodeSent(t, h.wsfe.sent("FECAESolicitar")[1]).Det.CbteDesde; got != 43 {
		t.Fatalf("second invoice numbered %d, want 43", got)
	}
}

func TestIssuer_Issue_AlreadyAuthenticatedWithoutCacheIsUnavailable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	h.wsaa.already = true
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if !errors.Is(err, domain.ErrInvoicingUnavailable) || !strings.Contains(err.Error(), "retry after it expires") {
		t.Fatalf("err = %v, want unavailable naming the active ticket", err)
	}
}

// A login refused over the certificate is the corralón's to fix; anything else is ARCA being down.
func TestIssuer_Issue_LoginFaults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fault    string
		rejected bool
	}{
		{"cms.cert.untrusted", true},
		{"cms.sign.invalid", true},
		{"coe.notAuthorized", true},
		{"wsn.unavailable", false},
	}
	for _, tc := range cases {
		t.Run(tc.fault, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
			h.wsaa.fault = tc.fault
			_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
			var rejected *domain.InvoiceRejectedError
			if got := errors.As(err, &rejected); got != tc.rejected {
				t.Fatalf("err = %v, rejected = %v, want %v", err, got, tc.rejected)
			}
			if tc.rejected && (len(rejected.Issues) != 1 || rejected.Issues[0] != "WSAA "+tc.fault+": Motivo de ARCA") {
				t.Fatalf("issues = %q, want the fault named", rejected.Issues)
			}
			if !tc.rejected && !errors.Is(err, domain.ErrInvoicingUnavailable) {
				t.Fatalf("err = %v, want unavailable", err)
			}
			if n := len(h.wsfe.sent("FECAESolicitar")); n != 0 {
				t.Fatalf("FECAESolicitar called %d times after a failed login", n)
			}
		})
	}
}

func TestIssuer_Issue_InvoiceARequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil {
		t.Fatal(err)
	}
	sent := h.wsfe.sent("FECAESolicitar")
	s := decodeSent(t, sent[0])
	d := s.Det
	if s.Cuit != "20123456789" || s.CbteTipo != 1 {
		t.Fatalf("Cuit %q CbteTipo %d", s.Cuit, s.CbteTipo)
	}
	if d.DocTipo != 80 || d.DocNro != "30712345678" || d.CbteDesde != 42 || d.CbteHasta != 42 || d.CbteFch != "20261005" {
		t.Fatalf("det header %+v", d)
	}
	if d.ImpTotal != "1370.50" || d.ImpNeto != "1100.00" || d.ImpOpEx != "50.00" || d.ImpIVA != "220.50" || d.ImpTotConc != "0.00" || d.ImpTrib != "0.00" || d.MonID != "PES" {
		t.Fatalf("det amounts %+v", d)
	}
	if d.CondicionIVAReceptorID != 1 {
		t.Fatalf("CondicionIVAReceptorId %d, want 1", d.CondicionIVAReceptorID)
	}
	want := []alicIva{{ID: 5, BaseImp: "1000.00", Importe: "210.00"}, {ID: 4, BaseImp: "100.00", Importe: "10.50"}}
	if d.Iva == nil || fmt.Sprint(d.Iva.AlicIva) != fmt.Sprint(want) {
		t.Fatalf("Iva %+v, want %+v", d.Iva, want)
	}
	if !strings.Contains(sent[0], "<MonCotiz>1</MonCotiz><CondicionIVAReceptorId>1</CondicionIVAReceptorId><Iva>") {
		t.Fatal("detail fields out of WSDL order")
	}
}

func TestIssuer_Issue_InvoiceCOmitsIvaAndNoneReceiver(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{7}, solicitar: []string{approved(8)}})
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceC()); err != nil {
		t.Fatal(err)
	}
	raw := h.wsfe.sent("FECAESolicitar")[0]
	s := decodeSent(t, raw)
	d := s.Det
	if s.CbteTipo != 11 || d.Iva != nil || strings.Contains(raw, "<Iva") {
		t.Fatalf("C invoice: CbteTipo %d, Iva %+v", s.CbteTipo, d.Iva)
	}
	if d.ImpNeto != "1210.00" || d.ImpIVA != "0.00" || d.ImpOpEx != "0.00" || d.ImpTotal != "1210.00" {
		t.Fatalf("C amounts %+v", d)
	}
	if d.DocTipo != 99 || d.DocNro != "0" || d.CondicionIVAReceptorID != 5 {
		t.Fatalf("NONE receiver: DocTipo %d DocNro %q condition %d", d.DocTipo, d.DocNro, d.CondicionIVAReceptorID)
	}
}

func TestIssuer_Issue_MapsApproval(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	auth, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Number != 42 || auth.CAE != "76401234567890" {
		t.Fatalf("number %d CAE %q", auth.Number, auth.CAE)
	}
	if !auth.CAEExpiresOn.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("CAE expires %v", auth.CAEExpiresOn)
	}
	if len(auth.Observations) != 1 || auth.Observations[0] != "10217: Observación informativa" {
		t.Fatalf("observations %q", auth.Observations)
	}
	if !strings.Contains(string(auth.RawResponse), "76401234567890") {
		t.Fatal("raw response not captured")
	}
}

func TestIssuer_Issue_RedactsTicketInRawRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	auth, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeSent(t, h.wsfe.sent("FECAESolicitar")[0]); got.Token != "TOKEN+secret/abc=" || got.Sign != "SIGN+secret/xyz=" {
		t.Fatalf("ARCA was sent token %q sign %q", got.Token, got.Sign)
	}
	raw := string(auth.RawRequest)
	if strings.Contains(raw, "secret") {
		t.Fatalf("raw request leaks the ticket: %s", raw)
	}
	if s := decodeSent(t, raw); s.Token != redacted || s.Sign != redacted || s.Det.CbteDesde != 42 {
		t.Fatalf("raw request is not the redacted invoice: %s", raw)
	}
}

func TestIssuer_Issue_MapsRejection(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{
		rejected(obs(10013, "DocTipo invalido")) + `<Errors><Err><Code>10048</Code><Msg>ImpTotal mal calculado</Msg></Err></Errors>`,
	}})
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	var rej *domain.InvoiceRejectedError
	if !errors.As(err, &rej) {
		t.Fatalf("err = %v, want InvoiceRejectedError", err)
	}
	want := []string{"10048: ImpTotal mal calculado", "10013: DocTipo invalido"}
	if fmt.Sprint(rej.Issues) != fmt.Sprint(want) {
		t.Fatalf("issues %q, want %q", rej.Issues, want)
	}
	if len(rej.RawRequest) == 0 || !strings.Contains(string(rej.RawResponse), "10013") || strings.Contains(string(rej.RawRequest), "secret") {
		t.Fatal("rejection raw XML missing or unredacted")
	}
	if errors.Is(err, domain.ErrInvoicingUnavailable) {
		t.Fatal("a rejection must not read as unavailable")
	}
}

// A ticket WSFEv1 refuses is forgotten, so the next call logs in afresh instead of failing until it expires.
func TestIssuer_Issue_RefusedTicketIsDropped(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{
		`<Errors><Err><Code>600</Code><Msg>ValidacionDeToken: No validaron las fechas del token</Msg></Err></Errors>`,
		approved(42),
	}})
	store := &memoryTicketStore{}
	h.creds.Tickets = store
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	var rej *domain.InvoiceRejectedError
	if !errors.Is(err, domain.ErrInvoicingUnavailable) || errors.As(err, &rej) {
		t.Fatalf("err = %v, want unavailable", err)
	}
	if store.ticket.Token != "" {
		t.Fatalf("stored ticket %+v, want it cleared", store.ticket)
	}
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil || h.wsaa.logins != 2 {
		t.Fatalf("err = %v after %d logins, want a fresh login", err, h.wsaa.logins)
	}
}

func TestIssuer_Issue_RetriesOnceWhenNumberWasTaken(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41, 42}, solicitar: []string{
		rejected(obs(10016, "comp. 42 no coincide con el próximo a autorizar")),
		approved(43),
	}})
	auth, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Number != 43 || decodeSent(t, h.wsfe.sent("FECAESolicitar")[1]).Det.CbteDesde != 43 {
		t.Fatalf("retried as %d, want 43", auth.Number)
	}
}

func TestIssuer_Issue_RetriesNumberOnlyOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41, 42}, solicitar: []string{
		rejected(obs(10016, "no coincide")),
		rejected(obs(10016, "no coincide")),
		approved(44),
	}})
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	var rej *domain.InvoiceRejectedError
	if !errors.As(err, &rej) || len(h.wsfe.sent("FECAESolicitar")) != 2 {
		t.Fatalf("err = %v after %d requests, want a rejection after 2", err, len(h.wsfe.sent("FECAESolicitar")))
	}
}

const consultedA = `<ResultGet><Concepto>1</Concepto><DocTipo>80</DocTipo><DocNro>30712345678</DocNro><CbteDesde>42</CbteDesde><CbteHasta>42</CbteHasta><CbteFch>20261005</CbteFch><ImpTotal>1370.5</ImpTotal><Resultado>A</Resultado><CodAutorizacion>76409999999999</CodAutorizacion><EmisionTipo>CAE</EmisionTipo><FchVto>20261015</FchVto><FchProceso>20261005150000</FchProceso><PtoVta>3</PtoVta><CbteTipo>1</CbteTipo></ResultGet>`

func TestIssuer_Issue_RecoversInvoiceAuthorizedInFlight(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41, 42}, solicitar: []string{"DROP"}, consult: consultedA})
	auth, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if err != nil {
		t.Fatal(err)
	}
	if auth.Number != 42 || auth.CAE != "76409999999999" || !auth.CAEExpiresOn.Equal(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("recovered %+v", auth)
	}
	consulted := h.wsfe.sent("FECompConsultar")
	if len(consulted) != 1 || !strings.Contains(consulted[0], "<CbteNro>42</CbteNro>") {
		t.Fatalf("FECompConsultar requests %q", consulted)
	}
	if strings.Contains(string(auth.RawRequest), "secret") || len(auth.RawRequest) == 0 {
		t.Fatal("recovered raw request missing or unredacted")
	}
}

const noRecords = `<Errors><Err><Code>602</Code><Msg>Sin Resultados: - en FECompConsultar</Msg></Err></Errors>`

// ARCA may still be processing a request whose answer was lost; only proof releases the number.
func TestIssuer_Issue_InFlightRequestNotOnRecordIsUnknown(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{"DROP"}, consult: noRecords})
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if !errors.Is(err, domain.ErrInvoiceOutcomeUnknown) || errors.Is(err, domain.ErrInvoicingUnavailable) {
		t.Fatalf("err = %v, want only outcome unknown", err)
	}
}

func TestIssuer_Issue_RecoveryRefusesAnotherSystemsInvoice(t *testing.T) {
	t.Parallel()
	other := strings.Replace(consultedA, "<ImpTotal>1370.5</ImpTotal>", "<ImpTotal>99</ImpTotal>", 1)
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{"DROP"}, consult: other})
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if !errors.Is(err, domain.ErrInvoiceOutcomeUnknown) {
		t.Fatalf("err = %v, want the uncertain outcome held", err)
	}
}

func TestIssuer_Issue_OutcomeUnknownWhenReadBackFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{"DROP"}, consult: "DROP"})
	_, err := h.issuer.Issue(context.Background(), h.creds, invoiceA())
	if !errors.Is(err, domain.ErrInvoiceOutcomeUnknown) || errors.Is(err, domain.ErrInvoicingUnavailable) {
		t.Errorf("err = %v, want only outcome unknown", err)
	}
}

// A request cut off by its own deadline may still have reached ARCA.
func TestIssuer_Issue_CancelledInFlightIsUnknown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{"DROP"}, consult: consultedA,
		onSolicitar: cancel})
	_, err := h.issuer.Issue(ctx, h.creds, invoiceA())
	if !errors.Is(err, domain.ErrInvoiceOutcomeUnknown) {
		t.Fatalf("err = %v, want outcome unknown", err)
	}
}

func TestIssuer_Issue_ClaimsTheNumberBeforeAskingForIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	var claimed []int64
	req := invoiceA()
	req.Claim = func(_ context.Context, number int64) error {
		if n := len(h.wsfe.sent("FECAESolicitar")); n != 0 {
			t.Errorf("claimed after %d requests, want before", n)
		}
		claimed = append(claimed, number)
		return nil
	}
	if _, err := h.issuer.Issue(context.Background(), h.creds, req); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(claimed) != "[42]" {
		t.Fatalf("claimed %v, want [42]", claimed)
	}

	refused := invoiceA()
	refused.Claim = func(context.Context, int64) error { return errors.New("database down") }
	h2 := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	if _, err := h2.issuer.Issue(context.Background(), h2.creds, refused); err == nil || len(h2.wsfe.sent("FECAESolicitar")) != 0 {
		t.Fatalf("err = %v, want the request never sent when the claim fails", err)
	}
}

// The date ARCA stamps is the calendar day the service chose, whatever the server's zone.
func TestIssuer_Issue_SendsTheCalendarDay(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	req := invoiceA()
	req.Date = domain.InvoiceDay(time.Date(2026, 10, 6, 1, 30, 0, 0, time.UTC)) // the 5th in Buenos Aires.
	if _, err := h.issuer.Issue(context.Background(), h.creds, req); err != nil {
		t.Fatal(err)
	}
	if got := decodeSent(t, h.wsfe.sent("FECAESolicitar")[0]).Det.CbteFch; got != "20261005" {
		t.Fatalf("CbteFch = %s, want 20261005", got)
	}
}

// memoryTicketStore stands in for an account's stored ticket.
type memoryTicketStore struct {
	mu     sync.Mutex
	ticket domain.ARCATicket
	saves  int
}

func (m *memoryTicketStore) Load(context.Context) (*domain.ARCATicket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ticket.Token == "" {
		return nil, nil
	}
	t := m.ticket
	return &t, nil
}

func (m *memoryTicketStore) Save(_ context.Context, t domain.ARCATicket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticket, m.saves = t, m.saves+1
	return nil
}

// WSAA refuses a second login while a ticket is live, so a restart must reuse the stored one.
func TestIssuer_Issue_ReusesTheStoredTicketAfterARestart(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41, 42}, solicitar: []string{approved(42), approved(43)}})
	store := &memoryTicketStore{}
	h.creds.Tickets = store
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil {
		t.Fatal(err)
	}
	if store.ticket.Token != "TOKEN+secret/abc=" {
		t.Fatalf("stored %+v, want the ticket WSAA issued", store.ticket)
	}

	restarted := NewIssuer(Settings{Timeout: 5 * time.Second, Now: func() time.Time { return fixedNow },
		WSAAURL: h.issuer.wsaaURL, WSFEURL: h.issuer.wsfeURL}, NewMemoryTicketCache())
	if _, err := restarted.Issue(context.Background(), h.creds, invoiceA()); err != nil {
		t.Fatal(err)
	}
	if h.wsaa.logins != 1 {
		t.Fatalf("logins = %d, want the stored ticket reused", h.wsaa.logins)
	}
}

// Inside the renew margin WSAA may still refuse a new login; the live ticket carries the call.
func TestIssuer_Issue_FallsBackToTheLiveTicketWhenWSAARefuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{last: []int64{41}, solicitar: []string{approved(42)}})
	h.wsaa.already = true
	h.creds.Tickets = &memoryTicketStore{ticket: domain.ARCATicket{Token: "LIVE", Sign: "SIG",
		ExpiresAt: fixedNow.Add(ticketRenewMargin / 2)}}
	if _, err := h.issuer.Issue(context.Background(), h.creds, invoiceA()); err != nil {
		t.Fatal(err)
	}
	if got := decodeSent(t, h.wsfe.sent("FECAESolicitar")[0]); got.Token != "LIVE" {
		t.Fatalf("token %q, want the live ticket", got.Token)
	}
}

func TestIssuer_Authorized_ReadsTheNumberAsked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{consult: consultedA})
	got, err := h.issuer.Authorized(context.Background(), h.creds, "20123456789", domain.InvoiceTypeA, 3, 42)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.AuthorizedInvoice{
		Number: 42, CAE: "76409999999999",
		CAEExpiresOn: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
		Date:         time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		DocType:      domain.ReceiverDocCUIT, DocNumber: "30712345678",
	}
	if got == nil || got.Number != want.Number || got.CAE != want.CAE || !got.CAEExpiresOn.Equal(want.CAEExpiresOn) ||
		!got.Date.Equal(want.Date) || got.DocType != want.DocType || got.DocNumber != want.DocNumber || !got.Total.Equal(dec("1370.5")) {
		t.Fatalf("got %+v, want %+v total 1370.5", got, want)
	}
	consulted := h.wsfe.sent("FECompConsultar")
	if len(consulted) != 1 || !strings.Contains(consulted[0], "<CbteTipo>1</CbteTipo><CbteNro>42</CbteNro><PtoVta>3</PtoVta>") ||
		!strings.Contains(consulted[0], "<Cuit>20123456789</Cuit>") {
		t.Fatalf("FECompConsultar requests %q", consulted)
	}
}

func TestIssuer_Authorized_NoneIsNil(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{consult: noRecords})
	got, err := h.issuer.Authorized(context.Background(), h.creds, "20123456789", domain.InvoiceTypeB, 3, 9)
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
}

func TestIssuer_Authorized_TransportFailureIsUnavailable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, &fakeWSFE{consult: "DROP"})
	_, err := h.issuer.Authorized(context.Background(), h.creds, "20123456789", domain.InvoiceTypeA, 3, 42)
	if !errors.Is(err, domain.ErrInvoicingUnavailable) {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestParseCredentials(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testCredentials(t, "CUIT 20123456789")
	_, otherKey, _ := testCredentials(t, "CUIT 20123456789")
	noCUITCert, noCUITKey, _ := testCredentials(t, "")

	info, err := ParseCredentials(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if info.CUIT != "20123456789" || !strings.Contains(info.Subject, "coti-test") || !info.NotAfter.Equal(fixedNow.Add(365*24*time.Hour)) {
		t.Fatalf("info %+v", info)
	}

	for name, pair := range map[string][2][]byte{
		"mismatched key": {certPEM, otherKey},
		"missing CUIT":   {noCUITCert, noCUITKey},
		"garbage":        {[]byte("nope"), keyPEM},
	} {
		_, err := ParseCredentials(pair[0], pair[1])
		if !errors.Is(err, domain.ErrInvalidInput) || domain.CodeOf(err) != domain.CodeARCACredentials {
			t.Errorf("%s: err = %v (code %q), want ARCA_CREDENTIALS invalid input", name, err, domain.CodeOf(err))
		}
	}
}
