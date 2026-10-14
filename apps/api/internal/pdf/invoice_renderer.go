package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

const invoiceContentBottom = 236.0

// TestInvoiceMark labels every page of an invoice authorized in ARCA's homologation environment.
const TestInvoiceMark = "COMPROBANTE DE PRUEBA — SIN VALIDEZ FISCAL"

// InvoiceRenderer renders branded fiscal documents from authorized snapshots.
type InvoiceRenderer struct{}

// NewInvoiceRenderer builds a fiscal document renderer.
func NewInvoiceRenderer() *InvoiceRenderer { return &InvoiceRenderer{} }

// Render returns a paginated A4 invoice with its stored identity, amounts and authorization.
func (r *InvoiceRenderer) Render(inv domain.Invoice, logo *domain.BrandLogo) ([]byte, error) {
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
	ref := fmt.Sprintf("%05d-%08d", inv.PointOfSale, *inv.Number)
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetMargins(leftMargin, 15, leftMargin)
	doc.SetAutoPageBreak(false, 15)
	doc.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	doc.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	doc.SetTitle("Factura "+string(inv.Type)+" "+ref, true)
	doc.SetAuthor(inv.Snapshot.Supplier.Name, true)
	if isTestInvoice(inv) {
		doc.SetSubject(TestInvoiceMark, true)
	}
	doc.SetCreationDate(inv.CreatedAt)
	doc.SetModificationDate(inv.CreatedAt)
	doc.SetCatalogSort(true)
	doc.AliasNbPages("")
	red, green, blue := parseHexColor(inv.Snapshot.Supplier.BrandColor)
	doc.RegisterImageOptionsReader("arca-qr", gofpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(qr))
	doc.SetHeaderFunc(func() { r.drawInvoiceHeader(doc, inv, logo, ref, red, green, blue) })
	doc.SetFooterFunc(func() { r.drawInvoiceFooter(doc, inv, ref, verification) })
	doc.AddPage()
	r.drawInvoiceTableHeader(doc, red, green, blue)
	for _, item := range inv.Snapshot.Items {
		label := item.ProductName
		if item.ProductCode != nil {
			label = *item.ProductCode + " · " + label
		}
		unit := "-"
		if item.Unit != nil {
			unit = *item.Unit
		}
		r.drawInvoiceRow(doc, []string{displayQuantity(item.Quantity), unit, label, commercialMoney(inv.Currency, item.UnitPrice), commercialMoney(inv.Currency, item.Subtotal)}, red, green, blue)
	}
	r.drawInvoiceTotals(doc, inv, red, green, blue)
	var output bytes.Buffer
	if err := doc.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), doc.Error()
}

func (r *InvoiceRenderer) drawInvoiceHeader(doc *gofpdf.Fpdf, inv domain.Invoice, logo *domain.BrandLogo, ref string, red, green, blue int) {
	if isTestInvoice(inv) {
		drawTestInvoiceBand(doc)
	}
	doc.SetTextColor(35, 35, 35)
	doc.SetDrawColor(185, 185, 185)
	doc.SetLineWidth(.2)
	doc.Rect(leftMargin, 15, pageWidth, 263, "D")
	doc.SetDrawColor(red, green, blue)
	doc.SetLineWidth(1)
	doc.Line(leftMargin, 15, leftMargin+pageWidth, 15)
	doc.SetLineWidth(.2)
	doc.SetDrawColor(185, 185, 185)
	y := 23.0
	if logo != nil {
		kind := "PNG"
		if logo.ContentType == "image/jpeg" {
			kind = "JPG"
		}
		opts := gofpdf.ImageOptions{ImageType: kind}
		info := doc.RegisterImageOptionsReader("supplier-logo", opts, bytes.NewReader(logo.Bytes))
		if info != nil {
			w, h := info.Extent()
			scale := min(62/w, 16/h)
			doc.ImageOptions("supplier-logo", 20, y, w*scale, h*scale, false, opts, 0, "")
			y += 20
		}
	}
	doc.SetXY(20, y)
	doc.SetFont("Go", "B", 11)
	doc.MultiCell(76, 5, inv.Snapshot.Supplier.Name, "", "L", false)
	doc.SetFont("Go", "", 8)
	name := inv.Snapshot.Supplier.Name
	if inv.Snapshot.Supplier.LegalName != nil {
		name = *inv.Snapshot.Supplier.LegalName
	}
	if name != inv.Snapshot.Supplier.Name {
		doc.SetX(20)
		doc.MultiCell(76, 4, name, "", "L", false)
	}
	for _, line := range []string{inv.IssuerProfile.Address, invoiceCondition(inv.IssuerCondition), "Sucursal: " + inv.Snapshot.Branch.Name} {
		doc.SetX(20)
		doc.MultiCell(76, 4, line, "", "L", false)
	}
	leftBottom := doc.GetY() + 5
	doc.SetXY(118, 23)
	doc.SetFont("Go", "B", 18)
	doc.CellFormat(72, 8, "FACTURA", "", 1, "L", false, 0, "")
	doc.SetX(118)
	doc.SetFont("Go", "B", 11)
	doc.CellFormat(72, 7, ref, "", 1, "L", false, 0, "")
	doc.SetFont("Go", "", 8)
	for _, line := range []string{"Fecha de emisión: " + inv.IssuedOn.Format("02/01/2006"), "CUIT: " + inv.IssuerCUIT, "Ingresos Brutos: " + invoiceGrossIncome(inv.IssuerProfile.GrossIncomeRegistration), "Inicio de actividades: " + invoiceActivityDate(inv.IssuerProfile.ActivityStartedOn), "Moneda: " + inv.Currency} {
		doc.SetX(118)
		doc.MultiCell(72, 4.5, line, "", "L", false)
	}
	bottom := max(leftBottom, doc.GetY()+5)
	doc.Line(105, 15, 105, bottom)
	doc.SetFillColor(255, 255, 255)
	doc.Rect(98, 15, 14, 19, "DF")
	doc.SetXY(98, 17)
	doc.SetFont("Go", "B", 20)
	doc.CellFormat(14, 9, string(inv.Type), "", 1, "C", false, 0, "")
	doc.SetX(98)
	doc.SetFont("Go", "", 6)
	doc.CellFormat(14, 4, fmt.Sprintf("Cód. %03d", inv.Type.ARCACode()), "", 0, "C", false, 0, "")
	doc.SetXY(118, 17)
	doc.SetFont("Go", "", 7)
	doc.CellFormat(72, 4, "ORIGINAL", "", 0, "R", false, 0, "")
	doc.Line(leftMargin, bottom, leftMargin+pageWidth, bottom)
	doc.SetXY(20, bottom+4)
	doc.SetFont("Go", "B", 8)
	doc.MultiCell(90, 4, "Cliente: "+inv.Receiver.Name, "", "L", false)
	doc.SetX(20)
	doc.SetFont("Go", "", 8)
	doc.MultiCell(90, 4, "Domicilio: "+inv.Receiver.Address, "", "L", false)
	receiverBottom := doc.GetY() + 4
	doc.SetXY(118, bottom+4)
	doc.MultiCell(72, 4, invoiceCondition(inv.Receiver.IVACondition), "", "L", false)
	doc.SetX(118)
	doc.MultiCell(72, 4, string(inv.Receiver.DocType)+": "+inv.Receiver.DocNumber, "", "L", false)
	receiverBottom = max(receiverBottom, doc.GetY()+4)
	doc.Line(leftMargin, receiverBottom, leftMargin+pageWidth, receiverBottom)
	doc.SetXY(leftMargin, receiverBottom+5)
}

func (r *InvoiceRenderer) drawInvoiceTableHeader(doc *gofpdf.Fpdf, red, green, blue int) {
	doc.SetFillColor(242, 242, 242)
	doc.SetTextColor(35, 35, 35)
	doc.SetFont("Go", "B", 8)
	for i, label := range []string{"Cantidad", "Unidad", "Detalle", "Precio unit.", "Importe"} {
		doc.CellFormat([]float64{18, 18, 76, 34, 34}[i], 7, label, "", 0, []string{"R", "C", "L", "R", "R"}[i], true, 0, "")
	}
	doc.Ln(7)
	doc.SetDrawColor(red, green, blue)
	doc.Line(leftMargin, doc.GetY(), 195, doc.GetY())
}

func (r *InvoiceRenderer) drawInvoiceRow(doc *gofpdf.Fpdf, values []string, red, green, blue int) {
	widths := []float64{18, 18, 76, 34, 34}
	aligns := []string{"R", "C", "L", "R", "R"}
	doc.SetFont("Go", "", 8)
	columns := make([][]string, len(values))
	total := 1
	for i, value := range values {
		columns[i] = doc.SplitText(value, widths[i]-4)
		total = max(total, len(columns[i]))
	}
	for offset := 0; offset < total; {
		if doc.GetY()+8 > invoiceContentBottom {
			doc.AddPage()
			r.drawInvoiceTableHeader(doc, red, green, blue)
		}
		doc.SetFont("Go", "", 8)
		doc.SetTextColor(35, 35, 35)
		count := min(total-offset, int((invoiceContentBottom-doc.GetY()-3)/4))
		height := float64(count)*4 + 3
		y, x := doc.GetY(), leftMargin
		for i, column := range columns {
			for j := 0; j < count && offset+j < len(column); j++ {
				doc.SetXY(x+2, y+1.5+float64(j)*4)
				doc.CellFormat(widths[i]-4, 4, column[offset+j], "", 0, aligns[i], false, 0, "")
			}
			x += widths[i]
		}
		doc.SetDrawColor(225, 225, 225)
		doc.Line(leftMargin, y+height, 195, y+height)
		doc.SetXY(leftMargin, y+height)
		offset += count
	}
}

func (r *InvoiceRenderer) drawInvoiceTotals(doc *gofpdf.Fpdf, inv domain.Invoice, red, green, blue int) {
	reserve := 43.0
	if inv.Type == domain.InvoiceTypeA && inv.Receiver.IVACondition == domain.IVAConditionMonotributo {
		reserve += 16
	}
	if doc.GetY()+reserve > invoiceContentBottom {
		doc.AddPage()
	}
	doc.Ln(5)
	doc.SetFont("Go", "", 8)
	for _, discount := range inv.Snapshot.Discounts {
		label := strings.TrimSpace(discount.Description)
		if label == "" {
			label = "Descuento"
		}
		if doc.GetY()+reserve > invoiceContentBottom {
			doc.AddPage()
		}
		doc.SetX(leftMargin)
		doc.MultiCell(pageWidth, 4, label+": -"+commercialMoney(inv.Currency, discount.Amount), "", "R", false)
	}
	for _, row := range []struct{ label, value string }{{"Importe neto", inv.Amounts.Net.StringFixed(2)}, {"Importe exento", inv.Amounts.Exempt.StringFixed(2)}, {invoiceVATLabel(inv.Type), inv.Amounts.VAT.StringFixed(2)}} {
		doc.SetX(107)
		doc.CellFormat(48, 6, row.label, "", 0, "L", false, 0, "")
		doc.CellFormat(40, 6, commercialMoney(inv.Currency, row.value), "", 1, "R", false, 0, "")
	}
	doc.SetDrawColor(red, green, blue)
	doc.SetLineWidth(.7)
	doc.Line(107, doc.GetY()+1, 195, doc.GetY()+1)
	doc.Ln(3)
	doc.SetX(107)
	doc.SetFont("Go", "B", 12)
	doc.CellFormat(38, 9, "TOTAL", "", 0, "L", false, 0, "")
	doc.CellFormat(50, 9, commercialMoney(inv.Currency, inv.Amounts.Total.StringFixed(2)), "", 1, "R", false, 0, "")
	if inv.Type == domain.InvoiceTypeA && inv.Receiver.IVACondition == domain.IVAConditionMonotributo {
		doc.SetX(leftMargin)
		doc.SetFont("Go", "", 7)
		doc.MultiCell(pageWidth, 4, "El crédito fiscal discriminado en el presente comprobante, sólo podrá ser computado a efectos del Régimen de Sostenimiento e Inclusión Fiscal para Pequeños Contribuyentes de la Ley Nº 27.618.", "", "L", false)
	}
}

func (r *InvoiceRenderer) drawInvoiceFooter(doc *gofpdf.Fpdf, inv domain.Invoice, ref, verification string) {
	doc.SetDrawColor(185, 185, 185)
	doc.SetLineWidth(.2)
	doc.Line(leftMargin, 242, 195, 242)
	doc.ImageOptions("arca-qr", 20, 246, 27, 27, false, gofpdf.ImageOptions{ImageType: "PNG"}, 0, verification)
	doc.SetTextColor(35, 35, 35)
	doc.SetFont("Go", "B", 9)
	doc.SetXY(53, 248)
	doc.CellFormat(137, 6, "CAE: "+*inv.CAE, "", 1, "L", false, 0, "")
	doc.SetFont("Go", "", 8)
	doc.SetX(53)
	doc.CellFormat(137, 5, "Vencimiento del CAE: "+inv.CAEExpiresOn.Format("02/01/2006"), "", 1, "L", false, 0, "")
	doc.SetX(53)
	doc.CellFormat(137, 5, "Consultá el comprobante en ARCA mediante el QR.", "", 1, "L", false, 0, "")
	doc.SetXY(leftMargin, 281)
	doc.SetFont("Go", "", 7)
	doc.CellFormat(pageWidth, 4, fmt.Sprintf("Factura %s %s · Página %d de {nb}", inv.Type, ref, doc.PageNo()), "", 0, "R", false, 0, "")
}

// isTestInvoice reports whether ARCA authorized inv anywhere but production, where nothing is fiscal.
func isTestInvoice(inv domain.Invoice) bool {
	return inv.Environment != domain.ARCAEnvironmentProduction
}

// drawTestInvoiceBand fills the top margin, above the fiscal frame, so the mark never covers its data.
func drawTestInvoiceBand(doc *gofpdf.Fpdf) {
	doc.SetFillColor(196, 30, 58)
	doc.Rect(leftMargin, 5, pageWidth, 8, "F")
	doc.SetTextColor(255, 255, 255)
	doc.SetFont("Go", "B", 10)
	doc.SetXY(leftMargin, 5)
	doc.CellFormat(pageWidth, 8, TestInvoiceMark, "", 0, "C", false, 0, "")
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

func invoiceGrossIncome(registration string) string {
	if strings.EqualFold(registration, domain.GrossIncomeExempt) {
		return "Exento"
	}
	return registration
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
