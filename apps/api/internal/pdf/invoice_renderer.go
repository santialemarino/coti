package pdf

import (
	"bytes"
	"fmt"
	"time"

	"github.com/phpdave11/gofpdf"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// InvoiceRenderer renders authorized documents using the existing A4 document vocabulary.
type InvoiceRenderer struct{}

// NewInvoiceRenderer builds a fiscal document renderer.
func NewInvoiceRenderer() *InvoiceRenderer { return &InvoiceRenderer{} }

// Render returns an A4 invoice with its stored fiscal identity, amounts, CAE and verification QR.
func (r *InvoiceRenderer) Render(inv domain.Invoice) ([]byte, error) {
	if inv.Status != domain.InvoiceStatusIssued || inv.Number == nil || inv.CAE == nil || inv.CAEExpiresOn == nil {
		return nil, domain.ErrInvalidInput
	}
	verification := domain.InvoiceQRURL(inv)
	if verification == "" {
		return nil, domain.ErrInvalidInput
	}
	qr, err := qrcode.Encode(verification, qrcode.Medium, 256)
	if err != nil {
		return nil, err
	}
	payload := inv.Snapshot
	ref := fmt.Sprintf("%05d-%08d", inv.PointOfSale, *inv.Number)
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetMargins(leftMargin, 14, leftMargin)
	doc.SetAutoPageBreak(false, 15)
	doc.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	doc.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	doc.SetTitle("Factura "+string(inv.Type)+" "+ref, true)
	doc.SetAuthor(payload.Supplier.Name, true)
	doc.SetCreationDate(inv.CreatedAt)
	doc.SetModificationDate(inv.CreatedAt)
	doc.SetCatalogSort(true)
	red, green, blue := parseHexColor(payload.Supplier.BrandColor)
	doc.SetHeaderFunc(func() {
		doc.SetDrawColor(red, green, blue)
		doc.SetLineWidth(1.2)
		doc.Line(leftMargin, 11, leftMargin+pageWidth, 11)
	})
	doc.SetFooterFunc(func() {
		doc.SetXY(leftMargin, 284)
		doc.SetFont("Go", "", 8)
		doc.CellFormat(pageWidth, 5, fmt.Sprintf("Factura %s %s · Página %d", inv.Type, ref, doc.PageNo()), "", 0, "R", false, 0, "")
	})
	doc.AddPage()
	doc.SetFont("Go", "B", 18)
	doc.MultiCell(pageWidth, 8, "Factura "+string(inv.Type)+" · "+ref, "", "L", false)
	doc.SetFont("Go", "", 9)
	name := payload.Supplier.Name
	if payload.Supplier.LegalName != nil {
		name = *payload.Supplier.LegalName
	}
	for _, line := range []string{name, "CUIT: " + inv.IssuerCUIT, invoiceCondition(inv.IssuerCondition), "Domicilio comercial: " + inv.IssuerProfile.Address, "Ingresos brutos: " + inv.IssuerProfile.GrossIncomeRegistration, "Inicio de actividades: " + invoiceActivityDate(inv.IssuerProfile.ActivityStartedOn), "Fecha de emisión: " + inv.IssuedOn.Format("02/01/2006"), "Comprobante código " + fmt.Sprintf("%03d", inv.Type.ARCACode()) + " · ORIGINAL"} {
		doc.MultiCell(pageWidth, 5, line, "", "L", false)
	}
	doc.Ln(4)
	for _, line := range []string{"Cliente: " + inv.Receiver.Name, "Domicilio: " + inv.Receiver.Address, string(inv.Receiver.DocType) + ": " + inv.Receiver.DocNumber, invoiceCondition(inv.Receiver.IVACondition)} {
		doc.MultiCell(pageWidth, 5, line, "", "L", false)
	}
	doc.Ln(5)
	NewQuoteRenderer().drawItems(doc, payload, red, green, blue)
	if doc.GetY()+85 > pageBottom {
		doc.AddPage()
	}
	doc.SetFont("Go", "", 9)
	for _, discount := range payload.Discounts {
		doc.MultiCell(pageWidth, 5, discount.Description+": -"+commercialMoney("ARS", discount.Amount), "", "L", false)
	}
	for _, row := range []struct{ label, value string }{{"Importe neto", inv.Amounts.Net.StringFixed(2)}, {"Importe exento", inv.Amounts.Exempt.StringFixed(2)}, {invoiceVATLabel(inv.Type), inv.Amounts.VAT.StringFixed(2)}} {
		doc.CellFormat(pageWidth, 6, row.label+": "+commercialMoney("ARS", row.value), "", 1, "R", false, 0, "")
	}
	doc.SetFont("Go", "B", 14)
	doc.CellFormat(pageWidth, 8, "Total: "+commercialMoney("ARS", inv.Amounts.Total.StringFixed(2)), "", 1, "R", false, 0, "")
	if inv.Type == domain.InvoiceTypeA && inv.Receiver.IVACondition == domain.IVAConditionMonotributo {
		doc.SetFont("Go", "", 8)
		doc.MultiCell(pageWidth, 4, "El crédito fiscal discriminado en el presente comprobante, sólo podrá ser computado a efectos del Régimen de Sostenimiento e Inclusión Fiscal para Pequeños Contribuyentes de la Ley Nº 27.618.", "", "L", false)
	}
	doc.Ln(4)
	y := doc.GetY()
	doc.RegisterImageOptionsReader("arca-qr", gofpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(qr))
	doc.ImageOptions("arca-qr", leftMargin, y, 32, 32, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, verification)
	doc.SetXY(52, y+3)
	doc.SetFont("Go", "", 9)
	doc.CellFormat(140, 6, "CAE: "+*inv.CAE, "", 1, "L", false, 0, "")
	doc.SetX(52)
	doc.CellFormat(140, 6, "Vencimiento del CAE: "+inv.CAEExpiresOn.Format("02/01/2006"), "", 1, "L", false, 0, "")
	doc.SetX(52)
	doc.MultiCell(140, 5, "Escaneá el QR para consultar el comprobante en ARCA.", "", "L", false)
	var result bytes.Buffer
	if err := doc.Output(&result); err != nil {
		return nil, err
	}
	return result.Bytes(), doc.Error()
}

func invoiceCondition(condition domain.IVACondition) string {
	switch condition {
	case domain.IVAConditionRegistered:
		return "IVA Responsable Inscripto"
	case domain.IVAConditionMonotributo:
		return "Responsable Monotributo"
	case domain.IVAConditionExempt:
		return "IVA Exento"
	default:
		return "Consumidor Final"
	}
}

func invoiceActivityDate(raw string) string {
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return raw
	}
	return day.Format("02/01/2006")
}

func invoiceVATLabel(kind domain.InvoiceType) string {
	if kind == domain.InvoiceTypeB {
		return "IVA contenido"
	}
	return "IVA"
}
