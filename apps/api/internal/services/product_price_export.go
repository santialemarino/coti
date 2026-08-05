package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
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
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", xlsxContentTypes},
		{"_rels/.rels", xlsxRootRelationships},
		{"xl/workbook.xml", buildXLSXWorkbook("Precios")},
		{"xl/_rels/workbook.xml.rels", xlsxWorkbookRelationships},
		{"xl/styles.xml", xlsxStyles},
		{"xl/worksheets/sheet1.xml", buildPriceExportSheet(export)},
		{"xl/worksheets/sheet2.xml", buildInstructionsSheet(export.BranchName)},
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

func buildPriceExportSheet(export domain.ProductPriceExport) string {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sheet.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	sheet.WriteString(`<cols><col min="1" max="1" width="20" customWidth="1"/><col min="2" max="2" width="38" customWidth="1"/><col min="3" max="4" width="18" customWidth="1"/></cols><sheetData>`)
	writeXLSXRow(&sheet, 1, priceExportHeaders, true)
	for index, row := range export.Rows {
		writeXLSXRow(&sheet, index+2, []string{
			row.Code,
			row.ProductName,
			row.Price,
			optionalString(row.MinPrice),
		}, false)
	}
	sheet.WriteString(`</sheetData>`)
	sheet.WriteString(fmt.Sprintf(`<autoFilter ref="A1:D%d"/>`, len(export.Rows)+1))
	sheet.WriteString(`</worksheet>`)
	return sheet.String()
}

func buildInstructionsSheet(branchName string) string {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><cols><col min="1" max="1" width="110" customWidth="1"/></cols><sheetData>`)
	writeXLSXRow(&sheet, 1, []string{"Exportación de precios — " + branchName}, true)
	for index, instruction := range priceExportInstructions {
		writeXLSXRow(&sheet, index+3, []string{instruction}, false)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	return sheet.String()
}

func writeXLSXRow(sheet *strings.Builder, number int, values []string, header bool) {
	sheet.WriteString(fmt.Sprintf(`<row r="%d">`, number))
	for index, value := range values {
		reference := xlsxColumnName(index+1) + fmt.Sprint(number)
		style := ""
		if header {
			style = ` s="1"`
		}
		sheet.WriteString(`<c r="` + reference + `" t="inlineStr"` + style + `><is><t xml:space="preserve">`)
		_ = xml.EscapeText(sheet, []byte(value))
		sheet.WriteString(`</t></is></c>`)
	}
	sheet.WriteString(`</row>`)
}

func xlsxColumnName(column int) string {
	var name string
	for column > 0 {
		column--
		name = string(rune('A'+column%26)) + name
		column /= 26
	}
	return name
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

const xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const xlsxRootRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

func buildXLSXWorkbook(firstSheetName string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets><sheet name="` + firstSheetName + `" sheetId="1" r:id="rId1"/><sheet name="Instrucciones" sheetId="2" r:id="rId2"/></sheets>
</workbook>`
}

const xlsxWorkbookRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
