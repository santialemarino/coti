package domain

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
)

// invoiceQRBase is where ARCA verifies an invoice from its printed QR code (RG 4892).
const invoiceQRBase = "https://www.arca.gob.ar/fe/qr/?p="

// invoiceQRPayload is the QR's JSON, version 1 of ARCA's specification.
type invoiceQRPayload struct {
	Ver        int         `json:"ver"`
	Fecha      string      `json:"fecha"`
	CUIT       int64       `json:"cuit"`
	PtoVta     int         `json:"ptoVta"`
	TipoCmp    int         `json:"tipoCmp"`
	NroCmp     int64       `json:"nroCmp"`
	Importe    json.Number `json:"importe"`
	Moneda     string      `json:"moneda"`
	Ctz        json.Number `json:"ctz"`
	TipoDocRec *int        `json:"tipoDocRec,omitempty"`
	NroDocRec  *int64      `json:"nroDocRec,omitempty"`
	TipoCodAut string      `json:"tipoCodAut"`
	CodAut     int64       `json:"codAut"`
}

// InvoiceQRURL is the verification link an issued invoice prints as its QR code, or empty for an
// invoice ARCA has not authorized.
func InvoiceQRURL(inv Invoice) string {
	if inv.Status != InvoiceStatusIssued || inv.Number == nil || inv.CAE == nil {
		return ""
	}
	cuit, err := strconv.ParseInt(inv.IssuerCUIT, 10, 64)
	if err != nil {
		return ""
	}
	cae, err := strconv.ParseInt(*inv.CAE, 10, 64)
	if err != nil {
		return ""
	}
	payload := invoiceQRPayload{
		Ver: 1, Fecha: inv.IssuedOn.Format("2006-01-02"), CUIT: cuit, PtoVta: inv.PointOfSale,
		TipoCmp: inv.Type.ARCACode(), NroCmp: *inv.Number, Importe: json.Number(inv.Amounts.Total.StringFixed(2)),
		Moneda: "PES", Ctz: json.Number("1"), TipoCodAut: "E", CodAut: cae,
	}
	if inv.Receiver.DocType != ReceiverDocNone {
		docType := inv.Receiver.DocType.ARCACode()
		payload.TipoDocRec = &docType
		if number, err := strconv.ParseInt(inv.Receiver.DocNumber, 10, 64); err == nil {
			payload.NroDocRec = &number
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return invoiceQRBase + base64.StdEncoding.EncodeToString(body)
}
