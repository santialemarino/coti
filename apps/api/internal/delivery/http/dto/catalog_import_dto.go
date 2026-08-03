package dto

import "time"

// ConfirmCatalogImportRequest is the body for POST /v1/products/import/confirm.
type ConfirmCatalogImportRequest struct {
	Rows []CatalogImportInput `json:"rows" binding:"required,min=1,dive"`
}

// CatalogImportInput is one reviewed spreadsheet row sent for confirmation.
type CatalogImportInput struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Category    *string `json:"category"`
	Price       string  `json:"price"`
	MinPrice    *string `json:"min_price"`
	Currency    string  `json:"currency"`
	Conditions  *string `json:"conditions"`
}

// CatalogImportRowResponse is one validated row in the catalog import preview.
type CatalogImportRowResponse struct {
	RowNumber   int      `json:"row_number"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`
	Category    *string  `json:"category"`
	Price       string   `json:"price"`
	MinPrice    *string  `json:"min_price"`
	Currency    string   `json:"currency"`
	Conditions  *string  `json:"conditions"`
	Errors      []string `json:"errors"`
}

// CatalogImportPreviewResponse is returned by POST /v1/products/import/preview.
type CatalogImportPreviewResponse struct {
	Rows        []CatalogImportRowResponse `json:"rows"`
	ValidRows   int                        `json:"valid_rows"`
	InvalidRows int                        `json:"invalid_rows"`
	CanConfirm  bool                       `json:"can_confirm"`
	PreviewedAt time.Time                  `json:"previewed_at"`
}

// ConfirmCatalogImportResponse reports how many catalog rows were created or skipped.
type ConfirmCatalogImportResponse struct {
	ImportedRows int `json:"imported_rows"`
	SkippedRows  int `json:"skipped_rows"`
}
