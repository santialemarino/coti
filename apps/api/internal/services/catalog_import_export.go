package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var catalogImportHeaders = []string{
	"codigo", "nombre", "descripcion", "unidad", "familia", "subgrupo", "precio",
	"precio_minimo",
}

var catalogImportInstructions = []string{
	"Completá la hoja Catálogo y volvé a importar este archivo desde el backoffice.",
	"Las columnas codigo, nombre, unidad, familia y precio son obligatorias.",
	"Elegí la familia y, si corresponde, el subgrupo de sus desplegables. El subgrupo debe pertenecer a la familia.",
	"Cada código debe ser único dentro del archivo y no puede existir previamente en el catálogo de la cuenta.",
	"El precio y el precio mínimo deben ser mayores a cero y admitir hasta 2 decimales.",
	"El precio mínimo no puede superar el precio de venta.",
	"Los precios iniciales se guardan en ARS.",
	"Las filas con errores se muestran en la vista previa y se omiten al confirmar.",
	"Los productos confirmados quedan activos y disponibles en la sucursal seleccionada.",
}

func buildCatalogImportXLSX(families []domain.ProductFamily) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", catalogXLSXContentTypes},
		{"_rels/.rels", xlsxRootRelationships},
		{"xl/workbook.xml", buildCatalogWorkbook(families)},
		{"xl/_rels/workbook.xml.rels", catalogXLSXWorkbookRelationships},
		{"xl/styles.xml", xlsxStyles},
		{"xl/worksheets/sheet1.xml", buildCatalogImportSheet()},
		{"xl/worksheets/sheet2.xml", buildCatalogImportInstructionsSheet()},
		{"xl/worksheets/sheet3.xml", buildCatalogListsSheet(families)},
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
	sheet.WriteString(`<cols><col min="1" max="1" width="20" customWidth="1"/><col min="2" max="3" width="38" customWidth="1"/><col min="4" max="4" width="18" customWidth="1"/><col min="5" max="6" width="34" customWidth="1"/><col min="7" max="8" width="18" customWidth="1"/></cols><sheetData>`)
	writeXLSXRow(&sheet, 1, catalogImportHeaders, true)
	sheet.WriteString(`</sheetData><autoFilter ref="A1:H1"/><dataValidations count="2">`)
	sheet.WriteString(`<dataValidation type="list" allowBlank="0" showErrorMessage="1" errorTitle="Familia inválida" error="Elegí una familia del listado." sqref="E2:E10000"><formula1>Familias</formula1></dataValidation>`)
	sheet.WriteString(`<dataValidation type="list" allowBlank="1" showErrorMessage="1" errorTitle="Subgrupo inválido" error="Elegí un subgrupo del listado que pertenezca a la familia seleccionada." sqref="F2:F10000"><formula1>Subgrupos</formula1></dataValidation>`)
	sheet.WriteString(`</dataValidations></worksheet>`)
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

func buildCatalogListsSheet(families []domain.ProductFamily) string {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	subgroups := catalogSubgroupNames(families)
	maxRows := len(families)
	if len(subgroups) > maxRows {
		maxRows = len(subgroups)
	}
	for row := 1; row <= maxRows+1; row++ {
		sheet.WriteString(fmt.Sprintf(`<row r="%d">`, row))
		if row == 1 {
			writeCatalogListCell(&sheet, 1, row, "Familia", true)
		} else if row-2 < len(families) {
			writeCatalogListCell(&sheet, 1, row, families[row-2].Name, false)
		}
		if row == 1 {
			writeCatalogListCell(&sheet, 2, row, "Subgrupo", true)
		} else if row-2 < len(subgroups) {
			writeCatalogListCell(&sheet, 2, row, subgroups[row-2], false)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	return sheet.String()
}

func writeCatalogListCell(sheet *strings.Builder, column, row int, value string, header bool) {
	style := ""
	if header {
		style = ` s="1"`
	}
	sheet.WriteString(fmt.Sprintf(`<c r="%s%d" t="inlineStr"%s><is><t xml:space="preserve">`, xlsxColumnName(column), row, style))
	_ = xml.EscapeText(sheet, []byte(value))
	sheet.WriteString(`</t></is></c>`)
}

func buildCatalogWorkbook(families []domain.ProductFamily) string {
	var workbook strings.Builder
	workbook.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Catálogo" sheetId="1" r:id="rId1"/><sheet name="Instrucciones" sheetId="2" r:id="rId2"/><sheet name="Listas" sheetId="3" state="hidden" r:id="rId3"/></sheets><definedNames>`)
	workbook.WriteString(fmt.Sprintf(`<definedName name="Familias">Listas!$A$2:$A$%d</definedName>`, len(families)+1))
	subgroupLastRow := len(catalogSubgroupNames(families)) + 1
	if subgroupLastRow < 2 {
		subgroupLastRow = 2
	}
	workbook.WriteString(fmt.Sprintf(`<definedName name="Subgrupos">Listas!$B$2:$B$%d</definedName>`, subgroupLastRow))
	workbook.WriteString(`</definedNames></workbook>`)
	return workbook.String()
}

func catalogSubgroupNames(families []domain.ProductFamily) []string {
	var subgroups []string
	for _, family := range families {
		for _, subgroup := range family.Subgroups {
			subgroups = append(subgroups, subgroup.Name)
		}
	}
	return subgroups
}

const catalogXLSXContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`

const catalogXLSXWorkbookRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet3.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
