package services

import (
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/utils/spreadsheet"
)

var catalogImportHeaders = []string{
	"codigo", "nombre", "descripcion", "unidad", "familia", "subgrupo", "precio",
	"precio_minimo",
}

var catalogImportInstructions = []string{
	"Completá la hoja Catálogo y volvé a importar este archivo desde el backoffice.",
	"Las columnas codigo, nombre, unidad, familia y precio son obligatorias.",
	"Elegí la familia y, si corresponde, el subgrupo de sus desplegables. El subgrupo debe pertenecer a la familia.",
	"Cada código debe ser único dentro del archivo y no puede existir previamente en el catálogo de la cuenta.",
	"El precio y el precio mínimo deben ser mayores a cero y admitir hasta 2 decimales.",
	"El precio mínimo no puede superar el precio de venta.",
	"Los precios iniciales se guardan en ARS.",
	"Las filas con errores se muestran en la vista previa y se omiten al confirmar.",
	"Los productos confirmados quedan activos y disponibles en la sucursal seleccionada.",
}

func buildCatalogImportXLSX(families []domain.ProductFamily) ([]byte, error) {
	subgroups := catalogSubgroupNames(families)
	familyLastRow := len(families) + 1
	if familyLastRow < 2 {
		familyLastRow = 2
	}
	subgroupLastRow := len(subgroups) + 1
	if subgroupLastRow < 2 {
		subgroupLastRow = 2
	}
	return spreadsheet.Write(spreadsheet.Workbook{
		Sheets: []spreadsheet.Sheet{
			{
				Name: "Catálogo", FreezeHeader: true, AutoFilter: true,
				Rows: []spreadsheet.ExportRow{{Number: 1, Values: catalogImportHeaders, Header: true}},
				ColumnWidths: []spreadsheet.ColumnWidth{
					{Min: 1, Max: 1, Width: 20}, {Min: 2, Max: 3, Width: 38},
					{Min: 4, Max: 4, Width: 18}, {Min: 5, Max: 6, Width: 34},
					{Min: 7, Max: 8, Width: 18},
				},
				DataValidations: []spreadsheet.DataValidation{
					{
						Range: "E2:E10000", Formula: "Familias", ErrorTitle: "Familia inválida",
						ErrorMessage: "Elegí una familia del listado.",
					},
					{
						Range: "F2:F10000", Formula: "Subgrupos", AllowBlank: true,
						ErrorTitle:   "Subgrupo inválido",
						ErrorMessage: "Elegí un subgrupo del listado que pertenezca a la familia seleccionada.",
					},
				},
			},
			{
				Name: "Instrucciones", Rows: catalogInstructionRows(),
				ColumnWidths: []spreadsheet.ColumnWidth{{Min: 1, Max: 1, Width: 110}},
			},
			{Name: "Listas", Hidden: true, Rows: catalogListRows(families, subgroups)},
		},
		DefinedNames: []spreadsheet.DefinedName{
			{Name: "Familias", Formula: fmt.Sprintf("Listas!$A$2:$A$%d", familyLastRow)},
			{Name: "Subgrupos", Formula: fmt.Sprintf("Listas!$B$2:$B$%d", subgroupLastRow)},
		},
	})
}

func catalogInstructionRows() []spreadsheet.ExportRow {
	rows := []spreadsheet.ExportRow{{Number: 1, Values: []string{"Carga inicial del catálogo"}, Header: true}}
	for index, instruction := range catalogImportInstructions {
		rows = append(rows, spreadsheet.ExportRow{Number: index + 3, Values: []string{instruction}})
	}
	return rows
}

func catalogListRows(families []domain.ProductFamily, subgroups []string) []spreadsheet.ExportRow {
	rowCount := len(families)
	if len(subgroups) > rowCount {
		rowCount = len(subgroups)
	}
	rows := []spreadsheet.ExportRow{{Number: 1, Values: []string{"Familia", "Subgrupo"}, Header: true}}
	for index := 0; index < rowCount; index++ {
		values := make([]string, 2)
		if index < len(families) {
			values[0] = families[index].Name
		}
		if index < len(subgroups) {
			values[1] = subgroups[index]
		}
		rows = append(rows, spreadsheet.ExportRow{Number: index + 2, Values: values})
	}
	return rows
}

func catalogSubgroupNames(families []domain.ProductFamily) []string {
	var subgroups []string
	for _, family := range families {
		for _, subgroup := range family.Subgroups {
			subgroups = append(subgroups, subgroup.Name)
		}
	}
	return subgroups
}
