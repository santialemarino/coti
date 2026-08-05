package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// ColumnWidth configures a contiguous range of worksheet columns.
type ColumnWidth struct {
	Min   int
	Max   int
	Width int
}

// DataValidation configures one list validation over a worksheet range.
type DataValidation struct {
	Range        string
	Formula      string
	AllowBlank   bool
	ErrorTitle   string
	ErrorMessage string
}

// DefinedName exposes a workbook formula under a stable name.
type DefinedName struct {
	Name    string
	Formula string
}

// ExportRow is one row rendered into an XLSX worksheet.
type ExportRow struct {
	Number int
	Values []string
	Header bool
}

// Sheet declares one XLSX worksheet.
type Sheet struct {
	Name            string
	Hidden          bool
	Rows            []ExportRow
	ColumnWidths    []ColumnWidth
	FreezeHeader    bool
	AutoFilter      bool
	DataValidations []DataValidation
}

// Workbook declares an XLSX file independently from any product feature.
type Workbook struct {
	Sheets       []Sheet
	DefinedNames []DefinedName
}

// Write creates an XLSX file from a declarative workbook.
func Write(workbook Workbook) ([]byte, error) {
	if len(workbook.Sheets) == 0 {
		return nil, fmt.Errorf("workbook needs at least one sheet")
	}
	seen := make(map[string]struct{}, len(workbook.Sheets))
	for _, sheet := range workbook.Sheets {
		if strings.TrimSpace(sheet.Name) == "" {
			return nil, fmt.Errorf("sheet name is required")
		}
		if _, exists := seen[sheet.Name]; exists {
			return nil, fmt.Errorf("duplicate sheet name %q", sheet.Name)
		}
		seen[sheet.Name] = struct{}{}
	}

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := []struct {
		name    string
		content string
	}{
		{"[Content_Types].xml", buildContentTypes(len(workbook.Sheets))},
		{"_rels/.rels", rootRelationships},
		{"xl/workbook.xml", buildWorkbook(workbook)},
		{"xl/_rels/workbook.xml.rels", buildWorkbookRelationships(len(workbook.Sheets))},
		{"xl/styles.xml", styles},
	}
	for index, sheet := range workbook.Sheets {
		files = append(files, struct {
			name    string
			content string
		}{fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1), buildSheet(sheet)})
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

func buildSheet(sheet Sheet) string {
	var output strings.Builder
	output.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	output.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	if sheet.FreezeHeader {
		output.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	}
	writeColumns(&output, sheet.ColumnWidths)
	output.WriteString(`<sheetData>`)
	maxRow, maxColumn := 0, 0
	for index, row := range sheet.Rows {
		number := row.Number
		if number == 0 {
			number = index + 1
		}
		writeRow(&output, number, row.Values, row.Header)
		if number > maxRow {
			maxRow = number
		}
		if len(row.Values) > maxColumn {
			maxColumn = len(row.Values)
		}
	}
	output.WriteString(`</sheetData>`)
	if sheet.AutoFilter && maxRow > 0 && maxColumn > 0 {
		output.WriteString(fmt.Sprintf(`<autoFilter ref="A1:%s%d"/>`, columnName(maxColumn), maxRow))
	}
	writeValidations(&output, sheet.DataValidations)
	output.WriteString(`</worksheet>`)
	return output.String()
}

func writeColumns(output *strings.Builder, widths []ColumnWidth) {
	if len(widths) == 0 {
		return
	}
	output.WriteString(`<cols>`)
	for _, width := range widths {
		output.WriteString(fmt.Sprintf(`<col min="%d" max="%d" width="%d" customWidth="1"/>`, width.Min, width.Max, width.Width))
	}
	output.WriteString(`</cols>`)
}

func writeRow(output *strings.Builder, number int, values []string, header bool) {
	output.WriteString(fmt.Sprintf(`<row r="%d">`, number))
	for index, value := range values {
		style := ""
		if header {
			style = ` s="1"`
		}
		output.WriteString(fmt.Sprintf(`<c r="%s%d" t="inlineStr"%s><is><t xml:space="preserve">`, columnName(index+1), number, style))
		_ = xml.EscapeText(output, []byte(value))
		output.WriteString(`</t></is></c>`)
	}
	output.WriteString(`</row>`)
}

func writeValidations(output *strings.Builder, validations []DataValidation) {
	if len(validations) == 0 {
		return
	}
	output.WriteString(fmt.Sprintf(`<dataValidations count="%d">`, len(validations)))
	for _, validation := range validations {
		allowBlank := 0
		if validation.AllowBlank {
			allowBlank = 1
		}
		output.WriteString(fmt.Sprintf(`<dataValidation type="list" allowBlank="%d" showErrorMessage="1" errorTitle="`, allowBlank))
		_ = xml.EscapeText(output, []byte(validation.ErrorTitle))
		output.WriteString(`" error="`)
		_ = xml.EscapeText(output, []byte(validation.ErrorMessage))
		output.WriteString(`" sqref="`)
		_ = xml.EscapeText(output, []byte(validation.Range))
		output.WriteString(`"><formula1>`)
		_ = xml.EscapeText(output, []byte(validation.Formula))
		output.WriteString(`</formula1></dataValidation>`)
	}
	output.WriteString(`</dataValidations>`)
}

func buildWorkbook(workbook Workbook) string {
	var output strings.Builder
	output.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for index, sheet := range workbook.Sheets {
		output.WriteString(`<sheet name="`)
		_ = xml.EscapeText(&output, []byte(sheet.Name))
		output.WriteString(fmt.Sprintf(`" sheetId="%d"`, index+1))
		if sheet.Hidden {
			output.WriteString(` state="hidden"`)
		}
		output.WriteString(fmt.Sprintf(` r:id="rId%d"/>`, index+1))
	}
	output.WriteString(`</sheets>`)
	if len(workbook.DefinedNames) > 0 {
		output.WriteString(`<definedNames>`)
		for _, name := range workbook.DefinedNames {
			output.WriteString(`<definedName name="`)
			_ = xml.EscapeText(&output, []byte(name.Name))
			output.WriteString(`">`)
			_ = xml.EscapeText(&output, []byte(name.Formula))
			output.WriteString(`</definedName>`)
		}
		output.WriteString(`</definedNames>`)
	}
	output.WriteString(`</workbook>`)
	return output.String()
}

func buildContentTypes(sheetCount int) string {
	var output strings.Builder
	output.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for index := 1; index <= sheetCount; index++ {
		output.WriteString(fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, index))
	}
	output.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`)
	return output.String()
}

func buildWorkbookRelationships(sheetCount int) string {
	var output strings.Builder
	output.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for index := 1; index <= sheetCount; index++ {
		output.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, index, index))
	}
	output.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`, sheetCount+1))
	return output.String()
}

func columnName(column int) string {
	var name string
	for column > 0 {
		column--
		name = string(rune('A'+column%26)) + name
		column /= 26
	}
	return name
}

const rootRelationships = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const styles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
