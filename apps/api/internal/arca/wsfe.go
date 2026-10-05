package arca

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const (
	wsfeNamespace     = "http://ar.gov.afip.dif.FEV1/"
	conceptProducts   = 1
	codeNotNextNumber = 10016
	redacted          = "REDACTED"
	arcaDate          = "20060102"
)

type feAuth struct {
	Token string `xml:"Token"`
	Sign  string `xml:"Sign"`
	Cuit  string `xml:"Cuit"`
}

type lastAuthorizedRequest struct {
	XMLName  xml.Name `xml:"http://ar.gov.afip.dif.FEV1/ FECompUltimoAutorizado"`
	Auth     feAuth   `xml:"Auth"`
	PtoVta   int      `xml:"PtoVta"`
	CbteTipo int      `xml:"CbteTipo"`
}

type solicitarRequest struct {
	XMLName  xml.Name `xml:"http://ar.gov.afip.dif.FEV1/ FECAESolicitar"`
	Auth     feAuth   `xml:"Auth"`
	FeCAEReq struct {
		FeCabReq struct {
			CantReg  int `xml:"CantReg"`
			PtoVta   int `xml:"PtoVta"`
			CbteTipo int `xml:"CbteTipo"`
		} `xml:"FeCabReq"`
		FeDetReq struct {
			Det []detRequest `xml:"FECAEDetRequest"`
		} `xml:"FeDetReq"`
	} `xml:"FeCAEReq"`
}

// detRequest follows the WSDL's sequence order, which the .asmx serializer enforces.
type detRequest struct {
	Concepto               int      `xml:"Concepto"`
	DocTipo                int      `xml:"DocTipo"`
	DocNro                 string   `xml:"DocNro"`
	CbteDesde              int64    `xml:"CbteDesde"`
	CbteHasta              int64    `xml:"CbteHasta"`
	CbteFch                string   `xml:"CbteFch"`
	ImpTotal               string   `xml:"ImpTotal"`
	ImpTotConc             string   `xml:"ImpTotConc"`
	ImpNeto                string   `xml:"ImpNeto"`
	ImpOpEx                string   `xml:"ImpOpEx"`
	ImpTrib                string   `xml:"ImpTrib"`
	ImpIVA                 string   `xml:"ImpIVA"`
	MonID                  string   `xml:"MonId"`
	MonCotiz               string   `xml:"MonCotiz"`
	CondicionIVAReceptorID int      `xml:"CondicionIVAReceptorId"`
	Iva                    *ivaList `xml:"Iva,omitempty"`
}

type ivaList struct {
	AlicIva []alicIva `xml:"AlicIva"`
}

type alicIva struct {
	ID      int    `xml:"Id"`
	BaseImp string `xml:"BaseImp"`
	Importe string `xml:"Importe"`
}

type consultRequest struct {
	XMLName       xml.Name `xml:"http://ar.gov.afip.dif.FEV1/ FECompConsultar"`
	Auth          feAuth   `xml:"Auth"`
	FeCompConsReq struct {
		CbteTipo int   `xml:"CbteTipo"`
		CbteNro  int64 `xml:"CbteNro"`
		PtoVta   int   `xml:"PtoVta"`
	} `xml:"FeCompConsReq"`
}

type feMessage struct {
	Code int    `xml:"Code"`
	Msg  string `xml:"Msg"`
}

func (m feMessage) String() string { return fmt.Sprintf("%d: %s", m.Code, strings.TrimSpace(m.Msg)) }

type lastAuthorizedEnvelope struct {
	Result struct {
		CbteNro int64       `xml:"CbteNro"`
		Errors  []feMessage `xml:"Errors>Err"`
	} `xml:"Body>FECompUltimoAutorizadoResponse>FECompUltimoAutorizadoResult"`
}

type solicitarResult struct {
	Cab struct {
		Resultado string `xml:"Resultado"`
	} `xml:"FeCabResp"`
	Det []struct {
		Resultado     string      `xml:"Resultado"`
		Observaciones []feMessage `xml:"Observaciones>Obs"`
		CAE           string      `xml:"CAE"`
		CAEFchVto     string      `xml:"CAEFchVto"`
	} `xml:"FeDetResp>FECAEDetResponse"`
	Errors []feMessage `xml:"Errors>Err"`
}

type solicitarEnvelope struct {
	Result solicitarResult `xml:"Body>FECAESolicitarResponse>FECAESolicitarResult"`
}

// consulted is an invoice as FECompConsultar returns it.
type consulted struct {
	DocTipo         int         `xml:"DocTipo"`
	DocNro          string      `xml:"DocNro"`
	CbteFch         string      `xml:"CbteFch"`
	ImpTotal        string      `xml:"ImpTotal"`
	Resultado       string      `xml:"Resultado"`
	CodAutorizacion string      `xml:"CodAutorizacion"`
	FchVto          string      `xml:"FchVto"`
	Observaciones   []feMessage `xml:"Observaciones>Obs"`
}

type consultEnvelope struct {
	Result struct {
		Get    consulted   `xml:"ResultGet"`
		Errors []feMessage `xml:"Errors>Err"`
	} `xml:"Body>FECompConsultarResponse>FECompConsultarResult"`
}

type exchange struct {
	request  []byte
	response []byte
}

// isServiceFailure reports ARCA's infrastructure (500–502) and authentication (600–602) errors,
// which say nothing about the invoice itself.
func isServiceFailure(code int) bool {
	return (code >= 500 && code <= 502) || (code >= 600 && code <= 602)
}

// checkErrors turns a call's Errors into the error the service acts on.
func checkErrors(op string, errs []feMessage, raw exchange) error {
	if len(errs) == 0 {
		return nil
	}
	issues := make([]string, 0, len(errs))
	for _, e := range errs {
		if isServiceFailure(e.Code) {
			return unavailable(op, errors.New(e.String()))
		}
		issues = append(issues, e.String())
	}
	return &domain.InvoiceRejectedError{Issues: issues, RawRequest: raw.request, RawResponse: raw.response}
}

func (i *Issuer) lastAuthorized(ctx context.Context, a feAuth, invoiceType domain.InvoiceType, pointOfSale int) (int64, error) {
	body, err := xml.Marshal(lastAuthorizedRequest{Auth: a, PtoVta: pointOfSale, CbteTipo: invoiceType.ARCACode()})
	if err != nil {
		return 0, fmt.Errorf("arca: encode FECompUltimoAutorizado: %w", err)
	}
	raw, err := i.call(ctx, i.wsfeURL, wsfeNamespace+"FECompUltimoAutorizado", "", body)
	if err != nil {
		return 0, fmt.Errorf("arca: FECompUltimoAutorizado: %w", err)
	}
	var env lastAuthorizedEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return 0, unavailable("decode FECompUltimoAutorizado", err)
	}
	if err := checkErrors("FECompUltimoAutorizado", env.Result.Errors, exchange{}); err != nil {
		return 0, err
	}
	return env.Result.CbteNro, nil
}

// solicitar asks for the CAE; the exchange carries the request even when the call fails.
func (i *Issuer) solicitar(ctx context.Context, a feAuth, req domain.InvoiceRequest, number int64) (*solicitarResult, exchange, error) {
	msg := buildSolicitar(req, number)
	msg.Auth = feAuth{Token: redacted, Sign: redacted, Cuit: a.Cuit}
	redactedBody, err := xml.Marshal(msg)
	if err != nil {
		return nil, exchange{}, fmt.Errorf("arca: encode FECAESolicitar: %w", err)
	}
	msg.Auth = a
	body, err := xml.Marshal(msg)
	if err != nil {
		return nil, exchange{}, fmt.Errorf("arca: encode FECAESolicitar: %w", err)
	}
	ex := exchange{request: envelope("", redactedBody)}
	raw, err := i.post(ctx, i.wsfeURL, wsfeNamespace+"FECAESolicitar", envelope("", body))
	if err != nil {
		return nil, ex, fmt.Errorf("arca: FECAESolicitar: %w", err)
	}
	ex.response = raw
	var env solicitarEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, ex, unavailable("decode FECAESolicitar", err)
	}
	return &env.Result, ex, nil
}

func buildSolicitar(req domain.InvoiceRequest, number int64) solicitarRequest {
	var msg solicitarRequest
	msg.FeCAEReq.FeCabReq.CantReg = 1
	msg.FeCAEReq.FeCabReq.PtoVta = req.PointOfSale
	msg.FeCAEReq.FeCabReq.CbteTipo = req.Type.ARCACode()

	docNumber := req.Receiver.DocNumber
	if req.Receiver.DocType == domain.ReceiverDocNone || docNumber == "" {
		docNumber = "0"
	}
	amounts := req.Amounts
	det := detRequest{
		Concepto:               conceptProducts,
		DocTipo:                req.Receiver.DocType.ARCACode(),
		DocNro:                 docNumber,
		CbteDesde:              number,
		CbteHasta:              number,
		CbteFch:                req.Date.In(argentina).Format(arcaDate),
		ImpTotal:               money(amounts.Total),
		ImpTotConc:             money(decimal.Zero),
		ImpNeto:                money(amounts.Net),
		ImpOpEx:                money(amounts.Exempt),
		ImpTrib:                money(decimal.Zero),
		ImpIVA:                 money(amounts.VAT),
		MonID:                  "PES",
		MonCotiz:               "1",
		CondicionIVAReceptorID: req.Receiver.IVACondition.ARCAReceiverID(),
	}
	if !req.Type.DiscriminatesVAT() {
		det.ImpNeto = money(amounts.Total)
		det.ImpOpEx = money(decimal.Zero)
		det.ImpIVA = money(decimal.Zero)
	} else {
		var rates []alicIva
		for _, r := range amounts.ByRate {
			if r.Rate == domain.VATRateExempt {
				continue
			}
			rates = append(rates, alicIva{ID: r.Rate.ARCAID(), BaseImp: money(r.Base), Importe: money(r.Amount)})
		}
		if len(rates) > 0 {
			det.Iva = &ivaList{AlicIva: rates}
		}
	}
	msg.FeCAEReq.FeDetReq.Det = []detRequest{det}
	return msg
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

func (r *solicitarResult) hasCode(code int) bool {
	for _, e := range r.Errors {
		if e.Code == code {
			return true
		}
	}
	for _, d := range r.Det {
		for _, o := range d.Observaciones {
			if o.Code == code {
				return true
			}
		}
	}
	return false
}

// authorization maps ARCA's answer to an authorization or to the error the service acts on.
func (r *solicitarResult) authorization(number int64, raw exchange) (*domain.InvoiceAuthorization, error) {
	for _, e := range r.Errors {
		if isServiceFailure(e.Code) {
			return nil, unavailable("FECAESolicitar", errors.New(e.String()))
		}
	}
	var observations []string
	for _, d := range r.Det {
		for _, o := range d.Observaciones {
			observations = append(observations, o.String())
		}
	}
	if r.Cab.Resultado == "A" && len(r.Errors) == 0 && len(r.Det) == 1 && r.Det[0].Resultado == "A" && r.Det[0].CAE != "" {
		expires, err := time.ParseInLocation(arcaDate, r.Det[0].CAEFchVto, argentina)
		if err != nil {
			return nil, unavailable("decode CAEFchVto", err)
		}
		return &domain.InvoiceAuthorization{
			Number:       number,
			CAE:          r.Det[0].CAE,
			CAEExpiresOn: expires,
			Observations: observations,
			RawRequest:   raw.request,
			RawResponse:  raw.response,
		}, nil
	}
	if r.Cab.Resultado != "R" && len(r.Errors) == 0 {
		return nil, unavailable("FECAESolicitar", fmt.Errorf("unexpected result %q", r.Cab.Resultado))
	}
	issues := make([]string, 0, len(r.Errors)+len(observations))
	for _, e := range r.Errors {
		issues = append(issues, e.String())
	}
	issues = append(issues, observations...)
	return nil, &domain.InvoiceRejectedError{Issues: issues, RawRequest: raw.request, RawResponse: raw.response}
}

// consult reads one authorized invoice back from ARCA.
func (i *Issuer) consult(ctx context.Context, a feAuth, invoiceType domain.InvoiceType, pointOfSale int, number int64) (*consulted, []byte, error) {
	var msg consultRequest
	msg.Auth = a
	msg.FeCompConsReq.CbteTipo = invoiceType.ARCACode()
	msg.FeCompConsReq.CbteNro = number
	msg.FeCompConsReq.PtoVta = pointOfSale
	body, err := xml.Marshal(msg)
	if err != nil {
		return nil, nil, fmt.Errorf("arca: encode FECompConsultar: %w", err)
	}
	raw, err := i.call(ctx, i.wsfeURL, wsfeNamespace+"FECompConsultar", "", body)
	if err != nil {
		return nil, nil, fmt.Errorf("arca: FECompConsultar: %w", err)
	}
	var env consultEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, nil, unavailable("decode FECompConsultar", err)
	}
	if len(env.Result.Errors) > 0 {
		return nil, nil, unavailable("FECompConsultar", errors.New(env.Result.Errors[0].String()))
	}
	got := env.Result.Get
	if got.Resultado != "A" || got.CodAutorizacion == "" {
		return nil, nil, unavailable("FECompConsultar", fmt.Errorf("invoice %d carries no authorization", number))
	}
	return &got, raw, nil
}

// authorized maps a consulted invoice to the domain's record of it.
func (c *consulted) authorized(number int64) (*domain.AuthorizedInvoice, error) {
	expires, err := time.ParseInLocation(arcaDate, c.FchVto, argentina)
	if err != nil {
		return nil, unavailable("decode FchVto", err)
	}
	date, err := time.ParseInLocation(arcaDate, c.CbteFch, argentina)
	if err != nil {
		return nil, unavailable("decode CbteFch", err)
	}
	total, err := decimal.NewFromString(c.ImpTotal)
	if err != nil {
		return nil, unavailable("decode ImpTotal", err)
	}
	docType, docNumber := domain.ReceiverDocNone, ""
	switch c.DocTipo {
	case domain.ReceiverDocCUIT.ARCACode():
		docType, docNumber = domain.ReceiverDocCUIT, c.DocNro
	case domain.ReceiverDocDNI.ARCACode():
		docType, docNumber = domain.ReceiverDocDNI, c.DocNro
	}
	return &domain.AuthorizedInvoice{
		Number:       number,
		CAE:          c.CodAutorizacion,
		CAEExpiresOn: expires,
		Date:         date,
		DocType:      docType,
		DocNumber:    docNumber,
		Total:        total,
	}, nil
}

// recoverIssued finds out whether a FECAESolicitar that failed in flight was authorized anyway,
// so a retry cannot issue the same sale twice.
func (i *Issuer) recoverIssued(ctx context.Context, a feAuth, req domain.InvoiceRequest, number int64, sent exchange, cause error) (*domain.InvoiceAuthorization, error) {
	last, err := i.lastAuthorized(ctx, a, req.Type, req.PointOfSale)
	if err != nil {
		return nil, outcomeUnknown(cause, err)
	}
	// ARCA only authorizes the number it was asked for, so a lower last number means it was not.
	if last < number {
		return nil, cause
	}
	got, raw, err := i.consult(ctx, a, req.Type, req.PointOfSale, number)
	if err != nil {
		return nil, outcomeUnknown(cause, err)
	}
	// The number may have gone to another system's invoice; only claim it if it is this one.
	det := buildSolicitar(req, number).FeCAEReq.FeDetReq.Det[0]
	total, err := decimal.NewFromString(got.ImpTotal)
	if err != nil || money(total) != det.ImpTotal || got.DocTipo != det.DocTipo || got.DocNro != det.DocNro {
		return nil, unavailable("recover FECAESolicitar", fmt.Errorf("invoice %d does not match the request: %w", number, cause))
	}
	expires, err := time.ParseInLocation(arcaDate, got.FchVto, argentina)
	if err != nil {
		return nil, outcomeUnknown(cause, err)
	}
	var observations []string
	for _, o := range got.Observaciones {
		observations = append(observations, o.String())
	}
	return &domain.InvoiceAuthorization{
		Number:       number,
		CAE:          got.CodAutorizacion,
		CAEExpiresOn: expires,
		Observations: observations,
		RawRequest:   sent.request,
		RawResponse:  raw,
	}, nil
}

// outcomeUnknown reports an invoice request that may have been authorized but could not be read back.
func outcomeUnknown(cause, readErr error) error {
	return fmt.Errorf("arca: FECAESolicitar failed in flight (%v) and the read-back failed (%v): %w", cause, readErr, domain.ErrInvoiceOutcomeUnknown)
}
