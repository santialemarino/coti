package services

import (
	"io"

	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

const (
	priceColumnCode     = "code"
	priceColumnPrice    = "price"
	priceColumnMinPrice = "min_price"
)

var priceImportSchema = spreadsheet.Schema{Columns: []spreadsheet.Column{
	{Key: priceColumnCode, Headers: []string{"codigo", "code"}, Required: true},
	{Key: priceColumnPrice, Headers: []string{"precio", "price"}, Required: true},
	{Key: priceColumnMinPrice, Headers: []string{"precio_minimo", "min_price", "minimum_price"}},
}}

type priceImportRawRow struct {
	rowNumber int
	code      string
	price     string
	minPrice  string
}

func parsePriceImport(filename string, src io.Reader) ([]priceImportRawRow, error) {
	rows, err := spreadsheet.Read(filename, src, priceImportSchema)
	if err != nil {
		return nil, err
	}
	result := make([]priceImportRawRow, len(rows))
	for index, row := range rows {
		result[index] = priceImportRawRow{
			rowNumber: row.Number,
			code:      row.Values[priceColumnCode],
			price:     row.Values[priceColumnPrice],
			minPrice:  row.Values[priceColumnMinPrice],
		}
	}
	return result, nil
}
