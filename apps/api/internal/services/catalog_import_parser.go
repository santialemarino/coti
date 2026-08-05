package services

import (
	"io"

	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

const (
	catalogColumnCode        = "code"
	catalogColumnName        = "name"
	catalogColumnDescription = "description"
	catalogColumnUnit        = "unit"
	catalogColumnFamily      = "family"
	catalogColumnSubgroup    = "subgroup"
	catalogColumnPrice       = "price"
	catalogColumnMinPrice    = "min_price"
)

var catalogImportSchema = spreadsheet.Schema{Columns: []spreadsheet.Column{
	{Key: catalogColumnCode, Headers: []string{"codigo", "code"}, Required: true},
	{Key: catalogColumnName, Headers: []string{"nombre", "name", "canonical_name"}, Required: true},
	{Key: catalogColumnDescription, Headers: []string{"descripcion", "description"}},
	{Key: catalogColumnUnit, Headers: []string{"unidad", "unit"}, Required: true},
	{Key: catalogColumnFamily, Headers: []string{"familia", "family"}, Required: true},
	{Key: catalogColumnSubgroup, Headers: []string{"subgrupo", "subgroup"}},
	{Key: catalogColumnPrice, Headers: []string{"precio", "price"}, Required: true},
	{Key: catalogColumnMinPrice, Headers: []string{"precio_minimo", "min_price", "minimum_price"}},
}}

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
	rows, err := spreadsheet.Read(filename, src, catalogImportSchema)
	if err != nil {
		return nil, err
	}
	result := make([]catalogImportRawRow, len(rows))
	for index, row := range rows {
		result[index] = catalogImportRawRow{
			rowNumber:   row.Number,
			code:        row.Values[catalogColumnCode],
			name:        row.Values[catalogColumnName],
			description: row.Values[catalogColumnDescription],
			unit:        row.Values[catalogColumnUnit],
			family:      row.Values[catalogColumnFamily],
			subgroup:    row.Values[catalogColumnSubgroup],
			price:       row.Values[catalogColumnPrice],
			minPrice:    row.Values[catalogColumnMinPrice],
		}
	}
	return result, nil
}
