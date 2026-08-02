package services

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

// maxXLSXEntryBytes bounds what one zip entry may expand to. The upload limit only caps the
// compressed size, so without this a few megabytes of XLSX can decompress into gigabytes.
const maxXLSXEntryBytes = 64 << 20

type priceImportRawRow struct {
	rowNumber  int
	code       string
	price      string
	minPrice   string
	currency   string
	conditions string
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

func parsePriceImport(filename string, src io.Reader) ([]priceImportRawRow, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return parsePriceImportCSV(src)
	case ".xlsx":
		return parsePriceImportXLSX(src)
	default:
		return nil, fmt.Errorf("unsupported file type %q", ext)
	}
}

func parsePriceImportCSV(src io.Reader) ([]priceImportRawRow, error) {
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
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	return mapPriceImportRows(records)
}

func parsePriceImportXLSX(src io.Reader) ([]priceImportRawRow, error) {
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
	worksheetFile, err := firstXLSXWorksheet(files)
	if err != nil {
		return nil, err
	}
	if worksheetFile == nil {
		return nil, fmt.Errorf("xlsx has no first worksheet")
	}

	sharedStrings, err := readXLSXSharedStrings(files["xl/sharedStrings.xml"])
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

	rows := make([][]string, 0, len(worksheet.Rows))
	for _, row := range worksheet.Rows {
		values := make([]string, 0)
		for _, cell := range row.Cells {
			column := xlsxColumnIndex(cell.Reference)
			for len(values) <= column {
				values = append(values, "")
			}
			value := cell.Value
			switch cell.Type {
			case "s":
				index, parseErr := strconv.Atoi(cell.Value)
				if parseErr != nil || index < 0 || index >= len(sharedStrings) {
					return nil, fmt.Errorf("invalid shared string in cell %s", cell.Reference)
				}
				value = sharedStrings[index]
			case "inlineStr":
				value = cell.Inline.Text
			}
			values[column] = value
		}
		rows = append(rows, values)
	}
	return mapPriceImportRows(rows)
}

func firstXLSXWorksheet(files map[string]*zip.File) (*zip.File, error) {
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

func readXLSXSharedStrings(file *zip.File) ([]string, error) {
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
	for i, item := range table.Items {
		result[i] = item.Text
		for _, run := range item.Runs {
			result[i] += run.Text
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

func xlsxColumnIndex(reference string) int {
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

func mapPriceImportRows(records [][]string) ([]priceImportRawRow, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("spreadsheet is empty")
	}
	headers := make(map[string]int, len(records[0]))
	for index, header := range records[0] {
		headers[normalizeImportHeader(header)] = index
	}
	codeIndex, codeOK := firstHeader(headers, "codigo", "code")
	priceIndex, priceOK := firstHeader(headers, "precio", "price")
	if !codeOK || !priceOK {
		return nil, fmt.Errorf("spreadsheet needs codigo and precio columns")
	}
	minPriceIndex, _ := firstHeader(headers, "precio_minimo", "min_price", "minimum_price")
	currencyIndex, _ := firstHeader(headers, "moneda", "currency")
	conditionsIndex, _ := firstHeader(headers, "condiciones", "conditions")

	rows := make([]priceImportRawRow, 0, len(records)-1)
	for index, record := range records[1:] {
		if rowIsEmpty(record) {
			continue
		}
		rows = append(rows, priceImportRawRow{
			rowNumber:  index + 2,
			code:       valueAt(record, codeIndex),
			price:      valueAt(record, priceIndex),
			minPrice:   valueAt(record, minPriceIndex),
			currency:   valueAt(record, currencyIndex),
			conditions: valueAt(record, conditionsIndex),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("spreadsheet has no data rows")
	}
	return rows, nil
}

func normalizeImportHeader(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", " ", "_", "-", "_")
	return replacer.Replace(normalized)
}

func firstHeader(headers map[string]int, names ...string) (int, bool) {
	for _, name := range names {
		if index, ok := headers[name]; ok {
			return index, true
		}
	}
	return -1, false
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
