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
	family      string
	subgroup    string
	price       string
	minPrice    string
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
	nameIndex, nameOK := firstHeader(headers, "nombre", "name", "canonical_name")
	unitIndex, unitOK := firstHeader(headers, "unidad", "unit")
	familyIndex, familyOK := firstHeader(headers, "familia", "family")
	priceIndex, priceOK := firstHeader(headers, "precio", "price")
	if !codeOK || !nameOK || !unitOK || !familyOK || !priceOK {
		return nil, fmt.Errorf("spreadsheet needs codigo, nombre, unidad, familia and precio columns")
	}
	descriptionIndex, _ := firstHeader(headers, "descripcion", "description")
	subgroupIndex, _ := firstHeader(headers, "subgrupo", "subgroup")
	minPriceIndex, _ := firstHeader(headers, "precio_minimo", "min_price", "minimum_price")

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
			family:      valueAt(record, familyIndex),
			subgroup:    valueAt(record, subgroupIndex),
			price:       valueAt(record, priceIndex),
			minPrice:    valueAt(record, minPriceIndex),
		})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("spreadsheet has no data rows")
	}
	return rows, nil
}
