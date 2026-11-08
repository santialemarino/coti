package dataexport

import (
	"strings"
	"testing"
)

func quoteTable() Table {
	return Table{
		Root: "cotizaciones", Record: "cotizacion",
		Attributes: []Attribute{{Name: "exportado_el", Value: "2026-11-05T10:00:00-03:00"}},
		Columns:    []string{"numero", "cliente"},
		Children: &ChildTable{
			Group: "items", Record: "item", Prefix: "item_",
			Columns: []string{"descripcion", "cantidad"},
		},
		Records: []Record{
			{Values: []string{"1", "Obra, \"Norte\""}, Children: [][]string{{"Cemento", "10.00"}, {"Arena", "2.50"}}},
			{Values: []string{"2", ""}},
		},
	}
}

func TestWriteCSV_RepeatsTheRecordOnEachLineAndKeepsALinelessRecord(t *testing.T) {
	content, err := WriteCSV(quoteTable())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "\xEF\xBB\xBF") {
		t.Fatal("CSV does not start with the UTF-8 byte order mark")
	}
	want := "numero,cliente,item_descripcion,item_cantidad\r\n" +
		"1,\"Obra, \"\"Norte\"\"\",Cemento,10.00\r\n" +
		"1,\"Obra, \"\"Norte\"\"\",Arena,2.50\r\n" +
		"2,,,\r\n"
	if got := strings.TrimPrefix(string(content), "\xEF\xBB\xBF"); got != want {
		t.Fatalf("CSV =\n%q\nwant\n%q", got, want)
	}
}

func TestWriteCSV_NeutralizesFormulasButNotSignedNumbers(t *testing.T) {
	table := Table{Root: "clientes", Record: "cliente", Columns: []string{"nombre", "telefono", "nota"},
		Records: []Record{{Values: []string{"=HYPERLINK(\"x\")", "+5491122334455", "-3"}}}}
	content, err := WriteCSV(table)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "\"'=HYPERLINK(\"\"x\"\")\",+5491122334455,-3\r\n") {
		t.Fatalf("CSV = %q", content)
	}
}

func TestWriteXML_NestsLinesEscapesValuesAndWritesEmptyFields(t *testing.T) {
	table := quoteTable()
	table.Records[1].Values[1] = "A & B <C>"
	content, err := WriteXML(table)
	if err != nil {
		t.Fatal(err)
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<cotizaciones exportado_el="2026-11-05T10:00:00-03:00">
  <cotizacion>
    <numero>1</numero>
    <cliente>Obra, &#34;Norte&#34;</cliente>
    <items>
      <item>
        <descripcion>Cemento</descripcion>
        <cantidad>10.00</cantidad>
      </item>
      <item>
        <descripcion>Arena</descripcion>
        <cantidad>2.50</cantidad>
      </item>
    </items>
  </cotizacion>
  <cotizacion>
    <numero>2</numero>
    <cliente>A &amp; B &lt;C&gt;</cliente>
    <items/>
  </cotizacion>
</cotizaciones>
`
	if string(content) != want {
		t.Fatalf("XML =\n%s\nwant\n%s", content, want)
	}
}

func TestWrite_RefusesAMisalignedRecordAndAnInvalidName(t *testing.T) {
	misaligned := quoteTable()
	misaligned.Records[0].Children[1] = []string{"Arena"}
	if _, err := WriteCSV(misaligned); err == nil {
		t.Fatal("WriteCSV accepted a line with a missing value")
	}
	named := quoteTable()
	named.Columns[0] = "número"
	if _, err := WriteXML(named); err == nil {
		t.Fatal("WriteXML accepted a non-ASCII element name")
	}
}
