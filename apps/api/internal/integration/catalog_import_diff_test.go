//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

const catalogDiffSeed = "codigo;nombre;descripcion;unidad;familia;subgrupo;precio;precio_minimo;activo\n" +
	"ARE-1;Arena fina;Lavada;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1500;1200;SI\n" +
	"ARE-2;Arena gruesa;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1400,50;;SI\n" +
	"CEM-1;Cemento;;bolsa;MATERIALES DE CONSTRUCCION;;9800;;SI\n"

// A downloaded workbook uploaded untouched changes nothing, and editing one cell changes one row.
func TestCatalogImport_ReimportingTheExportCountsOnlyRealChanges(t *testing.T) {
	e, token, branch, accountID := newCatalogDiffEnv(t)

	exported := e.download(t, token, branch, "/v1/products/export")
	preview := e.previewCatalog(t, token, branch, "catalogo.xlsx", exported)
	if preview.CreateRows != 0 || preview.UpdateRows != 0 || preview.UnchangedRows != 3 || preview.CanConfirm {
		t.Fatalf("untouched export preview = create %d, update %d, unchanged %d, can confirm %v; want 0, 0, 3, false",
			preview.CreateRows, preview.UpdateRows, preview.UnchangedRows, preview.CanConfirm)
	}

	// A client sending every row anyway writes nothing.
	before := e.catalogFootprint(t, accountID)
	result := e.confirmCatalog(t, token, branch, preview.Rows)
	if result.CreatedRows+result.UpdatedRows != 0 || result.UnchangedRows != 3 {
		t.Fatalf("confirm of an untouched export = %+v, want 3 unchanged and nothing written", result)
	}
	if after := e.catalogFootprint(t, accountID); after != before {
		t.Fatalf("footprint after an unchanged confirm = %+v, want %+v", after, before)
	}

	edited := editCell(t, exported, "ARE-2", "precio", "1450")
	preview = e.previewCatalog(t, token, branch, "catalogo.csv", edited)
	if preview.UpdateRows != 1 || preview.UnchangedRows != 2 || preview.CreateRows != 0 || !preview.CanConfirm {
		t.Fatalf("one-cell edit preview = create %d, update %d, unchanged %d; want 0, 1, 2",
			preview.CreateRows, preview.UpdateRows, preview.UnchangedRows)
	}
	for _, row := range preview.Rows {
		if want := map[bool]string{true: "UPDATE", false: "UNCHANGED"}[row.Code == "ARE-2"]; row.Action != want {
			t.Errorf("row %s action = %s, want %s", row.Code, row.Action, want)
		}
	}
	result = e.confirmCatalog(t, token, branch, preview.Rows)
	if result.UpdatedRows != 1 || result.UnchangedRows != 2 {
		t.Fatalf("confirm of one edit = %+v, want 1 updated and 2 unchanged", result)
	}
	after := e.catalogFootprint(t, accountID)
	if after.prices != before.prices+1 || after.closed != before.closed+1 ||
		after.productsTouched != before.productsTouched || after.availabilityTouched != before.availabilityTouched+1 {
		t.Fatalf("footprint after one price edit = %+v, want only ARE-2 repriced and rewritten (before %+v)",
			after, before)
	}
}

// The branch price list re-imported untouched closes and opens no price period.
func TestPriceImport_ReimportingTheExportWritesNoPeriod(t *testing.T) {
	e, token, branch, accountID := newCatalogDiffEnv(t)

	exported := e.download(t, token, branch, "/v1/product-prices/export")
	preview := e.previewPrices(t, token, branch, "precios.xlsx", exported)
	if preview.ValidRows != 3 || preview.UnchangedRows != 3 || preview.CanConfirm {
		t.Fatalf("untouched price list preview = valid %d, unchanged %d, can confirm %v; want 3, 3, false",
			preview.ValidRows, preview.UnchangedRows, preview.CanConfirm)
	}

	before := e.catalogFootprint(t, accountID)
	result := e.confirmPrices(t, token, branch, preview.Rows)
	if result.ImportedRows != 0 || result.UnchangedRows != 3 {
		t.Fatalf("confirm of an untouched price list = %+v, want 0 imported and 3 unchanged", result)
	}
	if after := e.catalogFootprint(t, accountID); after != before {
		t.Fatalf("footprint after an unchanged price import = %+v, want %+v", after, before)
	}

	edited := editCell(t, exported, "CEM-1", "precio", "9900")
	preview = e.previewPrices(t, token, branch, "precios.csv", edited)
	if preview.UnchangedRows != 2 || !preview.CanConfirm {
		t.Fatalf("one-cell edit preview = unchanged %d, can confirm %v; want 2, true", preview.UnchangedRows, preview.CanConfirm)
	}
	result = e.confirmPrices(t, token, branch, preview.Rows)
	if result.ImportedRows != 1 || result.UnchangedRows != 2 {
		t.Fatalf("confirm of one edit = %+v, want 1 imported and 2 unchanged", result)
	}
	if after := e.catalogFootprint(t, accountID); after.prices != before.prices+1 || after.closed != before.closed+1 {
		t.Fatalf("footprint after one price edit = %+v, want one period opened and one closed (before %+v)", after, before)
	}
}

// newCatalogDiffEnv seeds an account whose catalog was loaded through the import itself.
func newCatalogDiffEnv(t *testing.T) (*env, string, string, uuid.UUID) {
	t.Helper()
	e := newEnv(t, func(c *config.Config) {
		c.CatalogImport.MaxBytes, c.PriceImport.MaxBytes = 1<<20, 1<<20
	})
	accountID, branchID := e.seedAccount(t, "Catalog diff")
	token := e.tokenFor(t, e.seedUser(t, accountID, domain.UserRoleAdmin))
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_price WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM branch_product WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM product WHERE account_id = $1`, accountID)
	})
	branch := branchID.String()
	preview := e.previewCatalog(t, token, branch, "catalogo.csv", []byte(catalogDiffSeed))
	if preview.CreateRows != 3 || preview.InvalidRows != 0 {
		t.Fatalf("seed preview = %+v, want 3 rows to create", preview)
	}
	if result := e.confirmCatalog(t, token, branch, preview.Rows); result.CreatedRows != 3 {
		t.Fatalf("seed confirm = %+v, want 3 created", result)
	}
	return e, token, branch, accountID
}

// catalogFootprint is what an import can leave behind: price periods opened and closed, and
// product and availability rows rewritten since they were created.
type catalogFootprint struct {
	prices, closed, productsTouched, availabilityTouched int
}

func (e *env) catalogFootprint(t *testing.T, accountID uuid.UUID) catalogFootprint {
	t.Helper()
	var f catalogFootprint
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM product_price WHERE account_id = $1),
		        (SELECT count(*) FROM product_price WHERE account_id = $1 AND valid_to IS NOT NULL),
		        (SELECT count(*) FROM product WHERE account_id = $1 AND updated_at > created_at),
		        (SELECT count(*) FROM branch_product WHERE account_id = $1 AND updated_at > created_at)`,
		accountID).Scan(&f.prices, &f.closed, &f.productsTouched, &f.availabilityTouched); err != nil {
		t.Fatal(err)
	}
	return f
}

func (e *env) download(t *testing.T, token, branch, path string) []byte {
	t.Helper()
	rec := e.do(t, request{method: http.MethodGet, path: path, token: token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

func (e *env) previewCatalog(t *testing.T, token, branch, filename string, content []byte) dto.CatalogImportPreviewResponse {
	t.Helper()
	var preview dto.CatalogImportPreviewResponse
	e.upload(t, token, branch, "/v1/products/import/preview", filename, content, &preview)
	return preview
}

func (e *env) previewPrices(t *testing.T, token, branch, filename string, content []byte) dto.ProductPriceImportPreviewResponse {
	t.Helper()
	var preview dto.ProductPriceImportPreviewResponse
	e.upload(t, token, branch, "/v1/product-prices/import/preview", filename, content, &preview)
	return preview
}

func (e *env) confirmCatalog(t *testing.T, token, branch string, rows []dto.CatalogImportRowResponse) dto.ConfirmCatalogImportResponse {
	t.Helper()
	inputs := make([]dto.CatalogImportInput, 0, len(rows))
	for _, row := range rows {
		inputs = append(inputs, dto.CatalogImportInput{
			Code: row.Code, Name: row.Name, Description: row.Description, Unit: row.Unit,
			Family: row.Family, Subgroup: row.Subgroup, Price: row.Price, MinPrice: row.MinPrice,
			IsActive: row.IsActive, EconomyAlternative: row.EconomyAlternative,
			PremiumAlternative: row.PremiumAlternative,
		})
	}
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/products/import/confirm", token: token,
		branch: branch, body: dto.ConfirmCatalogImportRequest{Rows: inputs}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("confirm catalog = %d %s", rec.Code, rec.Body)
	}
	var result dto.ConfirmCatalogImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (e *env) confirmPrices(t *testing.T, token, branch string, rows []dto.ProductPriceImportRowResponse) dto.ConfirmProductPriceImportResponse {
	t.Helper()
	inputs := make([]dto.ProductPriceImportInput, 0, len(rows))
	for _, row := range rows {
		inputs = append(inputs, dto.ProductPriceImportInput{Code: row.Code, Price: row.Price, MinPrice: row.MinPrice})
	}
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/product-prices/import/confirm", token: token,
		branch: branch, body: dto.ConfirmProductPriceImportRequest{Rows: inputs}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("confirm prices = %d %s", rec.Code, rec.Body)
	}
	var result dto.ConfirmProductPriceImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (e *env) upload(t *testing.T, token, branch, path, filename string, content []byte, out any) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Branch-Id", branch)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s = %d %s", path, rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatal(err)
	}
}

// editCell reads a downloaded workbook's first sheet, sets one cell of the row with the given code,
// and returns the sheet as the CSV a spreadsheet would save.
func editCell(t *testing.T, workbook []byte, code, column, value string) []byte {
	t.Helper()
	rows, err := spreadsheet.ReadRaw(bytes.NewReader(workbook))
	if err != nil {
		t.Fatal(err)
	}
	columnIndex := slices.Index(rows[0], column)
	if columnIndex < 0 {
		t.Fatalf("no %q column in %v", column, rows[0])
	}
	edited := false
	for i := range rows {
		// A workbook omits trailing empty cells; a CSV row carries every column.
		for len(rows[i]) < len(rows[0]) {
			rows[i] = append(rows[i], "")
		}
		if i > 0 && rows[i][0] == code {
			rows[i][columnIndex], edited = value, true
		}
	}
	if !edited {
		t.Fatalf("no row with code %q", code)
	}
	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	writer.Comma = ';'
	if err := writer.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
