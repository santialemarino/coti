// Package pdf renders client-facing documents from immutable domain snapshots.
package pdf

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const (
	pageBottom = 273.0
	leftMargin = 15.0
	pageWidth  = 180.0
)

// QuoteRenderer renders a branded A4 document without external reads or monetary calculations.
type QuoteRenderer struct{}

// NewQuoteRenderer builds a QuoteRenderer.
func NewQuoteRenderer() *QuoteRenderer {
	return &QuoteRenderer{}
}

// Render returns one A4 PDF sourced exclusively from the canonical payload.
func (r *QuoteRenderer) Render(
	payload domain.QuoteRepresentationPayload, logo *domain.BrandLogo,
) ([]byte, error) {
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetMargins(leftMargin, 14, leftMargin)
	doc.SetAutoPageBreak(false, 15)
	doc.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	doc.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	doc.SetTitle("Cotización "+payload.Reference, true)
	doc.SetAuthor(payload.Supplier.Name, true)
	doc.SetCreationDate(payload.ApprovedAt)
	doc.SetModificationDate(payload.ApprovedAt)
	doc.SetCatalogSort(true)
	brandR, brandG, brandB := parseHexColor(payload.Supplier.BrandColor)
	doc.SetHeaderFunc(func() {
		doc.SetDrawColor(brandR, brandG, brandB)
		doc.SetLineWidth(1.2)
		doc.Line(leftMargin, 11, leftMargin+pageWidth, 11)
	})
	doc.SetFooterFunc(func() {
		doc.SetFont("Go", "", 7.5)
		doc.SetTextColor(90, 90, 90)
		doc.SetXY(leftMargin, 281)
		doc.CellFormat(pageWidth, 4, payload.ValidityNote, "", 1, "L", false, 0, "")
		doc.SetXY(leftMargin, 286)
		doc.CellFormat(pageWidth, 4, fmt.Sprintf("%s · Página %d", payload.Reference, doc.PageNo()),
			"", 0, "L", false, 0, "")
	})
	doc.AddPage()
	r.drawHeader(doc, payload, logo, brandR, brandG, brandB)
	r.drawQuoteDetails(doc, payload)
	r.drawItems(doc, payload, brandR, brandG, brandB)
	r.drawSummary(doc, payload, brandR, brandG, brandB)

	var output bytes.Buffer
	if err := doc.Output(&output); err != nil {
		return nil, err
	}
	if doc.Error() != nil {
		return nil, doc.Error()
	}
	return output.Bytes(), nil
}

func (r *QuoteRenderer) drawHeader(
	doc *gofpdf.Fpdf, payload domain.QuoteRepresentationPayload, logo *domain.BrandLogo,
	brandR, brandG, brandB int,
) {
	startY := 18.0
	if logo != nil {
		typeName := "PNG"
		if logo.ContentType == "image/jpeg" {
			typeName = "JPG"
		}
		options := gofpdf.ImageOptions{ImageType: typeName, ReadDpi: true}
		info := doc.RegisterImageOptionsReader("supplier-logo", options, bytes.NewReader(logo.Bytes))
		if info != nil {
			width, height := info.Extent()
			scale := min(34/width, 18/height)
			doc.ImageOptions("supplier-logo", leftMargin, startY, width*scale, height*scale, false, options, 0, "")
		}
	}
	doc.SetXY(54, startY)
	doc.SetFont("Go", "B", 17)
	doc.SetTextColor(25, 25, 25)
	doc.MultiCell(141, 7, payload.Supplier.Name, "", "R", false)
	doc.SetX(54)
	doc.SetFont("Go", "", 8.5)
	doc.SetTextColor(70, 70, 70)
	if payload.Supplier.LegalName != nil {
		doc.MultiCell(141, 5, *payload.Supplier.LegalName, "", "R", false)
	}
	if payload.Supplier.TaxID != nil {
		doc.SetX(54)
		doc.CellFormat(141, 5, "CUIT "+*payload.Supplier.TaxID, "", 1, "R", false, 0, "")
	}
	doc.SetY(max(42, doc.GetY()+5))
	doc.SetFont("Go", "B", 20)
	doc.SetTextColor(25, 25, 25)
	doc.CellFormat(pageWidth, 9, "Cotización", "", 1, "L", false, 0, "")
}

func (r *QuoteRenderer) drawQuoteDetails(
	doc *gofpdf.Fpdf, payload domain.QuoteRepresentationPayload,
) {
	location := time.FixedZone("America/Buenos_Aires", -3*60*60)
	approved := payload.ApprovedAt.In(location).Format("02/01/2006 15:04")
	doc.SetFont("Go", "", 9)
	doc.SetTextColor(55, 55, 55)
	doc.CellFormat(90, 6, "Referencia: "+payload.Reference, "", 0, "L", false, 0, "")
	doc.CellFormat(90, 6, "Versión: "+strconv.Itoa(payload.VersionNumber), "", 1, "R", false, 0, "")
	doc.CellFormat(90, 6, "Aprobada: "+approved, "", 0, "L", false, 0, "")
	doc.CellFormat(90, 6, "Moneda: "+payload.Currency, "", 1, "R", false, 0, "")
	branch := "Sucursal: " + payload.Branch.Name
	if payload.Branch.Address != nil {
		branch += " · " + *payload.Branch.Address
	}
	doc.MultiCell(pageWidth, 6, branch, "", "L", false)
	if payload.Customer.Name != nil {
		doc.MultiCell(pageWidth, 6, "Cliente: "+*payload.Customer.Name, "", "L", false)
	}
	doc.Ln(4)
}

func (r *QuoteRenderer) drawItems(
	doc *gofpdf.Fpdf, payload domain.QuoteRepresentationPayload, brandR, brandG, brandB int,
) {
	r.drawTableHeader(doc, brandR, brandG, brandB)
	for _, item := range payload.Items {
		label := item.ProductName
		if item.ProductCode != nil {
			label = *item.ProductCode + " · " + label
		}
		if strings.TrimSpace(item.RequestedDescription) != "" && item.RequestedDescription != item.ProductName {
			label += "\nPedido: " + item.RequestedDescription
		}
		unit := "-"
		if item.Unit != nil {
			unit = *item.Unit
		}
		r.drawRow(doc, []string{label, displayQuantity(item.Quantity), unit,
			commercialMoney(payload.Currency, item.UnitPrice),
			commercialMoney(payload.Currency, item.Subtotal)},
			[]float64{80, 16, 16, 34, 34}, []string{"L", "R", "C", "R", "R"},
			func() { r.drawTableHeader(doc, brandR, brandG, brandB) })
		for _, alternative := range item.Alternatives {
			r.drawAlternative(doc, payload.Currency, alternative, brandR, brandG, brandB)
		}
	}
}

func (r *QuoteRenderer) drawTableHeader(doc *gofpdf.Fpdf, brandR, brandG, brandB int) {
	doc.SetFillColor(238, 238, 238)
	doc.SetTextColor(25, 25, 25)
	doc.SetDrawColor(200, 200, 200)
	doc.SetLineWidth(0.2)
	doc.SetFont("Go", "B", 8)
	for _, cell := range []struct {
		width float64
		text  string
		align string
	}{{80, "Producto", "L"}, {16, "Cantidad", "R"}, {16, "Unidad", "C"},
		{34, "Precio unit.", "R"}, {34, "Subtotal", "R"}} {
		doc.CellFormat(cell.width, 7, cell.text, "", 0, cell.align, true, 0, "")
	}
	doc.Ln(7)
}

func (r *QuoteRenderer) drawRow(
	doc *gofpdf.Fpdf, values []string, widths []float64, alignments []string, header func(),
) {
	doc.SetFont("Go", "", 8)
	lines := make([][]string, len(values))
	lineCount := 1
	for i, value := range values {
		lines[i] = doc.SplitText(value, widths[i]-4)
		lineCount = max(lineCount, len(lines[i]))
	}
	for offset := 0; offset < lineCount; {
		r.ensureSpace(doc, 9, header)
		doc.SetFont("Go", "", 8)
		doc.SetTextColor(35, 35, 35)
		count := min(lineCount-offset, int((pageBottom-doc.GetY()-3)/4.5))
		height := float64(count)*4.5 + 3
		y, x := doc.GetY(), leftMargin
		for i, column := range lines {
			doc.Rect(x, y, widths[i], height, "D")
			for j := 0; j < count && offset+j < len(column); j++ {
				doc.SetXY(x+2, y+1.5+float64(j)*4.5)
				doc.CellFormat(widths[i]-4, 4.5, column[offset+j], "", 0,
					alignments[i], false, 0, "")
			}
			x += widths[i]
		}
		doc.SetXY(leftMargin, y+height)
		offset += count
	}
}

func (r *QuoteRenderer) drawAlternative(
	doc *gofpdf.Fpdf, currency string, alternative domain.QuoteRepresentationAlternative,
	brandR, brandG, brandB int,
) {
	name := "Alternativa aprobada: " + alternative.Name
	if alternative.Code != nil {
		name = "Alternativa aprobada: " + *alternative.Code + " · " + alternative.Name
	}
	if alternative.Unit != nil {
		name += " (" + *alternative.Unit + ")"
	}
	r.drawRow(doc, []string{name, commercialMoney(currency, alternative.UnitPrice)},
		[]float64{124, 56}, []string{"L", "R"},
		func() { r.drawTableHeader(doc, brandR, brandG, brandB) })
}

func (r *QuoteRenderer) drawSummary(
	doc *gofpdf.Fpdf, payload domain.QuoteRepresentationPayload, brandR, brandG, brandB int,
) {
	r.ensureSpace(doc, 18, func() {})
	doc.Ln(5)
	doc.SetFont("Go", "", 9)
	doc.SetTextColor(55, 55, 55)
	for _, discount := range payload.Discounts {
		description := discount.Description
		if description == "" {
			description = "Descuento"
		}
		r.drawRow(doc, []string{description, "- " + commercialMoney(payload.Currency, discount.Amount)},
			[]float64{125, 55}, []string{"R", "R"}, func() {})
	}
	r.ensureSpace(doc, 14, func() {})
	doc.SetDrawColor(brandR, brandG, brandB)
	doc.SetLineWidth(0.8)
	doc.Line(125, doc.GetY()+1, 195, doc.GetY()+1)
	doc.Ln(3)
	doc.SetFont("Go", "B", 13)
	doc.SetTextColor(25, 25, 25)
	doc.CellFormat(125, 8, "Total", "", 0, "R", false, 0, "")
	doc.CellFormat(55, 8, commercialMoney(payload.Currency, payload.Total),
		"", 1, "R", false, 0, "")
}

func (r *QuoteRenderer) ensureSpace(doc *gofpdf.Fpdf, height float64, header func()) {
	if doc.GetY()+height <= pageBottom {
		return
	}
	doc.AddPage()
	doc.SetY(18)
	header()
}

func parseHexColor(raw string) (int, int, int) {
	raw = strings.TrimPrefix(raw, "#")
	if len(raw) != 6 {
		return 31, 41, 55
	}
	value, err := strconv.ParseUint(raw, 16, 32)
	if err != nil {
		return 31, 41, 55
	}
	return int(value >> 16), int((value >> 8) & 0xff), int(value & 0xff)
}

func commercialMoney(currency, raw string) string {
	parts := strings.SplitN(raw, ".", 2)
	integer := parts[0]
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "." + integer[i:]
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return currency + " " + integer + "," + fraction
}

func displayQuantity(raw string) string {
	if !strings.Contains(raw, ".") {
		return raw
	}
	return strings.TrimSuffix(strings.TrimRight(raw, "0"), ".")
}
