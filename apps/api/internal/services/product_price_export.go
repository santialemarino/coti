package services

import (
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

var priceExportHeaders = []string{
	"codigo", "producto", "precio", "precio_minimo",
}

var priceExportInstructions = []string{
	"Editá los valores de la hoja Precios y volvé a importar este archivo.",
	"Las columnas codigo y precio son obligatorias.",
	"El precio y el precio mínimo deben ser mayores a cero y admitir hasta 2 decimales.",
	"El precio mínimo no puede superar el precio de venta.",
	"La columna producto es informativa. La importación identifica cada producto por codigo.",
	"Eliminá del archivo las filas que no quieras actualizar.",
	"La actualización crea una nueva vigencia y no modifica cotizaciones anteriores.",
}

func buildProductPriceXLSX(export domain.ProductPriceExport) ([]byte, error) {
	priceRows := []spreadsheet.ExportRow{{Number: 1, Values: priceExportHeaders, Header: true}}
	for index, row := range export.Rows {
		priceRows = append(priceRows, spreadsheet.ExportRow{Number: index + 2, Values: []string{
			row.Code, row.ProductName, row.Price, optionalString(row.MinPrice),
		}})
	}
	instructionRows := []spreadsheet.ExportRow{{
		Number: 1, Values: []string{"Exportación de precios — " + export.BranchName}, Header: true,
	}}
	for index, instruction := range priceExportInstructions {
		instructionRows = append(instructionRows, spreadsheet.ExportRow{Number: index + 3, Values: []string{instruction}})
	}
	return spreadsheet.Write(spreadsheet.Workbook{Sheets: []spreadsheet.Sheet{
		{
			Name: "Precios", Rows: priceRows, FreezeHeader: true, AutoFilter: true,
			ColumnWidths: []spreadsheet.ColumnWidth{
				{Min: 1, Max: 1, Width: 20}, {Min: 2, Max: 2, Width: 38}, {Min: 3, Max: 4, Width: 18},
			},
		},
		{
			Name: "Instrucciones", Rows: instructionRows,
			ColumnWidths: []spreadsheet.ColumnWidth{{Min: 1, Max: 1, Width: 110}},
		},
	}})
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func slugFilename(value string) string {
	normalized := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n",
	).Replace(strings.ToLower(value))
	var slug strings.Builder
	lastWasDash := false
	for _, char := range normalized {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			slug.WriteRune(char)
			lastWasDash = false
		} else if !lastWasDash && slug.Len() > 0 {
			slug.WriteByte('-')
			lastWasDash = true
		}
	}
	result := strings.Trim(slug.String(), "-")
	if result == "" {
		return "sucursal"
	}
	return result
}
