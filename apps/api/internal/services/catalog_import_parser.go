package services

import (
	"io"
	"strings"

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
	catalogColumnActive      = "active"
	catalogColumnEconomy     = "economy_alternative"
	catalogColumnPremium     = "premium_alternative"
)

// catalogClearAlternative are the cell values that clear a recommendation slot; blank keeps it.
var catalogClearAlternative = []string{"NINGUNA", "NINGUNO", "-"}

var catalogImportSchema = spreadsheet.Schema{Columns: []spreadsheet.Column{
	{Key: catalogColumnCode, Headers: []string{"codigo", "code"}, Required: true},
	{Key: catalogColumnName, Headers: []string{"nombre", "name", "canonical_name"}, Required: true},
	{Key: catalogColumnDescription, Headers: []string{"descripcion", "description"}},
	{Key: catalogColumnUnit, Headers: []string{"unidad", "unit"}, Required: true},
	{Key: catalogColumnFamily, Headers: []string{"familia", "family"}, Required: true},
	{Key: catalogColumnSubgroup, Headers: []string{"subgrupo", "subgroup"}},
	{Key: catalogColumnPrice, Headers: []string{"precio", "price"}, Required: true},
	{Key: catalogColumnMinPrice, Headers: []string{"precio_minimo", "min_price", "minimum_price"}},
	{Key: catalogColumnActive, Headers: []string{"activo", "active"}},
	{Key: catalogColumnEconomy, Headers: []string{"alternativa_economica", "economy_alternative"}},
	{Key: catalogColumnPremium, Headers: []string{"alternativa_premium", "premium_alternative"}},
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
	active      string
	// nil keeps the slot, an empty code clears it, a code sets it.
	economyAlternative *string
	premiumAlternative *string
	// The cell as written when it was read as a clear word, to flag one that is also a product code.
	economyClearWord string
	premiumClearWord string
}

func parseCatalogImport(src io.Reader) ([]catalogImportRawRow, error) {
	rows, err := spreadsheet.Read(src, catalogImportSchema)
	if err != nil {
		return nil, err
	}
	result := make([]catalogImportRawRow, len(rows))
	for index, row := range rows {
		economy, economyClearWord := alternativeCell(row.Values[catalogColumnEconomy])
		premium, premiumClearWord := alternativeCell(row.Values[catalogColumnPremium])
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
			active:      row.Values[catalogColumnActive],

			economyAlternative: economy,
			premiumAlternative: premium,
			economyClearWord:   economyClearWord,
			premiumClearWord:   premiumClearWord,
		}
	}
	return result, nil
}

// alternativeCell reads a recommendation slot cell: blank keeps the slot, a clear word empties it
// and is returned as written.
func alternativeCell(raw string) (*string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, ""
	}
	for _, clear := range catalogClearAlternative {
		if strings.EqualFold(value, clear) {
			empty := ""
			return &empty, value
		}
	}
	return &value, ""
}
