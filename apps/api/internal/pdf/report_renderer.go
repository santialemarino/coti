package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/phpdave11/gofpdf"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

//go:embed assets/coti-isotype.png
var cotiIsotype []byte

//go:embed assets/coti-wordmark.png
var cotiWordmark []byte

const (
	reportSectionGap = 4.0
	reportCardRadius = 2.0
)

var (
	reportInk       = [3]int{26, 39, 60}
	reportMuted     = [3]int{100, 116, 139}
	reportSubtle    = [3]int{148, 163, 184}
	reportBorder    = [3]int{226, 232, 240}
	reportSurface   = [3]int{248, 250, 252}
	reportBrand     = [3]int{47, 108, 179}
	reportBrandSoft = [3]int{235, 245, 255}
	reportAccepted  = [3]int{0, 182, 155}
	reportWarning   = [3]int{255, 167, 86}
	reportDanger    = [3]int{239, 56, 38}
	reportQuoted    = [3]int{255, 204, 0}
)

// SellerReportRenderer renders the seller dashboard snapshot as an A4 PDF.
type SellerReportRenderer struct{}

// NewSellerReportRenderer builds a SellerReportRenderer.
func NewSellerReportRenderer() *SellerReportRenderer {
	return &SellerReportRenderer{}
}

// Render returns one A4 PDF sourced exclusively from the canonical report snapshot.
func (r *SellerReportRenderer) Render(report domain.SellerReport, generatedAt time.Time) ([]byte, error) {
	doc := gofpdf.New("P", "mm", "A4", "")
	doc.SetMargins(leftMargin, 14, leftMargin)
	doc.SetAutoPageBreak(false, 15)
	doc.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	doc.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	doc.SetTitle("Reporte de rendimiento comercial", true)
	doc.SetSubject("Reporte de rendimiento comercial de Coti", true)
	doc.SetAuthor("Coti", true)
	doc.SetCreator("Coti", true)
	doc.SetCreationDate(generatedAt)
	doc.SetModificationDate(generatedAt)
	doc.SetCatalogSort(true)
	doc.SetHeaderFunc(func() {
		doc.SetDrawColor(reportBrand[0], reportBrand[1], reportBrand[2])
		doc.SetLineWidth(1.2)
		doc.Line(leftMargin, 11, leftMargin+pageWidth, 11)
	})
	doc.SetFooterFunc(func() {
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(leftMargin, 282)
		doc.CellFormat(pageWidth/2, 4, "Coti · Reporte de rendimiento comercial", "", 0, "L", false, 0, "")
		doc.SetXY(leftMargin+pageWidth/2, 282)
		doc.CellFormat(pageWidth/2, 4, fmt.Sprintf("Página %d", doc.PageNo()), "", 0, "R", false, 0, "")
	})

	doc.AddPage()
	r.drawReportHeader(doc, report, generatedAt)
	r.drawKPIs(doc, report.KPIs)
	r.drawStatusAndTimeline(doc, report)

	y := r.nextSectionY(doc, 66)
	if y+66 > pageBottom {
		r.newContentPage(doc, report, generatedAt)
		y = reportPageTop
	}
	y = r.drawRankings(doc, report, y)
	if y+reportSectionGap+r.clientsHeight(report) <= reportContentBottom {
		r.drawClients(doc, report, y+reportSectionGap)
		r.newContentPage(doc, report, generatedAt)
		y = doc.GetY()
	} else {
		r.newContentPage(doc, report, generatedAt)
		y = r.drawClients(doc, report, doc.GetY())
	}
	y = r.drawAIMatch(doc, report, y+reportSectionGap)
	if report.Scope.MultiBranch {
		y = r.drawBranches(doc, report, generatedAt, y+reportSectionGap)
	}
	r.drawMethodology(doc, report, generatedAt, y+reportSectionGap)

	var output bytes.Buffer
	if err := doc.Output(&output); err != nil {
		return nil, err
	}
	if doc.Error() != nil {
		return nil, doc.Error()
	}
	return output.Bytes(), nil
}

const (
	reportPageTop       = 20.0
	reportContentBottom = 270.0
	reportKPIHeight     = 26.0
	reportKPIWidth      = 57.33
)

func (r *SellerReportRenderer) newContentPage(doc *gofpdf.Fpdf, report domain.SellerReport, generatedAt time.Time) {
	doc.AddPage()
	doc.SetY(reportPageTop)
	doc.SetFont("Go", "B", 10)
	doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
	doc.CellFormat(pageWidth, 6, "Reporte de rendimiento comercial", "", 1, "L", false, 0, "")
	doc.SetFont("Go", "", 7)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.CellFormat(pageWidth, 4, r.reportPeriodLabel(report)+" · Generado "+r.reportTimestamp(generatedAt), "", 1, "R", false, 0, "")
	doc.SetY(reportPageTop + 12)
}

func (r *SellerReportRenderer) drawReportHeader(doc *gofpdf.Fpdf, report domain.SellerReport, generatedAt time.Time) {
	startY := 19.0
	imageOptions := gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
	if info := doc.RegisterImageOptionsReader("coti-isotype", imageOptions,
		bytes.NewReader(cotiIsotype)); info != nil {
		width, height := info.Extent()
		scale := min(10/width, 10.5/height)
		doc.ImageOptions("coti-isotype", leftMargin, startY, width*scale, height*scale,
			false, imageOptions, 0, "")
	}
	if info := doc.RegisterImageOptionsReader("coti-wordmark", imageOptions,
		bytes.NewReader(cotiWordmark)); info != nil {
		width, height := info.Extent()
		scale := min(23/width, 8.8/height)
		doc.ImageOptions("coti-wordmark", leftMargin+12, startY+1.5, width*scale,
			height*scale, false, imageOptions, 0, "")
	}

	doc.SetFont("Go", "B", 9)
	doc.SetTextColor(reportBrand[0], reportBrand[1], reportBrand[2])
	doc.SetXY(leftMargin, startY+12)
	doc.CellFormat(pageWidth, 5, "REPORTE DE RENDIMIENTO COMERCIAL", "", 1, "L", false, 0, "")

	doc.SetFont("Go", "", 7.5)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(leftMargin, startY+18)
	doc.CellFormat(pageWidth*0.62, 4.5, "Período: "+r.reportPeriodLabel(report), "", 0, "L", false, 0, "")
	doc.CellFormat(pageWidth*0.38, 4.5, "Generado: "+r.reportTimestamp(generatedAt), "", 1, "R", false, 0, "")
	doc.SetX(leftMargin)
	doc.CellFormat(pageWidth, 4.5, r.reportScopeLabel(report), "", 1, "L", false, 0, "")
	if filters := r.reportFiltersLabel(report); filters != "" {
		doc.SetX(leftMargin)
		doc.CellFormat(pageWidth, 4.5, r.fitText(doc, filters, pageWidth), "", 1, "L", false, 0, "")
		doc.SetY(startY + 33)
		return
	}
	doc.SetY(startY + 28)
}

func (r *SellerReportRenderer) drawKPIs(doc *gofpdf.Fpdf, kpis domain.SellerReportKPIs) {
	conversionDetail := fmt.Sprintf("%s de %s enviadas.",
		reportCount(kpis.SentAccepted), reportCount(kpis.QuotesSent))
	values := []struct {
		label  string
		value  string
		detail string
	}{
		{"Pedidos analizados", reportCount(kpis.RFQsReceived), "Órdenes recibidas en el período."},
		{"Cotizaciones enviadas", reportCount(kpis.QuotesSent), "Envíos comerciales realizados."},
		{"Cotizaciones aceptadas", reportCount(kpis.QuotesAccepted), "Aceptaciones registradas."},
		{"Tasa de aceptación", reportPercent(kpis.ConversionRate), conversionDetail},
		{"Importe aceptado", reportMoney(kpis.AcceptedAmount), "Sobre la versión aceptada."},
		{"Tiempo de respuesta", reportDuration(kpis.ResponseTime.AverageSeconds), r.responseDetail(kpis.ResponseTime)},
	}

	startY := doc.GetY()
	for index, item := range values {
		column := index % 3
		row := index / 3
		x := leftMargin + float64(column)*(reportKPIWidth+reportSectionGap)
		y := startY + float64(row)*(reportKPIHeight+reportSectionGap)
		doc.SetFillColor(reportSurface[0], reportSurface[1], reportSurface[2])
		doc.SetDrawColor(reportBorder[0], reportBorder[1], reportBorder[2])
		doc.SetLineWidth(0.2)
		doc.RoundedRect(x, y, reportKPIWidth, reportKPIHeight, reportCardRadius, "1234", "FD")
		doc.SetFillColor(reportBrand[0], reportBrand[1], reportBrand[2])
		doc.RoundedRect(x+6, y+6, 1.4, 6, 0.7, "1234", "F")
		doc.SetFont("Go", "B", 7)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(x+10, y+5.5)
		doc.CellFormat(reportKPIWidth-16, 4, item.label, "", 1, "L", false, 0, "")
		doc.SetFont("Go", "B", 14)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(x+6, y+12)
		doc.CellFormat(reportKPIWidth-12, 6, item.value, "", 1, "L", false, 0, "")
		doc.SetFont("Go", "", 6)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(x+6, y+20)
		doc.CellFormat(reportKPIWidth-12, 3.5, item.detail, "", 0, "L", false, 0, "")
	}
	doc.SetY(startY + 2*reportKPIHeight + reportSectionGap)
}

func (r *SellerReportRenderer) drawStatusAndTimeline(doc *gofpdf.Fpdf, report domain.SellerReport) {
	startY := doc.GetY() + 2
	width := 100.0
	height := 62.0
	r.drawCard(doc, leftMargin, startY, width, height, "Cotizaciones por estado", r.statusSummary(report))
	r.drawStatusPie(doc, report.QuotesByStatus, leftMargin+8, startY+17, width-16)
	timelineX := leftMargin + width + reportSectionGap
	timelineWidth := pageWidth - width - reportSectionGap
	r.drawCard(doc, timelineX, startY, timelineWidth, height, "Actividad de cotizaciones",
		r.granularityLabel(report.Granularity))
	r.drawTimeline(doc, report.Timeline, report.Granularity, timelineX+6, startY+18,
		timelineWidth-12, 36)
	doc.SetY(startY + height)
}

func (r *SellerReportRenderer) drawStatusPie(
	doc *gofpdf.Fpdf, items []domain.SellerReportStatusCount, x, y, width float64,
) {
	total := 0
	for _, item := range items {
		total += item.Count
	}
	if total == 0 {
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(x, y+16)
		doc.CellFormat(width, 6, "Sin datos en el período.", "", 0, "C", false, 0, "")
		return
	}

	centerX := x + 19
	centerY := y + 20
	radius := 17.0
	start := -90.0
	visible := make([]domain.SellerReportStatusCount, 0, len(items))
	for _, item := range items {
		if item.Count > 0 {
			visible = append(visible, item)
		}
	}
	if len(visible) == 1 {
		doc.SetFillColor(r.statusColor(visible[0].Status))
		doc.Circle(centerX, centerY, radius, "F")
	} else {
		for _, item := range visible {
			sweep := float64(item.Count) / float64(total) * 360
			doc.SetFillColor(r.statusColor(item.Status))
			doc.Polygon(r.piePoints(centerX, centerY, radius, start, sweep), "F")
			start += sweep
		}
	}
	doc.SetFillColor(255, 255, 255)
	doc.Circle(centerX, centerY, radius*0.55, "F")
	doc.SetFont("Go", "B", 8)
	doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
	doc.SetXY(centerX-11, centerY-3)
	doc.CellFormat(22, 4.5, reportCount(total), "", 0, "C", false, 0, "")
	doc.SetFont("Go", "", 4.8)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(centerX-11, centerY+1.5)
	doc.CellFormat(22, 3, "cotiz.", "", 0, "C", false, 0, "")

	legendX := x + 39
	legendY := y + 1
	doc.SetFont("Go", "", 6.3)
	row := 0
	for _, item := range items {
		if item.Count == 0 {
			continue
		}
		rowY := legendY + float64(row)*7.2
		row++
		doc.SetFillColor(r.statusColor(item.Status))
		doc.RoundedRect(legendX, rowY+1, 2.2, 2.2, 0.6, "1234", "F")
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(legendX+4, rowY)
		doc.CellFormat(20, 4.5, r.statusLabel(item.Status), "", 0, "L", false, 0, "")
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(legendX+24, rowY)
		doc.CellFormat(10, 4.5, reportCount(item.Count), "", 0, "R", false, 0, "")
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(legendX+34, rowY)
		doc.CellFormat(11, 4.5, reportShare(item.Count, total), "", 0, "R", false, 0, "")
	}
}

func (r *SellerReportRenderer) drawTimeline(
	doc *gofpdf.Fpdf, points []domain.SellerReportTimelinePoint,
	granularity domain.ReportGranularity, x, y, width, height float64,
) {
	doc.SetFont("Go", "", 5.8)
	if len(points) == 0 {
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(x, y+height/2)
		doc.CellFormat(width, 6, "Sin datos en el período.", "", 0, "C", false, 0, "")
		return
	}

	maxValue := 1
	for _, point := range points {
		maxValue = max(maxValue, max(point.Sent, point.Accepted))
	}
	plotLeft := x + 12
	plotWidth := width - 14
	plotTop := y
	plotHeight := height - 12
	doc.SetDrawColor(reportBorder[0], reportBorder[1], reportBorder[2])
	doc.SetLineWidth(0.15)
	for step := 0; step <= 4; step++ {
		lineY := plotTop + float64(step)*plotHeight/4
		doc.Line(plotLeft, lineY, plotLeft+plotWidth, lineY)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(plotLeft-10, lineY-1.5)
		doc.CellFormat(8, 3, strconv.Itoa(int(math.Round(float64(maxValue)*(1-float64(step)/4)))), "", 0, "R", false, 0, "")
	}

	pointX := func(index int) float64 {
		if len(points) == 1 {
			return plotLeft + plotWidth/2
		}
		return plotLeft + float64(index)*plotWidth/float64(len(points)-1)
	}
	pointY := func(value int) float64 {
		return plotTop + plotHeight - float64(value)*plotHeight/float64(maxValue)
	}

	for _, series := range []struct {
		color [3]int
		value func(domain.SellerReportTimelinePoint) int
	}{
		{reportBrand, func(point domain.SellerReportTimelinePoint) int { return point.Sent }},
		{reportAccepted, func(point domain.SellerReportTimelinePoint) int { return point.Accepted }},
	} {
		doc.SetDrawColor(series.color[0], series.color[1], series.color[2])
		doc.SetFillColor(series.color[0], series.color[1], series.color[2])
		doc.SetLineWidth(0.9)
		for index := 0; index < len(points); index++ {
			if index > 0 {
				doc.Line(pointX(index-1), pointY(series.value(points[index-1])), pointX(index), pointY(series.value(points[index])))
			}
			doc.Circle(pointX(index), pointY(series.value(points[index])), 0.9, "F")
		}
	}

	doc.SetFont("Go", "", 5.5)
	doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
	labelStep := max(1, int(math.Ceil(float64(len(points))/3)))
	for index, point := range points {
		if index%labelStep != 0 && index != len(points)-1 {
			continue
		}
		label := r.reportBucketLabel(point.Date, granularity)
		doc.SetXY(pointX(index)-7, plotTop+plotHeight+2)
		doc.CellFormat(14, 3.5, label, "", 0, "C", false, 0, "")
	}
	doc.SetFont("Go", "", 6)
	doc.SetXY(x+width-52, y-9)
	doc.SetFillColor(reportBrand[0], reportBrand[1], reportBrand[2])
	doc.Circle(x+width-50, y-7.5, 1, "F")
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(x+width-47, y-9)
	doc.CellFormat(20, 4, "Enviadas", "", 0, "L", false, 0, "")
	doc.SetFillColor(reportAccepted[0], reportAccepted[1], reportAccepted[2])
	doc.Circle(x+width-26, y-7.5, 1, "F")
	doc.SetXY(x+width-23, y-9)
	doc.CellFormat(20, 4, "Aceptadas", "", 0, "L", false, 0, "")
}

func (r *SellerReportRenderer) drawRankings(doc *gofpdf.Fpdf, report domain.SellerReport, y float64) float64 {
	width := (pageWidth - reportSectionGap) / 2
	height := max(float64(len(report.TopProducts)), float64(len(report.TopRevenueProducts))) * 9.2
	height = max(42, 18+height)
	r.drawCard(doc, leftMargin, y, width, height, "Productos más pedidos", "Por cantidad de veces solicitados.")
	r.drawProductRanking(doc, report.TopProducts, leftMargin+6, y+16, width-12, reportBrandSoft, reportBrand)
	r.drawCard(doc, leftMargin+width+reportSectionGap, y, width, height,
		"Productos con mayor ingreso", "Versión aceptada, antes de descuentos.")
	r.drawRevenueRanking(doc, report.TopRevenueProducts, leftMargin+width+reportSectionGap+6, y+16, width-12)
	doc.SetY(y + height)
	return y + height
}

func (r *SellerReportRenderer) drawProductRanking(
	doc *gofpdf.Fpdf, products []domain.SellerReportProduct, x, y, width float64,
	barColor, textColor [3]int,
) {
	if len(products) == 0 {
		r.drawEmpty(doc, x, y, width, "Sin datos en el período.")
		return
	}
	maxValue := 1
	for _, product := range products {
		maxValue = max(maxValue, product.Requests)
	}
	for index, product := range products {
		rowY := y + float64(index)*9.2
		doc.SetFillColor(barColor[0], barColor[1], barColor[2])
		doc.RoundedRect(x, rowY+5.7, width*float64(product.Requests)/float64(maxValue), 1.1, 0.4, "1234", "F")
		doc.SetFont("Go", "B", 6.5)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(x, rowY)
		doc.CellFormat(5, 5, strconv.Itoa(index+1), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(x+5, rowY)
		name := product.Name
		if name == "" {
			name = "Sin nombre"
		}
		doc.CellFormat(width-29, 5, r.fitText(doc, name, width-29), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "B", 7)
		doc.SetTextColor(textColor[0], textColor[1], textColor[2])
		doc.SetXY(x+width-24, rowY)
		doc.CellFormat(24, 5, reportCount(product.Requests)+" ped.", "", 0, "R", false, 0, "")
	}
}

func (r *SellerReportRenderer) drawRevenueRanking(
	doc *gofpdf.Fpdf, products []domain.SellerReportRevenueProduct, x, y, width float64,
) {
	if len(products) == 0 {
		r.drawEmpty(doc, x, y, width, "Sin datos en el período.")
		return
	}
	maxValue := decimal.Zero
	for _, product := range products {
		maxValue = decimal.Max(maxValue, product.Revenue)
	}
	if maxValue.IsZero() {
		maxValue = decimal.NewFromInt(1)
	}
	for index, product := range products {
		rowY := y + float64(index)*9.2
		ratio, _ := product.Revenue.Div(maxValue).Float64()
		doc.SetFillColor(reportAccepted[0], reportAccepted[1], reportAccepted[2])
		doc.RoundedRect(x, rowY+5.7, width*ratio, 1.1, 0.4, "1234", "F")
		doc.SetFont("Go", "B", 6.5)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(x, rowY)
		doc.CellFormat(5, 5, strconv.Itoa(index+1), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(x+5, rowY)
		name := product.Name
		if name == "" {
			name = "Sin nombre"
		}
		doc.CellFormat(width-36, 5, r.fitText(doc, name, width-36), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "B", 7)
		doc.SetTextColor(reportAccepted[0], reportAccepted[1], reportAccepted[2])
		doc.SetXY(x+width-31, rowY)
		doc.CellFormat(31, 5, reportMoney(product.Revenue), "", 0, "R", false, 0, "")
	}
}

func (r *SellerReportRenderer) drawClients(doc *gofpdf.Fpdf, report domain.SellerReport, y float64) float64 {
	height := r.clientsHeight(report)
	r.drawCard(doc, leftMargin, y, pageWidth, height, "Clientes por importe aceptado", "Ordenados por importe aceptado en el período.")
	if len(report.TopClients) == 0 {
		r.drawEmpty(doc, leftMargin+6, y+18, pageWidth-12, "Sin datos en el período.")
		return y + height
	}
	tableX := leftMargin + 6
	tableWidth := pageWidth - 12
	doc.SetFillColor(reportSurface[0], reportSurface[1], reportSurface[2])
	doc.Rect(tableX, y+16, tableWidth, 7, "F")
	doc.SetFont("Go", "B", 6.2)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(tableX+3, y+18)
	doc.CellFormat(tableWidth-66, 4, "CLIENTE", "", 0, "L", false, 0, "")
	doc.SetXY(tableX+tableWidth-63, y+18)
	doc.CellFormat(60, 4, "IMPORTE ACEPTADO", "", 0, "R", false, 0, "")
	for index, client := range report.TopClients {
		rowY := y + 24 + float64(index)*8
		if index%2 == 1 {
			doc.SetFillColor(252, 253, 255)
			doc.Rect(tableX, rowY, tableWidth, 8, "F")
		}
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		name := client.Name
		if name == "" {
			name = "Sin nombre"
		}
		doc.SetXY(tableX+3, rowY+2)
		doc.CellFormat(tableWidth-66, 4, r.fitText(doc, name, tableWidth-66), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "B", 7)
		doc.SetXY(tableX+tableWidth-63, rowY+2)
		doc.CellFormat(60, 4, reportMoney(client.AcceptedAmount), "", 0, "R", false, 0, "")
	}
	return y + height
}

func (r *SellerReportRenderer) clientsHeight(report domain.SellerReport) float64 {
	if len(report.TopClients) == 0 {
		return 42
	}
	return 24 + float64(len(report.TopClients))*8
}

func (r *SellerReportRenderer) drawAIMatch(doc *gofpdf.Fpdf, report domain.SellerReport, y float64) float64 {
	height := 54.0
	r.drawCard(doc, leftMargin, y, pageWidth, height, "Lectura automática del catálogo", "Resultado de la última generación de IA por pedido.")
	if report.AIMatch.Total == 0 {
		r.drawEmpty(doc, leftMargin+6, y+18, pageWidth-12, "Sin datos en el período.")
		return y + height
	}

	doc.SetFont("Go", "B", 18)
	doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
	doc.SetXY(leftMargin+8, y+17)
	doc.CellFormat(55, 8, reportPercent(report.AIMatch.Rate), "", 0, "L", false, 0, "")
	doc.SetFont("Go", "", 6.5)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(leftMargin+8, y+27)
	doc.CellFormat(55, 4, fmt.Sprintf("Tasa de coincidencia · %s de %s líneas", reportCount(report.AIMatch.Matched), reportCount(report.AIMatch.Total)), "", 0, "L", false, 0, "")

	barX := leftMargin + 68
	barY := y + 22
	barWidth := pageWidth - 82
	barHeight := 6.5
	total := max(1, report.AIMatch.Total)
	segments := []struct {
		count int
		label string
		color [3]int
	}{
		{report.AIMatch.Confidence.High, "Alta", reportAccepted},
		{report.AIMatch.Confidence.Medium, "Media", reportBrand},
		{report.AIMatch.Confidence.Low, "Baja", reportWarning},
	}
	offset := barX
	doc.SetFillColor(reportBorder[0], reportBorder[1], reportBorder[2])
	doc.RoundedRect(barX, barY, barWidth, barHeight, 1.5, "1234", "F")
	for _, segment := range segments {
		width := barWidth * float64(segment.count) / float64(total)
		if width <= 0 {
			continue
		}
		doc.SetFillColor(segment.color[0], segment.color[1], segment.color[2])
		doc.Rect(offset, barY, width, barHeight, "F")
		offset += width
	}
	legendX := barX
	for _, segment := range segments {
		doc.SetFillColor(segment.color[0], segment.color[1], segment.color[2])
		doc.Circle(legendX+1, y+35, 1.2, "F")
		doc.SetFont("Go", "", 6.5)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(legendX+4, y+33)
		doc.CellFormat(31, 4, fmt.Sprintf("%s: %s", segment.label, reportCount(segment.count)), "", 0, "L", false, 0, "")
		legendX += 37
	}
	return y + height
}

func (r *SellerReportRenderer) drawBranches(
	doc *gofpdf.Fpdf, report domain.SellerReport, generatedAt time.Time, y float64,
) float64 {
	if y+16 > reportContentBottom {
		r.newContentPage(doc, report, generatedAt)
		y = doc.GetY()
	}
	doc.SetFont("Go", "B", 9)
	doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
	doc.SetXY(leftMargin, y)
	doc.CellFormat(pageWidth, 5, "Pedidos por sucursal", "", 1, "L", false, 0, "")
	y += 7
	r.drawBranchHeader(doc, y)
	y += 7
	if len(report.Branches) == 0 {
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(leftMargin+3, y+2)
		doc.CellFormat(pageWidth-6, 5, "Sin datos en el período.", "", 0, "L", false, 0, "")
		return y
	}
	for index, branch := range report.Branches {
		if y+8 > reportContentBottom {
			r.newContentPage(doc, report, generatedAt)
			r.drawBranchHeader(doc, doc.GetY())
			y = doc.GetY() + 7
		}
		if index%2 == 1 {
			doc.SetFillColor(252, 253, 255)
			doc.Rect(leftMargin, y, pageWidth, 8, "F")
		}
		doc.SetFont("Go", "", 7)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(leftMargin+3, y+2)
		doc.CellFormat(pageWidth-62, 4, r.fitText(doc, branch.BranchName, pageWidth-62), "", 0, "L", false, 0, "")
		doc.SetFont("Go", "B", 7)
		doc.SetXY(leftMargin+pageWidth-51, y+2)
		doc.CellFormat(48, 4, reportCount(branch.RFQs), "", 0, "R", false, 0, "")
		y += 8
	}
	doc.SetY(y)
	return y
}

func (r *SellerReportRenderer) drawBranchHeader(doc *gofpdf.Fpdf, y float64) {
	doc.SetFillColor(reportSurface[0], reportSurface[1], reportSurface[2])
	doc.Rect(leftMargin, y, pageWidth, 7, "F")
	doc.SetFont("Go", "B", 6.2)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(leftMargin+3, y+2)
	doc.CellFormat(pageWidth-62, 4, "SUCURSAL", "", 0, "L", false, 0, "")
	doc.SetXY(leftMargin+pageWidth-51, y+2)
	doc.CellFormat(48, 4, "PEDIDOS", "", 0, "R", false, 0, "")
}

func (r *SellerReportRenderer) drawMethodology(
	doc *gofpdf.Fpdf, report domain.SellerReport, generatedAt time.Time, y float64,
) {
	height := 66.0
	if y+height > reportContentBottom {
		r.newContentPage(doc, report, generatedAt)
		y = doc.GetY()
	}
	r.drawCard(doc, leftMargin, y, pageWidth, height, "Notas metodológicas", "")
	notes := []struct {
		label string
		body  string
	}{
		{"Tasa de aceptación", "de las cotizaciones enviadas por primera vez en el período, las que el cliente aceptó hasta hoy."},
		{"Tiempo de respuesta", "primera cotización enviada después de recibir el pedido. Los pedidos manuales no entran en el cálculo."},
		{"Importe aceptado", "suma de los totales comerciales de las versiones aceptadas, con descuentos aplicados. No representa utilidad."},
		{"Ingreso por producto", "ingreso bruto de las líneas aceptadas, antes de descuentos; los ítems sin coincidencia no suman."},
		{"Lectura automática", "resultado de la última generación de IA por pedido; la confianza agrupa las líneas puntuadas."},
	}
	currentY := y + 16
	for _, note := range notes {
		doc.SetFont("Go", "B", 6.4)
		doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
		doc.SetXY(leftMargin+8, currentY)
		doc.CellFormat(37, 3.4, note.label+":", "", 0, "L", false, 0, "")
		doc.SetFont("Go", "", 6.4)
		doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
		doc.SetXY(leftMargin+45, currentY)
		doc.MultiCell(pageWidth-53, 3.4, note.body, "", "L", false)
		currentY = max(currentY+6.6, doc.GetY()+1.2)
	}
	doc.SetFont("Go", "", 6.2)
	doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
	doc.SetXY(leftMargin+8, y+57)
	doc.CellFormat(pageWidth-16, 3.2, "Moneda: pesos argentinos (ARS). Alcance: "+r.scopeName(report)+".", "", 0, "L", false, 0, "")
}

func (r *SellerReportRenderer) drawCard(
	doc *gofpdf.Fpdf, x, y, width, height float64, title, subtitle string,
) {
	doc.SetFillColor(255, 255, 255)
	doc.SetDrawColor(reportBorder[0], reportBorder[1], reportBorder[2])
	doc.SetLineWidth(0.25)
	doc.RoundedRect(x, y, width, height, reportCardRadius, "1234", "FD")
	doc.SetFont("Go", "B", 8)
	doc.SetTextColor(reportInk[0], reportInk[1], reportInk[2])
	doc.SetXY(x+6, y+5)
	doc.CellFormat(width-12, 4.5, r.fitText(doc, title, width-12), "", 0, "L", false, 0, "")
	if subtitle != "" {
		doc.SetFont("Go", "", 6)
		doc.SetTextColor(reportSubtle[0], reportSubtle[1], reportSubtle[2])
		doc.SetXY(x+6, y+10.5)
		doc.CellFormat(width-12, 3.5, r.fitText(doc, subtitle, width-12), "", 0, "L", false, 0, "")
	}
}

func (r *SellerReportRenderer) drawEmpty(doc *gofpdf.Fpdf, x, y, width float64, message string) {
	doc.SetFont("Go", "", 7)
	doc.SetTextColor(reportMuted[0], reportMuted[1], reportMuted[2])
	doc.SetXY(x, y+16)
	doc.CellFormat(width, 6, message, "", 0, "C", false, 0, "")
}

func (r *SellerReportRenderer) nextSectionY(doc *gofpdf.Fpdf, sectionHeight float64) float64 {
	y := doc.GetY() + reportSectionGap
	if y+sectionHeight > reportContentBottom {
		return pageBottom + 1
	}
	return y
}

func (r *SellerReportRenderer) piePoints(x, y, radius, start, sweep float64) []gofpdf.PointType {
	points := []gofpdf.PointType{{X: x, Y: y}}
	steps := max(1, int(math.Ceil(math.Abs(sweep)/4)))
	for step := 0; step <= steps; step++ {
		angle := (start + sweep*float64(step)/float64(steps)) * math.Pi / 180
		points = append(points, gofpdf.PointType{
			X: x + radius*math.Cos(angle),
			Y: y + radius*math.Sin(angle),
		})
	}
	return points
}

func (r *SellerReportRenderer) reportPeriodLabel(report domain.SellerReport) string {
	return r.reportShortDate(report.Range.From) + " al " + r.reportShortDate(report.Range.To)
}

func (r *SellerReportRenderer) reportScopeLabel(report domain.SellerReport) string {
	return "Alcance: " + r.scopeName(report) + "."
}

func (r *SellerReportRenderer) scopeName(report domain.SellerReport) string {
	switch {
	case len(report.Filters.Branches) > 0:
		return "sucursales elegidas en los filtros"
	case report.Scope.MultiBranch:
		return "todas las sucursales disponibles para el usuario"
	}
	return "sucursal activa"
}

func (r *SellerReportRenderer) reportFiltersLabel(report domain.SellerReport) string {
	parts := make([]string, 0, 6)
	if len(report.Filters.Statuses) > 0 {
		labels := make([]string, 0, len(report.Filters.Statuses))
		for _, status := range report.Filters.Statuses {
			labels = append(labels, r.statusLabel(status))
		}
		parts = append(parts, "Estados: "+strings.Join(labels, ", "))
	}
	if report.Filters.Channel != nil {
		parts = append(parts, "Canal: "+r.channelLabel(*report.Filters.Channel))
	}
	if report.Filters.SellerID != nil {
		parts = append(parts, "Vendedor: "+reportFilterName(report.Filters.SellerName))
	}
	if report.Filters.ClientID != nil {
		parts = append(parts, "Cliente: "+reportFilterName(report.Filters.ClientName))
	}
	if len(report.Filters.Branches) > 0 {
		parts = append(parts, "Sucursales: "+reportOptionNames(report.Filters.Branches))
	}
	if len(report.Filters.Tags) > 0 {
		parts = append(parts, "Etiquetas: "+reportOptionNames(report.Filters.Tags))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Filtros: " + strings.Join(parts, " · ")
}

func (r *SellerReportRenderer) reportTimestamp(value time.Time) string {
	return value.In(domain.Argentina).Format("02/01/2006 15:04")
}

func (r *SellerReportRenderer) reportShortDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02/01/2006")
}

func (r *SellerReportRenderer) reportBucketLabel(value string, granularity domain.ReportGranularity) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	if granularity == domain.ReportGranularityMonth {
		return reportMonthNames[parsed.Month()-1] + " " + parsed.Format("06")
	}
	return parsed.Format("02/01")
}

var reportMonthNames = [12]string{
	"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic",
}

func (r *SellerReportRenderer) granularityLabel(granularity domain.ReportGranularity) string {
	switch granularity {
	case domain.ReportGranularityWeek:
		return "Por semana, desde el lunes."
	case domain.ReportGranularityMonth:
		return "Por mes."
	}
	return "Por día."
}

func (r *SellerReportRenderer) responseDetail(response domain.SellerReportResponseTime) string {
	if response.Sample == 0 || response.MedianSeconds == nil {
		return "Sin datos suficientes."
	}
	return fmt.Sprintf("Mediana: %s · %s pedidos", reportDuration(response.MedianSeconds), reportCount(response.Sample))
}

func (r *SellerReportRenderer) statusSummary(report domain.SellerReport) string {
	total := 0
	for _, item := range report.QuotesByStatus {
		total += item.Count
	}
	if total == 0 {
		return "Sin cotizaciones registradas."
	}
	return reportCount(total) + " cotizaciones registradas."
}

func (r *SellerReportRenderer) statusColor(status domain.QuoteStatus) (int, int, int) {
	switch status {
	case domain.QuoteStatusQuoted:
		return reportQuoted[0], reportQuoted[1], reportQuoted[2]
	case domain.QuoteStatusSent:
		return reportBrand[0], reportBrand[1], reportBrand[2]
	case domain.QuoteStatusChangeRequested:
		return reportWarning[0], reportWarning[1], reportWarning[2]
	case domain.QuoteStatusAccepted:
		return reportAccepted[0], reportAccepted[1], reportAccepted[2]
	case domain.QuoteStatusRejected:
		return reportDanger[0], reportDanger[1], reportDanger[2]
	default:
		return reportMuted[0], reportMuted[1], reportMuted[2]
	}
}

func (r *SellerReportRenderer) channelLabel(channel domain.ChannelType) string {
	switch channel {
	case domain.ChannelTypeWhatsApp:
		return "WhatsApp"
	case domain.ChannelTypeEmail:
		return "Email"
	case domain.ChannelTypeWebApp:
		return "Web"
	case domain.ChannelTypeManualEntry:
		return "Carga manual"
	default:
		return string(channel)
	}
}

func reportFilterName(name string) string {
	if name == "" {
		return "Sin nombre"
	}
	return name
}

func reportOptionNames(options []domain.SellerReportFilterOption) string {
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, reportFilterName(option.Name))
	}
	return strings.Join(names, ", ")
}

func (r *SellerReportRenderer) statusLabel(status domain.QuoteStatus) string {
	switch status {
	case domain.QuoteStatusDraft:
		return "Borrador"
	case domain.QuoteStatusQuoted:
		return "Cotizada"
	case domain.QuoteStatusSent:
		return "Enviada"
	case domain.QuoteStatusChangeRequested:
		return "Cambio solicitado"
	case domain.QuoteStatusAccepted:
		return "Aceptada"
	case domain.QuoteStatusRejected:
		return "Rechazada"
	default:
		return string(status)
	}
}

func (r *SellerReportRenderer) fitText(doc *gofpdf.Fpdf, value string, width float64) string {
	if doc.GetStringWidth(value) <= width {
		return value
	}
	suffix := "…"
	for doc.GetStringWidth(value+suffix) > width && len(value) > 0 {
		runes := []rune(value)
		value = string(runes[:len(runes)-1])
	}
	return value + suffix
}

func reportCount(value int) string {
	raw := strconv.Itoa(value)
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	var builder strings.Builder
	for index, digit := range raw {
		if index > 0 && (len(raw)-index)%3 == 0 {
			builder.WriteByte('.')
		}
		builder.WriteRune(digit)
	}
	if negative {
		return "-" + builder.String()
	}
	return builder.String()
}

func reportMoney(value decimal.Decimal) string {
	return commercialMoney("$", value.StringFixed(2))
}

func reportPercent(value *decimal.Decimal) string {
	if value == nil {
		return "Sin datos"
	}
	number := value.Mul(decimal.NewFromInt(100)).InexactFloat64()
	formatted := strings.ReplaceAll(strconv.FormatFloat(number, 'f', 1, 64), ".", ",")
	formatted = strings.TrimSuffix(formatted, ",0")
	return formatted + "%"
}

func reportShare(value, total int) string {
	if total == 0 {
		return "0%"
	}
	return strconv.Itoa(int(math.Round(float64(value)*100/float64(total)))) + "%"
}

func reportDuration(seconds *int64) string {
	if seconds == nil {
		return "Sin datos"
	}
	value := *seconds
	if value < 60 {
		return fmt.Sprintf("%d s", value)
	}
	minutes := int(math.Round(float64(value) / 60))
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	hours := minutes / 60
	rest := minutes % 60
	if rest == 0 {
		return fmt.Sprintf("%d h", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, rest)
}
