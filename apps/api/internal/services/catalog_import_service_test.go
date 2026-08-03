package services

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type catalogImportTestDB struct{}

func (catalogImportTestDB) InTenantTx(
	_ context.Context, _ domain.Tenant, fn func(repository.Querier) error,
) error {
	return fn(nil)
}

type catalogImportTestRepository struct {
	existing map[string]struct{}
	applied  []domain.CatalogImportRow
}

func (r *catalogImportTestRepository) ListExistingCodes(
	_ context.Context, _ repository.Querier, _ uuid.UUID, _ []string,
) (map[string]struct{}, error) {
	return r.existing, nil
}

func (r *catalogImportTestRepository) ApplyImport(
	_ context.Context, _ repository.Querier, _ domain.Tenant, _ time.Time,
	rows []domain.CatalogImportRow,
) error {
	r.applied = rows
	return nil
}

func TestCatalogImportService_Preview_ReportsInvalidRowsWithoutBlockingValidOnes(t *testing.T) {
	t.Parallel()
	repo := &catalogImportTestRepository{existing: map[string]struct{}{"EXISTE": {}}}
	service := NewCatalogImportService(catalogImportTestDB{}, repo, func() time.Time {
		return time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	})
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	csvFile := "codigo;descripcion;unidad;precio\n" +
		"EXISTE;Cemento;bolsa;10000\n" +
		"DUP;Arena;m3;5000\n" +
		"DUP;Arena fina;m3;5500\n" +
		"OK;Piedra partida;m3;$ 12.500,50\n" +
		"MAL;Cal;;abc"

	preview, err := service.Preview(
		context.Background(), tenant, "catalogo.csv", strings.NewReader(csvFile),
	)
	if err != nil {
		t.Fatalf("Preview() = %v, want no error", err)
	}
	if !preview.CanConfirm {
		t.Error("CanConfirm = false, want true while at least one row is valid")
	}
	if preview.ValidRows != 1 || preview.InvalidRows != 4 {
		t.Fatalf("valid, invalid = %d, %d; want 1, 4", preview.ValidRows, preview.InvalidRows)
	}
	valid := preview.Rows[3]
	if valid.Name != "Piedra partida" || valid.Price != "12500.50" || len(valid.Errors) != 0 {
		t.Errorf("valid row = %#v, want fallback name and normalized price", valid)
	}
	if !containsError(preview.Rows[0].Errors, "existing_code") {
		t.Errorf("existing row errors = %v, want existing_code", preview.Rows[0].Errors)
	}
	if !containsError(preview.Rows[1].Errors, "duplicate_code") ||
		!containsError(preview.Rows[2].Errors, "duplicate_code") {
		t.Errorf("duplicate row errors = %v, %v", preview.Rows[1].Errors, preview.Rows[2].Errors)
	}
}

func TestCatalogImportService_Confirm_SkipsInvalidRowsAndAppliesValidRows(t *testing.T) {
	t.Parallel()
	repo := &catalogImportTestRepository{existing: map[string]struct{}{"EXISTE": {}}}
	service := NewCatalogImportService(catalogImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}

	result, err := service.Confirm(context.Background(), tenant, []domain.CatalogImportInput{
		{Code: "NUEVO", Description: "Cemento", Unit: "bolsa", Price: "10000", Currency: "ars"},
		{Code: "EXISTE", Description: "Arena", Unit: "m3", Price: "5000"},
		{Code: "MAL", Description: "Cal", Price: "abc"},
	})
	if err != nil {
		t.Fatalf("Confirm() = %v, want no error", err)
	}
	if result.ImportedRows != 1 || result.SkippedRows != 2 {
		t.Fatalf("result = %#v, want 1 imported and 2 skipped", result)
	}
	if len(repo.applied) != 1 || repo.applied[0].Code != "NUEVO" {
		t.Fatalf("applied = %#v, want only NUEVO", repo.applied)
	}
	if repo.applied[0].Name != "Cemento" || repo.applied[0].Currency != "ARS" {
		t.Errorf("applied row = %#v, want normalized defaults", repo.applied[0])
	}
}

func TestCatalogImportService_Template_IsSpanishAndCarriesInstructions(t *testing.T) {
	t.Parallel()
	service := NewCatalogImportService(catalogImportTestDB{}, &catalogImportTestRepository{}, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}

	file, err := service.Template(context.Background(), tenant)
	if err != nil {
		t.Fatalf("Template() = %v, want no error", err)
	}
	if file.Filename != "catalogo-inicial.xlsx" {
		t.Errorf("Filename = %q, want catalogo-inicial.xlsx", file.Filename)
	}
	records, err := readImportXLSXRecords(bytes.NewReader(file.Content))
	if err != nil {
		t.Fatalf("read template = %v, want no error", err)
	}
	if len(records) != 1 || strings.Join(records[0], ",") != strings.Join(catalogImportHeaders, ",") {
		t.Fatalf("headers = %#v, want %#v", records, catalogImportHeaders)
	}
	archive, err := zip.NewReader(bytes.NewReader(file.Content), int64(len(file.Content)))
	if err != nil {
		t.Fatal(err)
	}
	var instructions, workbook string
	for _, entry := range archive.File {
		switch entry.Name {
		case "xl/workbook.xml":
			content, readErr := readZIPFile(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			workbook = string(content)
		case "xl/worksheets/sheet2.xml":
			content, readErr := readZIPFile(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			instructions = string(content)
		}
	}
	if !strings.Contains(workbook, `name="Catálogo"`) || !strings.Contains(workbook, `name="Instrucciones"`) {
		t.Errorf("workbook does not carry the Spanish catalog and instructions sheet names")
	}
	if !strings.Contains(instructions, "Las columnas codigo, descripcion, unidad y precio son obligatorias.") {
		t.Errorf("instructions sheet does not explain the required Spanish columns")
	}
}

func containsError(errors []string, expected string) bool {
	for _, current := range errors {
		if current == expected {
			return true
		}
	}
	return false
}
