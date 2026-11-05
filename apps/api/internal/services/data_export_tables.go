package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/dataexport"
)

// The labels match the backoffice's wording, so a file reads like the screens it came from.
var (
	exportQuoteStatusLabels = map[domain.QuoteStatus]string{
		domain.QuoteStatusDraft: "Borrador", domain.QuoteStatusQuoted: "Cotizada",
		domain.QuoteStatusSent: "Enviada", domain.QuoteStatusChangeRequested: "Cambio solicitado",
		domain.QuoteStatusAccepted: "Aceptada", domain.QuoteStatusRejected: "Rechazada",
	}
	exportMatchStatusLabels = map[domain.ItemMatchStatus]string{
		domain.ItemMatchStatusMatched: "Emparejado", domain.ItemMatchStatusAmbiguous: "Ambiguo",
		domain.ItemMatchStatusNoMatch: "Sin emparejar",
	}
	exportVATRateLabels = map[domain.VATRate]string{
		domain.VATRateTwentyOne: "IVA 21 %", domain.VATRateTenFive: "IVA 10,5 %",
		domain.VATRateTwentySeven: "IVA 27 %", domain.VATRateFive: "IVA 5 %",
		domain.VATRateTwoFive: "IVA 2,5 %", domain.VATRateZero: "IVA 0 %",
		domain.VATRateExempt: "Exento",
	}
	exportIVAConditionLabels = map[domain.IVACondition]string{
		domain.IVAConditionRegistered: "Responsable inscripto", domain.IVAConditionMonotributo: "Monotributo",
		domain.IVAConditionExempt: "Exento", domain.IVAConditionFinalConsumer: "Consumidor final",
	}
	exportClientOriginLabels = map[domain.ClientOrigin]string{
		domain.ClientOriginWhatsApp: "WhatsApp", domain.ClientOriginEmail: "Correo",
		domain.ClientOriginWebApp: "Web", domain.ClientOriginPhone: "Teléfono",
		domain.ClientOriginWalkIn: "Mostrador",
	}
	exportContactKindLabels = map[domain.ChannelType]string{
		domain.ChannelTypeWhatsApp: "WhatsApp", domain.ChannelTypeEmail: "Correo",
	}
	exportVoucherKindLabels = map[domain.VoucherKind]string{
		domain.VoucherKindInvoice: "Factura", domain.VoucherKindCreditNote: "Nota de crédito",
	}
	exportInvoiceStatusLabels = map[domain.InvoiceStatus]string{
		domain.InvoiceStatusPending: "Pendiente", domain.InvoiceStatusIssued: "Autorizada",
		domain.InvoiceStatusRejected: "Rechazada",
	}
	exportEnvironmentLabels = map[domain.ARCAEnvironment]string{
		domain.ARCAEnvironmentProduction: "Producción", domain.ARCAEnvironmentHomologation: "Prueba",
	}
	exportDocTypeLabels = map[domain.ReceiverDocType]string{
		domain.ReceiverDocCUIT: "CUIT", domain.ReceiverDocDNI: "DNI", domain.ReceiverDocNone: "Sin documento",
	}
)

func quoteExportTable(quotes []domain.DataExportQuote) (dataexport.Table, error) {
	table := dataexport.Table{
		Root: "cotizaciones", Record: "cotizacion",
		Columns: []string{
			"id", "numero", "sucursal", "estado", "archivada", "requiere_seguimiento", "id_cliente",
			"cliente", "vendedor", "fecha_creacion", "vencimiento", "cobrada_el", "version", "moneda",
			"subtotal", "descuentos", "total",
		},
		Children: &dataexport.ChildTable{
			Group: "items", Record: "item", Prefix: "item_",
			Columns: []string{
				"descripcion_solicitada", "codigo", "producto", "cantidad", "unidad",
				"precio_unitario", "subtotal", "estado_match",
			},
		},
	}
	for _, quote := range quotes {
		discounts, err := quoteDiscounts(quote)
		if err != nil {
			return dataexport.Table{}, err
		}
		record := dataexport.Record{Values: []string{
			quote.ID.String(), strconv.FormatInt(quote.Number, 10), quote.Branch,
			exportQuoteStatusLabels[quote.Status], exportYesNo(quote.ArchivedAt != nil),
			exportYesNo(quote.NeedsFollowUp), exportUUID(quote.ClientID), optionalString(quote.Client),
			optionalString(quote.Seller), exportTime(&quote.CreatedAt), exportTime(quote.ExpiresAt),
			exportTime(quote.PaidAt), exportInt(quote.VersionNumber), optionalString(quote.Currency),
			optionalString(quote.ItemsSubtotal), discounts, optionalString(quote.Total),
		}}
		for _, item := range quote.Items {
			record.Children = append(record.Children, []string{
				item.RequestedDescription, optionalString(item.ProductCode),
				optionalString(item.ProductName), item.Quantity, optionalString(item.Unit),
				optionalString(item.UnitPrice), optionalString(item.Subtotal),
				exportMatchStatusLabels[item.MatchStatus],
			})
		}
		table.Records = append(table.Records, record)
	}
	return table, nil
}

// quoteDiscounts is what the version's discounts take off its lines, which is how its total is
// defined; a version without priced lines has no discount to show.
func quoteDiscounts(quote domain.DataExportQuote) (string, error) {
	if quote.ItemsSubtotal == nil || quote.Total == nil {
		return "", nil
	}
	subtotal, err := decimal.NewFromString(*quote.ItemsSubtotal)
	if err != nil {
		return "", fmt.Errorf("quote %d subtotal: %w", quote.Number, err)
	}
	total, err := decimal.NewFromString(*quote.Total)
	if err != nil {
		return "", fmt.Errorf("quote %d total: %w", quote.Number, err)
	}
	return subtotal.Sub(total).StringFixed(2), nil
}

func catalogExportTable(products []domain.DataExportProduct) dataexport.Table {
	table := dataexport.Table{
		Root: "catalogo", Record: "producto",
		Columns: []string{
			"id", "codigo", "nombre", "descripcion", "unidad", "familia", "subgrupo", "iva", "activo",
		},
		Children: &dataexport.ChildTable{
			Group: "sucursales", Record: "sucursal", Prefix: "sucursal_",
			Columns: []string{"nombre", "disponible", "stock", "precio", "precio_minimo", "moneda"},
		},
	}
	for _, product := range products {
		record := dataexport.Record{Values: []string{
			product.ID.String(), optionalString(product.Code), product.Name,
			optionalString(product.Description), optionalString(product.Unit),
			optionalString(product.Family), optionalString(product.Subgroup),
			exportVATRateLabels[product.VATRate], exportYesNo(product.IsActive),
		}}
		for _, branch := range product.Branches {
			record.Children = append(record.Children, []string{
				branch.Branch, exportYesNo(branch.IsActive), optionalString(branch.Stock),
				optionalString(branch.Price), optionalString(branch.MinPrice),
				optionalString(branch.Currency),
			})
		}
		table.Records = append(table.Records, record)
	}
	return table
}

func clientExportTable(clients []domain.DataExportClient) dataexport.Table {
	table := dataexport.Table{
		Root: "clientes", Record: "cliente",
		Columns: []string{
			"id", "nombre", "registrado", "razon_social", "cuit", "dni", "condicion_iva",
			"domicilio_fiscal", "origen", "etiquetas", "notas", "fecha_alta",
		},
		Children: &dataexport.ChildTable{
			Group: "contactos", Record: "contacto", Prefix: "contacto_",
			Columns: []string{"tipo", "valor", "principal", "verificado"},
		},
	}
	for _, client := range clients {
		var ivaCondition, origin string
		if client.IVACondition != nil {
			ivaCondition = exportIVAConditionLabels[*client.IVACondition]
		}
		if client.Origin != nil {
			origin = exportClientOriginLabels[*client.Origin]
		}
		record := dataexport.Record{Values: []string{
			client.ID.String(), optionalString(client.Name), exportYesNo(client.TaxID != nil),
			optionalString(client.LegalName), optionalString(client.TaxID), optionalString(client.DNI),
			ivaCondition, optionalString(client.FiscalAddress), origin,
			strings.Join(client.Tags, "; "), optionalString(client.Notes), exportTime(&client.CreatedAt),
		}}
		for _, contact := range client.Contacts {
			record.Children = append(record.Children, []string{
				exportContactKindLabels[contact.Kind], contact.Value, exportYesNo(contact.IsMain),
				exportYesNo(contact.Verified),
			})
		}
		table.Records = append(table.Records, record)
	}
	return table
}

func invoiceExportTable(invoices []domain.DataExportInvoice) dataexport.Table {
	table := dataexport.Table{
		Root: "comprobantes", Record: "comprobante",
		Columns: []string{
			"id", "comprobante", "tipo", "punto_venta", "numero", "numero_completo", "fecha_emision",
			"estado", "cae", "vencimiento_cae", "entorno", "sucursal", "cotizacion_numero",
			"id_comprobante_anulado", "receptor_nombre", "receptor_tipo_documento",
			"receptor_documento", "receptor_condicion_iva", "neto", "exento", "iva", "total", "moneda",
		},
	}
	for _, invoice := range invoices {
		var number, fullNumber string
		if invoice.Number != nil {
			number = strconv.FormatInt(*invoice.Number, 10)
			fullNumber = fmt.Sprintf("%05d-%08d", invoice.PointOfSale, *invoice.Number)
		}
		table.Records = append(table.Records, dataexport.Record{Values: []string{
			invoice.ID.String(), exportVoucherKindLabels[invoice.Kind], string(invoice.Type),
			strconv.Itoa(invoice.PointOfSale), number, fullNumber, invoice.IssuedOn,
			exportInvoiceStatusLabels[invoice.Status], optionalString(invoice.CAE),
			optionalString(invoice.CAEExpiresOn), exportEnvironmentLabels[invoice.Environment],
			invoice.Branch, strconv.FormatInt(invoice.QuoteNumber, 10),
			exportUUID(invoice.CreditedInvoiceID), invoice.ReceiverName,
			exportDocTypeLabels[invoice.ReceiverDocType], optionalString(invoice.ReceiverDocNumber),
			exportIVAConditionLabels[invoice.ReceiverIVACondition], invoice.NetAmount,
			invoice.ExemptAmount, invoice.VATAmount, invoice.Total, invoice.Currency,
		}})
	}
	return table
}

// exportYesNo matches the SI/NO the catalog spreadsheet already uses.
func exportYesNo(value bool) string {
	if value {
		return "SI"
	}
	return "NO"
}

func exportTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.In(domain.Argentina).Format(time.RFC3339)
}

func exportUUID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func exportInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}
