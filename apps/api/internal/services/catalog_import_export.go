package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

var catalogImportHeaders = []string{
	"codigo", "nombre", "descripcion", "unidad", "categoria", "precio",
	"precio_minimo", "moneda", "condiciones",
}

var catalogImportInstructions = []string{
	"Completá la hoja Catálogo y volvé a importar este archivo desde el backoffice.",
	"Las columnas codigo, descripcion, unidad y precio son obligatorias.",
	"La columna nombre es opcional; si queda vacía, se usa la descripción como nombre del producto.",
	"Cada código debe ser único dentro del archivo y no puede existir previamente en el catálogo de la cuenta.",
	"El precio y el precio mínimo deben ser mayores a cero y admitir hasta 2 decimales.",
	"El precio mínimo no puede superar el precio de venta.",
	"La moneda usa un código de 3 letras; si queda vacía, se toma ARS.",
	"Las filas con errores se muestran en la vista previa y se omiten al confirmar.",
	"Los productos confirmados quedan activos y disponibles en la sucursal seleccionada.",
}

func buildCatalogImportXLSX() ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", xlsxContentTypes},
		{"_rels/.rels", xlsxRootRelationships},
		{"xl/workbook.xml", buildXLSXWorkbook("Catálogo")},
		{"xl/_rels/workbook.xml.rels", xlsxWorkbookRelationships},
		{"xl/styles.xml", xlsxStyles},
		{"xl/worksheets/sheet1.xml", buildCatalogImportSheet()},
		{"xl/worksheets/sheet2.xml", buildCatalogImportInstructionsSheet()},
	}
	for _, file := range files {
		writer, err := archive.Create(file.name)
		if err != nil {
			return nil, fmt.Errorf("create xlsx file %s: %w", file.name, err)
		}
		if _, err := writer.Write([]byte(file.content)); err != nil {
			return nil, fmt.Errorf("write xlsx file %s: %w", file.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close xlsx: %w", err)
	}
	return buffer.Bytes(), nil
}

func buildCatalogImportSheet() string {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sheet.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	sheet.WriteString(`<cols><col min="1" max="1" width="20" customWidth="1"/><col min="2" max="3" width="38" customWidth="1"/><col min="4" max="5" width="18" customWidth="1"/><col min="6" max="8" width="18" customWidth="1"/><col min="9" max="9" width="42" customWidth="1"/></cols><sheetData>`)
	writeXLSXRow(&sheet, 1, catalogImportHeaders, true)
	sheet.WriteString(`</sheetData><autoFilter ref="A1:I1"/></worksheet>`)
	return sheet.String()
}

func buildCatalogImportInstructionsSheet() string {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><cols><col min="1" max="1" width="110" customWidth="1"/></cols><sheetData>`)
	writeXLSXRow(&sheet, 1, []string{"Carga inicial del catálogo"}, true)
	for index, instruction := range catalogImportInstructions {
		writeXLSXRow(&sheet, index+3, []string{instruction}, false)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	return sheet.String()
}
