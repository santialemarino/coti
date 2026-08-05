package dto

import "time"

// ConfirmProductPriceImportRequest is the body for POST /v1/product-prices/import/confirm.
type ConfirmProductPriceImportRequest struct {
	Rows []ProductPriceImportInput `json:"rows" binding:"required,min=1,dive"`
}

// ProductPriceImportInput is a reviewed spreadsheet row sent for confirmation.
type ProductPriceImportInput struct {
	Code     string  `json:"code" binding:"required,max=255"`
	Price    string  `json:"price" binding:"required"`
	MinPrice *string `json:"min_price"`
	Currency string  `json:"currency" binding:"omitempty,len=3"`
}

// ProductPriceImportRowResponse is one validated row in the import preview.
type ProductPriceImportRowResponse struct {
	RowNumber       int      `json:"row_number"`
	Code            string   `json:"code"`
	ProductName     string   `json:"product_name"`
	CurrentPrice    *string  `json:"current_price"`
	CurrentMinPrice *string  `json:"current_min_price"`
	Price           string   `json:"price"`
	MinPrice        *string  `json:"min_price"`
	Currency        string   `json:"currency"`
	Errors          []string `json:"errors"`
}

// ProductPriceImportPreviewResponse is returned by POST /v1/product-prices/import/preview.
type ProductPriceImportPreviewResponse struct {
	Rows        []ProductPriceImportRowResponse `json:"rows"`
	ValidRows   int                             `json:"valid_rows"`
	InvalidRows int                             `json:"invalid_rows"`
	CanConfirm  bool                            `json:"can_confirm"`
	PreviewedAt time.Time                       `json:"previewed_at"`
}

// ConfirmProductPriceImportResponse reports how many new price versions were created.
type ConfirmProductPriceImportResponse struct {
	ImportedRows int `json:"imported_rows"`
}
