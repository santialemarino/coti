// Package spreadsheet reads and writes the tabular formats supported by imports and exports.
package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const maxXLSXEntryBytes = 64 << 20

// Column declares one logical import field and its accepted spreadsheet headers.
type Column struct {
	Key      string
	Headers  []string
	Required bool
}

// Row is one non-empty spreadsheet row mapped to a schema.
type Row struct {
	Number int
	Values map[string]string
}

// Schema declares how spreadsheet columns map to logical import fields.
type Schema struct {
	Columns []Column
}

type record struct {
	number int
	values []string
}

type xlsxCell struct {
	Reference string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Value     string `xml:"v"`
	Inline    struct {
		Text string `xml:"t"`
	} `xml:"is"`
}

type xlsxRow struct {
	Number int        `xml:"r,attr"`
	Cells  []xlsxCell `xml:"c"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxSharedString struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

type xlsxSharedStrings struct {
	Items []xlsxSharedString `xml:"si"`
}

type xlsxWorkbook struct {
	Sheets []struct {
		RelationshipID string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type xlsxRelationships struct {
	Items []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

// Read parses CSV or XLSX content and maps its first row through the supplied schema.
func Read(filename string, src io.Reader, schema Schema) ([]Row, error) {
	records, err := readRecords(filename, src)
	if err != nil {
		return nil, err
	}
	return mapRecords(records, schema)
}

func readRecords(filename string, src io.Reader) ([]record, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		return readCSV(src)
	case ".xlsx":
		return readXLSX(src)
	default:
		return nil, fmt.Errorf("unsupported file type %q", strings.ToLower(filepath.Ext(filename)))
	}
}

func readCSV(src io.Reader) ([]record, error) {
	content, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	comma := ','
	if firstLine, _, _ := bytes.Cut(content, []byte("\n")); bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		comma = ';'
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = comma
	reader.TrimLeadingSpace = true
	var records []record
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read csv: %w", readErr)
		}
		line := len(records) + 1
		if len(row) > 0 {
			line, _ = reader.FieldPos(0)
		}
		records = append(records, record{number: line, values: row})
	}
	return records, nil
}

func readXLSX(src io.Reader) ([]record, error) {
	content, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[path.Clean(filepath.ToSlash(file.Name))] = file
	}
	worksheetFile, err := firstWorksheet(files)
	if err != nil {
		return nil, err
	}
	if worksheetFile == nil {
		return nil, fmt.Errorf("xlsx has no first worksheet")
	}
	sharedStrings, err := readSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	worksheetContent, err := readZIPFile(worksheetFile)
	if err != nil {
		return nil, err
	}
	var worksheet xlsxWorksheet
	if err := xml.Unmarshal(worksheetContent, &worksheet); err != nil {
		return nil, fmt.Errorf("read first worksheet: %w", err)
	}
	records := make([]record, 0, len(worksheet.Rows))
	for index, row := range worksheet.Rows {
		values := make([]string, 0)
		for _, cell := range row.Cells {
			column := columnIndex(cell.Reference)
			for len(values) <= column {
				values = append(values, "")
			}
			value := cell.Value
			switch cell.Type {
			case "s":
				sharedIndex, parseErr := strconv.Atoi(cell.Value)
				if parseErr != nil || sharedIndex < 0 || sharedIndex >= len(sharedStrings) {
					return nil, fmt.Errorf("invalid shared string in cell %s", cell.Reference)
				}
				value = sharedStrings[sharedIndex]
			case "inlineStr":
				value = cell.Inline.Text
			}
			values[column] = value
		}
		number := row.Number
		if number == 0 {
			number = index + 1
		}
		records = append(records, record{number: number, values: values})
	}
	return records, nil
}

func firstWorksheet(files map[string]*zip.File) (*zip.File, error) {
	workbookFile := files["xl/workbook.xml"]
	relationshipsFile := files["xl/_rels/workbook.xml.rels"]
	if workbookFile == nil || relationshipsFile == nil {
		return files["xl/worksheets/sheet1.xml"], nil
	}
	workbookContent, err := readZIPFile(workbookFile)
	if err != nil {
		return nil, err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookContent, &workbook); err != nil {
		return nil, fmt.Errorf("read xlsx workbook: %w", err)
	}
	if len(workbook.Sheets) == 0 {
		return nil, nil
	}
	relationshipsContent, err := readZIPFile(relationshipsFile)
	if err != nil {
		return nil, err
	}
	var relationships xlsxRelationships
	if err := xml.Unmarshal(relationshipsContent, &relationships); err != nil {
		return nil, fmt.Errorf("read xlsx workbook relationships: %w", err)
	}
	for _, relationship := range relationships.Items {
		if relationship.ID == workbook.Sheets[0].RelationshipID {
			target := strings.TrimPrefix(relationship.Target, "/")
			if !strings.HasPrefix(target, "xl/") {
				target = path.Join("xl", target)
			}
			return files[path.Clean(target)], nil
		}
	}
	return nil, nil
}

func readSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	content, err := readZIPFile(file)
	if err != nil {
		return nil, err
	}
	var table xlsxSharedStrings
	if err := xml.Unmarshal(content, &table); err != nil {
		return nil, fmt.Errorf("read xlsx shared strings: %w", err)
	}
	result := make([]string, len(table.Items))
	for index, item := range table.Items {
		result[index] = item.Text
		for _, run := range item.Runs {
			result[index] += run.Text
		}
	}
	return result, nil
}

func readZIPFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxXLSXEntryBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxXLSXEntryBytes {
		return nil, fmt.Errorf("xlsx entry %q is too large", file.Name)
	}
	return content, nil
}

func mapRecords(records []record, schema Schema) ([]Row, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("spreadsheet is empty")
	}
	headers := make(map[string]int, len(records[0].values))
	for index, header := range records[0].values {
		headers[normalizeHeader(header)] = index
	}
	indexes := make(map[string]int, len(schema.Columns))
	var missing []string
	for _, column := range schema.Columns {
		index, ok := findHeader(headers, column.Headers)
		indexes[column.Key] = index
		if column.Required && !ok {
			missing = append(missing, firstName(column))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("spreadsheet needs %s columns", strings.Join(missing, ", "))
	}
	rows := make([]Row, 0, len(records)-1)
	for _, current := range records[1:] {
		if rowIsEmpty(current.values) {
			continue
		}
		values := make(map[string]string, len(schema.Columns))
		for _, column := range schema.Columns {
			values[column.Key] = valueAt(current.values, indexes[column.Key])
		}
		rows = append(rows, Row{Number: current.number, Values: values})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("spreadsheet has no data rows")
	}
	return rows, nil
}

func normalizeHeader(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", " ", "_", "-", "_")
	return replacer.Replace(normalized)
}

func findHeader(headers map[string]int, names []string) (int, bool) {
	for _, name := range names {
		if index, ok := headers[normalizeHeader(name)]; ok {
			return index, true
		}
	}
	return -1, false
}

func firstName(column Column) string {
	if len(column.Headers) > 0 {
		return column.Headers[0]
	}
	return column.Key
}

func valueAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func rowIsEmpty(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func columnIndex(reference string) int {
	column := 0
	for _, char := range reference {
		if char < 'A' || char > 'Z' {
			break
		}
		column = column*26 + int(char-'A'+1)
	}
	if column == 0 {
		return 0
	}
	return column - 1
}
