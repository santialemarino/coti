package services

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type priceImportTestDB struct{}

func (priceImportTestDB) InTenantTx(_ context.Context, _ domain.Tenant, fn func(repository.Querier) error) error {
	return fn(nil)
}

func (priceImportTestDB) CrossAccount() repository.Querier {
	return nil
}

type priceImportTestRepository struct {
	products map[string]domain.ProductPriceLookup
	export   *domain.ProductPriceExport
	applied  []domain.ProductPriceUpdate
}

func (r *priceImportTestRepository) ListCurrentForExport(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID,
) (*domain.ProductPriceExport, error) {
	return r.export, nil
}

func (r *priceImportTestRepository) GetByCodes(
	_ context.Context, _ repository.Querier, _, _ uuid.UUID, _ []string,
) (map[string]domain.ProductPriceLookup, error) {
	return r.products, nil
}

func (r *priceImportTestRepository) ApplyImport(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ time.Time,
	updates []domain.ProductPriceUpdate,
) error {
	r.applied = updates
	return nil
}

func TestProductPriceImportService_Preview_ReportsEveryInvalidRow(t *testing.T) {
	t.Parallel()
	productID := uuid.New()
	repo := &priceImportTestRepository{products: map[string]domain.ProductPriceLookup{
		"CEM-001": {ProductID: productID, Code: "CEM-001", ProductName: "Cemento"},
	}}
	service := NewProductPriceImportService(priceImportTestDB{}, repo, func() time.Time {
		return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	})
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	csvFile := "codigo;precio;precio_minimo\nCEM-001;10000;11000\nDESCONOCIDO;abc;"

	preview, err := service.Preview(context.Background(), tenant, "precios.csv", strings.NewReader(csvFile))
	if err != nil {
		t.Fatalf("Preview() = %v, want no error", err)
	}
	if preview.CanConfirm {
		t.Error("CanConfirm = true, want false")
	}
	if preview.InvalidRows != 2 {
		t.Errorf("InvalidRows = %d, want 2", preview.InvalidRows)
	}
	if len(preview.Rows[0].Errors) == 0 || len(preview.Rows[1].Errors) == 0 {
		t.Fatalf("Errors = %#v, want errors on both rows", preview.Rows)
	}
}

func TestProductPriceImportService_Confirm_AppliesReviewedRows(t *testing.T) {
	t.Parallel()
	productID := uuid.New()
	currency := "USD"
	repo := &priceImportTestRepository{products: map[string]domain.ProductPriceLookup{
		"CEM-001": {
			ProductID: productID, Code: "CEM-001", ProductName: "Cemento",
			CurrentCurrency: &currency,
		},
	}}
	service := NewProductPriceImportService(priceImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	minPrice := "9.500,00"

	count, err := service.Confirm(context.Background(), tenant, []domain.ProductPriceImportInput{{
		Code: "CEM-001", Price: "$ 10.000,00", MinPrice: &minPrice,
	}})
	if err != nil {
		t.Fatalf("Confirm() = %v, want no error", err)
	}
	if count != 1 || len(repo.applied) != 1 {
		t.Fatalf("count = %d, applied = %d; want 1, 1", count, len(repo.applied))
	}
	if repo.applied[0].Price != "10000.00" || *repo.applied[0].MinPrice != "9500.00" {
		t.Errorf("applied = %#v, want normalized decimal strings", repo.applied[0])
	}
	if repo.applied[0].Currency != currency {
		t.Errorf("currency = %q, want current currency %q", repo.applied[0].Currency, currency)
	}
}

func TestProductPriceImportService_Preview_DefaultsCurrencyWithoutCurrentPrice(t *testing.T) {
	t.Parallel()
	productID := uuid.New()
	repo := &priceImportTestRepository{products: map[string]domain.ProductPriceLookup{
		"CEM-001": {ProductID: productID, Code: "CEM-001", ProductName: "Cemento"},
	}}
	service := NewProductPriceImportService(priceImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	csvFile := "codigo;precio;moneda\nCEM-001;10000;USD"

	preview, err := service.Preview(context.Background(), tenant, "precios.csv", strings.NewReader(csvFile))
	if err != nil {
		t.Fatalf("Preview() = %v, want no error", err)
	}
	if preview.Rows[0].Currency != domain.DefaultCurrency {
		t.Errorf("currency = %q, want %q", preview.Rows[0].Currency, domain.DefaultCurrency)
	}
}

func TestProductPriceImportService_Export_CreatesImportableWorkbook(t *testing.T) {
	t.Parallel()
	minPrice := "9500.00"
	repo := &priceImportTestRepository{export: &domain.ProductPriceExport{
		BranchName: "Villa Bosch",
		Rows: []domain.ProductPriceExportRow{{
			Code: "CEM-001", ProductName: "Cemento", Price: "10000.00",
			MinPrice: &minPrice,
		}},
	}}
	service := NewProductPriceImportService(priceImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}

	file, err := service.Export(context.Background(), tenant)
	if err != nil {
		t.Fatalf("Export() = %v, want no error", err)
	}
	if file.Filename != "precios-villa-bosch.xlsx" {
		t.Errorf("Filename = %q, want precios-villa-bosch.xlsx", file.Filename)
	}
	rows, err := parsePriceImportXLSX(bytes.NewReader(file.Content))
	if err != nil {
		t.Fatalf("parse exported workbook = %v, want no error", err)
	}
	if len(rows) != 1 || rows[0].code != "CEM-001" || rows[0].price != "10000.00" {
		t.Fatalf("rows = %#v, want the exported price row", rows)
	}
	if rows[0].minPrice != "9500.00" {
		t.Errorf("row = %#v, want all editable values preserved", rows[0])
	}
	wantHeaders := []string{"codigo", "producto", "precio", "precio_minimo"}
	if strings.Join(priceExportHeaders, ",") != strings.Join(wantHeaders, ",") {
		t.Errorf("headers = %#v, want %#v", priceExportHeaders, wantHeaders)
	}
}

func TestParsePriceImportXLSX_ReadsInlineStrings(t *testing.T) {
	t.Parallel()
	var file bytes.Buffer
	writer := zip.NewWriter(&file)
	sheet, err := writer.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(sheet, `<?xml version="1.0"?><worksheet><sheetData>`+
		`<row r="1"><c r="A1" t="inlineStr"><is><t>codigo</t></is></c>`+
		`<c r="B1" t="inlineStr"><is><t>precio</t></is></c></row>`+
		`<row r="2"><c r="A2" t="inlineStr"><is><t>CEM-001</t></is></c>`+
		`<c r="B2"><v>12500</v></c></row></sheetData></worksheet>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rows, err := parsePriceImportXLSX(bytes.NewReader(file.Bytes()))
	if err != nil {
		t.Fatalf("parsePriceImportXLSX() = %v, want no error", err)
	}
	if len(rows) != 1 || rows[0].code != "CEM-001" || rows[0].price != "12500" {
		t.Fatalf("rows = %#v, want one parsed row", rows)
	}
}
