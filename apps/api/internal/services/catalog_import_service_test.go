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

func (r *catalogImportTestRepository) ListTaxonomy(
	_ context.Context, _ repository.Querier,
) ([]domain.ProductFamily, error) {
	return []domain.ProductFamily{
		{
			ID: uuid.MustParse("f0000000-0000-4000-8000-000000000001"), Name: "MATERIALES DE CONSTRUCCION",
			Subgroups: []domain.ProductSubgroup{{
				ID:   uuid.MustParse("e0000000-0000-4000-8000-000000000001"),
				Name: "ARIDOS",
			}},
		},
		{ID: uuid.MustParse("f0000000-0000-4000-8000-000000000002"), Name: "TANQUES"},
	}, nil
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
	csvFile := "codigo;nombre;descripcion;unidad;familia;subgrupo;precio\n" +
		"EXISTE;Cemento;;bolsa;MATERIALES DE CONSTRUCCION;;10000\n" +
		"DUP;Arena;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;5000\n" +
		"DUP;Arena fina;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;5500\n" +
		"OK;Piedra partida;Granítica;m3;MATERIALES DE CONSTRUCCION;ARIDOS;$ 12.500,50\n" +
		"MAL;Cal;;;MATERIALES DE CONSTRUCCION;;abc"

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
		t.Errorf("valid row = %#v, want canonical name and normalized price", valid)
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
		{Code: "NUEVO", Name: "Cemento", Unit: "bolsa", Family: "MATERIALES DE CONSTRUCCION", Price: "10000"},
		{Code: "EXISTE", Name: "Arena", Unit: "m3", Family: "MATERIALES DE CONSTRUCCION", Price: "5000"},
		{Code: "MAL", Name: "Cal", Family: "MATERIALES DE CONSTRUCCION", Price: "abc"},
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
	if repo.applied[0].Name != "Cemento" || repo.applied[0].Family != "MATERIALES DE CONSTRUCCION" {
		t.Errorf("applied row = %#v, want normalized defaults", repo.applied[0])
	}
}

func TestCatalogImportService_Preview_RejectsSubgroupFromAnotherFamily(t *testing.T) {
	t.Parallel()
	service := NewCatalogImportService(catalogImportTestDB{}, &catalogImportTestRepository{}, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	csvFile := "codigo;nombre;unidad;familia;subgrupo;precio\n" +
		"ARE-1;Arena;m3;MATERIALES DE CONSTRUCCION;NO PERTENECE;5000"

	preview, err := service.Preview(
		context.Background(), tenant, "catalogo.csv", strings.NewReader(csvFile),
	)
	if err != nil {
		t.Fatalf("Preview() = %v, want no error", err)
	}
	if preview.ValidRows != 0 || !containsError(preview.Rows[0].Errors, "invalid_subgroup") {
		t.Fatalf("preview = %#v, want invalid_subgroup", preview)
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
	archive, err := zip.NewReader(bytes.NewReader(file.Content), int64(len(file.Content)))
	if err != nil {
		t.Fatal(err)
	}
	var catalog, instructions, lists, workbook string
	for _, entry := range archive.File {
		switch entry.Name {
		case "xl/workbook.xml":
			content, readErr := readTestZIPEntry(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			workbook = string(content)
		case "xl/worksheets/sheet1.xml":
			content, readErr := readTestZIPEntry(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			catalog = string(content)
		case "xl/worksheets/sheet2.xml":
			content, readErr := readTestZIPEntry(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			instructions = string(content)
		case "xl/worksheets/sheet3.xml":
			content, readErr := readTestZIPEntry(entry)
			if readErr != nil {
				t.Fatal(readErr)
			}
			lists = string(content)
		}
	}
	if !strings.Contains(workbook, `name="Catálogo"`) || !strings.Contains(workbook, `name="Instrucciones"`) {
		t.Errorf("workbook does not carry the Spanish catalog and instructions sheet names")
	}
	for _, header := range catalogImportHeaders {
		if !strings.Contains(catalog, ">"+header+"</t>") {
			t.Errorf("catalog sheet does not carry header %q", header)
		}
	}
	if !strings.Contains(workbook, `name="Listas" sheetId="3" state="hidden"`) ||
		strings.Contains(workbook, "MapaFamilias") ||
		strings.Contains(workbook, `<definedName name="SG1">`) ||
		!strings.Contains(workbook, `<definedName name="Subgrupos">Listas!$B$2:$B$2</definedName>`) ||
		!strings.Contains(catalog, `<formula1>Familias</formula1>`) ||
		!strings.Contains(catalog, `<formula1>Subgrupos</formula1>`) ||
		strings.Contains(catalog, "INDIRECT") {
		t.Error("workbook does not carry the hidden database-backed family and subgroup validations")
	}
	if !strings.Contains(lists, "MATERIALES DE CONSTRUCCION") || !strings.Contains(lists, "ARIDOS") {
		t.Error("hidden lists sheet does not carry the configured taxonomy")
	}
	if !strings.Contains(instructions, "Las columnas codigo, nombre, unidad, familia y precio son obligatorias.") {
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

func readTestZIPEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
