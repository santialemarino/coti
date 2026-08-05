package spreadsheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestRead_ReturnsTheSameMappedRowsForCSVAndXLSX(t *testing.T) {
	t.Parallel()
	schema := Schema{Columns: []Column{
		{Key: "sku", Headers: []string{"código", "sku"}, Required: true},
		{Key: "stock", Headers: []string{"cantidad", "stock"}, Required: true},
	}}
	xlsx, err := Write(Workbook{Sheets: []Sheet{{
		Name: "Stock",
		Rows: []ExportRow{
			{Number: 1, Values: []string{"sku", "stock"}, Header: true},
			{Number: 4, Values: []string{"ARE-001", "25"}},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		filename string
		content  []byte
	}{
		{name: "csv", filename: "stock.csv", content: []byte("código;cantidad\n\n\nARE-001;25\n")},
		{name: "xlsx", filename: "stock.xlsx", content: xlsx},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rows, readErr := Read(test.filename, bytes.NewReader(test.content), schema)
			if readErr != nil {
				t.Fatalf("Read() = %v, want no error", readErr)
			}
			if len(rows) != 1 || rows[0].Number != 4 {
				t.Fatalf("rows = %#v, want one row at source line 4", rows)
			}
			if rows[0].Values["sku"] != "ARE-001" || rows[0].Values["stock"] != "25" {
				t.Errorf("values = %#v, want the caller-defined mapping", rows[0].Values)
			}
		})
	}
}

func TestWrite_RendersDeclarativeSheetsAndValidation(t *testing.T) {
	t.Parallel()
	content, err := Write(Workbook{
		Sheets: []Sheet{
			{
				Name: "Datos", FreezeHeader: true, AutoFilter: true,
				Rows: []ExportRow{{Values: []string{"codigo"}, Header: true}},
				DataValidations: []DataValidation{{
					Range: "A2:A100", Formula: "Codigos", ErrorTitle: "Código inválido",
					ErrorMessage: "Elegí un código del listado.",
				}},
			},
			{Name: "Instrucciones", Rows: []ExportRow{{Values: []string{"Completá la hoja Datos."}}}},
			{Name: "Listas", Hidden: true, Rows: []ExportRow{{Values: []string{"ARE-001"}}}},
		},
		DefinedNames: []DefinedName{{Name: "Codigos", Formula: "Listas!$A$1:$A$1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]string, len(archive.File))
	for _, entry := range archive.File {
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.HasSuffix(entry.Name, ".xml") || strings.HasSuffix(entry.Name, ".rels") {
			decoder := xml.NewDecoder(bytes.NewReader(data))
			for {
				if _, decodeErr := decoder.Token(); decodeErr == io.EOF {
					break
				} else if decodeErr != nil {
					t.Fatalf("%s is not valid XML: %v", entry.Name, decodeErr)
				}
			}
		}
		entries[entry.Name] = string(data)
	}
	workbook := entries["xl/workbook.xml"]
	dataSheet := entries["xl/worksheets/sheet1.xml"]
	if !strings.Contains(workbook, `name="Listas" sheetId="3" state="hidden"`) {
		t.Error("workbook does not preserve the caller-defined hidden sheet")
	}
	if !strings.Contains(workbook, `<definedName name="Codigos">Listas!$A$1:$A$1</definedName>`) {
		t.Error("workbook does not preserve the caller-defined name")
	}
	if !strings.Contains(dataSheet, `<formula1>Codigos</formula1>`) ||
		!strings.Contains(dataSheet, `<autoFilter ref="A1:A1"/>`) {
		t.Error("data sheet does not preserve validation and filtering")
	}
}

func TestRead_RejectsMissingRequiredCallerColumn(t *testing.T) {
	t.Parallel()
	schema := Schema{Columns: []Column{{Key: "code", Headers: []string{"codigo"}, Required: true}}}
	_, err := Read("third-import.csv", strings.NewReader("nombre\nArena"), schema)
	if err == nil || !strings.Contains(err.Error(), "codigo") {
		t.Fatalf("Read() = %v, want a missing-column error", err)
	}
}
