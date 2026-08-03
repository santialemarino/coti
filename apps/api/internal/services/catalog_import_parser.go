package services

import (
	"fmt"
	"io"
)

type catalogImportRawRow struct {
	rowNumber   int
	code        string
	name        string
	description string
	unit        string
	category    string
	price       string
	minPrice    string
	currency    string
	conditions  string
}

func parseCatalogImport(filename string, src io.Reader) ([]catalogImportRawRow, error) {
	records, err := readImportRecords(filename, src)
	if err != nil {
		return nil, err
	}
	return mapCatalogImportRows(records)
}

func mapCatalogImportRows(records [][]string) ([]catalogImportRawRow, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("spreadsheet is empty")
	}
	headers := make(map[string]int, len(records[0]))
	for index, header := range records[0] {
		headers[normalizeImportHeader(header)] = index
	}
	codeIndex, codeOK := firstHeader(headers, "codigo", "code")
	descriptionIndex, descriptionOK := firstHeader(headers, "descripcion", "description")
	unitIndex, unitOK := firstHeader(headers, "unidad", "unit")
	priceIndex, priceOK := firstHeader(headers, "precio", "price")
	if !codeOK || !descriptionOK || !unitOK || !priceOK {
		return nil, fmt.Errorf("spreadsheet needs codigo, descripcion, unidad and precio columns")
	}
	nameIndex, _ := firstHeader(headers, "nombre", "name")
	categoryIndex, _ := firstHeader(headers, "categoria", "category")
	minPriceIndex, _ := firstHeader(headers, "precio_minimo", "min_price", "minimum_price")
	currencyIndex, _ := firstHeader(headers, "moneda", "currency")
	conditionsIndex, _ := firstHeader(headers, "condiciones", "conditions")

	rows := make([]catalogImportRawRow, 0, len(records)-1)
	for index, record := range records[1:] {
		if rowIsEmpty(record) {
			continue
		}
		rows = append(rows, catalogImportRawRow{
			rowNumber:   index + 2,
			code:        valueAt(record, codeIndex),
			name:        valueAt(record, nameIndex),
			description: valueAt(record, descriptionIndex),
			unit:        valueAt(record, unitIndex),
			category:    valueAt(record, categoryIndex),
			price:       valueAt(record, priceIndex),
			minPrice:    valueAt(record, minPriceIndex),
			currency:    valueAt(record, currencyIndex),
			conditions:  valueAt(record, conditionsIndex),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("spreadsheet has no data rows")
	}
	return rows, nil
}
