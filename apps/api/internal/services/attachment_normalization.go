package services

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

// spreadsheetCellSeparator joins a row's cells for the model. A tab survives commas and
// semicolons inside a cell, which a client's own description regularly carries.
const spreadsheetCellSeparator = "\t"

// spreadsheetOrderText turns an uploaded sheet into the text a model reads. Both doors into the
// engine share it, so a sheet is an order or a catalog by its own size and never by whether it
// arrived inline or through the sweep.
func spreadsheetOrderText(filename string, data []byte, maxRows int) (string, error) {
	rows, err := spreadsheet.ReadRaw(filename, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: the spreadsheet could not be read: %s",
			domain.ErrInvalidInput, err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("%w: the spreadsheet has no rows", domain.ErrInvalidInput)
	}
	if maxRows > 0 && len(rows) > maxRows {
		return "", fmt.Errorf("%w: the spreadsheet has %d rows and the limit is %d, which is a "+
			"catalog rather than an order", domain.ErrInvalidInput, len(rows), maxRows)
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, strings.Join(row, spreadsheetCellSeparator))
	}
	return strings.Join(lines, "\n"), nil
}
