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
	Family      string  `json:"family"`
	Subgroup    *string `json:"subgroup"`
	Price       string  `json:"price"`
	MinPrice    *string `json:"min_price"`
	IsActive    bool    `json:"is_active"`
}

// CatalogImportRowResponse is one validated row in the catalog import preview.
type CatalogImportRowResponse struct {
	RowNumber   int      `json:"row_number"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`
	Family      string   `json:"family"`
	Subgroup    *string  `json:"subgroup"`
	Price       string   `json:"price"`
	MinPrice    *string  `json:"min_price"`
	IsActive    bool     `json:"is_active"`
	Action      string   `json:"action"`
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

// ConfirmCatalogImportResponse reports how many catalog rows were created, updated, or skipped.
type ConfirmCatalogImportResponse struct {
	CreatedRows int `json:"created_rows"`
	UpdatedRows int `json:"updated_rows"`
	SkippedRows int `json:"skipped_rows"`
}

// ProductSubgroupResponse is one taxonomy subgroup.
type ProductSubgroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProductFamilyResponse is one taxonomy family and its optional subgroups.
type ProductFamilyResponse struct {
	ID        string                    `json:"id"`
	Name      string                    `json:"name"`
	Subgroups []ProductSubgroupResponse `json:"subgroups"`
}

// ProductTaxonomyResponse is returned by GET /v1/product-taxonomy.
type ProductTaxonomyResponse struct {
	Families []ProductFamilyResponse `json:"families"`
}
