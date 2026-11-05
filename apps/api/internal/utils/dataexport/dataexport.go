// Package dataexport writes a table of records, each with optional child lines, as CSV or XML.
package dataexport

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// utf8BOM lets spreadsheet programs read the CSV as UTF-8 instead of the system code page.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Table is one exported entity: its records and, when it has them, the lines nested in each.
type Table struct {
	// Root and Record name the XML document element and each record's element.
	Root   string
	Record string
	// Attributes are written on the XML root element only.
	Attributes []Attribute
	Columns    []string
	// Children is nil for an entity without nested lines.
	Children *ChildTable
	Records  []Record
}

// Attribute is one name and value written on the XML root element.
type Attribute struct {
	Name  string
	Value string
}

// ChildTable names the lines nested in each record.
type ChildTable struct {
	// Group and Record name the XML wrapper and each line's element; Prefix starts each CSV header.
	Group   string
	Record  string
	Prefix  string
	Columns []string
}

// Record is one record's values, in column order, and its child lines.
type Record struct {
	Values   []string
	Children [][]string
}

// WriteCSV writes one row per child line, repeating the record's values on each; a record with no
// lines is one row with the child columns empty.
func WriteCSV(table Table) ([]byte, error) {
	if err := table.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(utf8BOM)
	writer := csv.NewWriter(&buf)
	writer.UseCRLF = true

	header := append([]string(nil), table.Columns...)
	var emptyChild []string
	if table.Children != nil {
		for _, column := range table.Children.Columns {
			header = append(header, table.Children.Prefix+column)
		}
		emptyChild = make([]string, len(table.Children.Columns))
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, record := range table.Records {
		lines := record.Children
		if table.Children == nil || len(lines) == 0 {
			lines = [][]string{emptyChild}
		}
		for _, line := range lines {
			row := make([]string, 0, len(record.Values)+len(line))
			for _, value := range append(append(row, record.Values...), line...) {
				row = append(row, neutralizeFormula(value))
			}
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteXML writes the records as elements under the root, each child line nested in its record.
func WriteXML(table Table) ([]byte, error) {
	if err := table.validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString("<" + table.Root)
	for _, attribute := range table.Attributes {
		buf.WriteString(" " + attribute.Name + `="`)
		if err := xml.EscapeText(&buf, []byte(attribute.Value)); err != nil {
			return nil, err
		}
		buf.WriteString(`"`)
	}
	buf.WriteString(">\n")
	for _, record := range table.Records {
		buf.WriteString("  <" + table.Record + ">\n")
		if err := writeFields(&buf, "    ", table.Columns, record.Values); err != nil {
			return nil, err
		}
		if table.Children != nil {
			if len(record.Children) == 0 {
				buf.WriteString("    <" + table.Children.Group + "/>\n")
			} else {
				buf.WriteString("    <" + table.Children.Group + ">\n")
				for _, child := range record.Children {
					buf.WriteString("      <" + table.Children.Record + ">\n")
					if err := writeFields(&buf, "        ", table.Children.Columns, child); err != nil {
						return nil, err
					}
					buf.WriteString("      </" + table.Children.Record + ">\n")
				}
				buf.WriteString("    </" + table.Children.Group + ">\n")
			}
		}
		buf.WriteString("  </" + table.Record + ">\n")
	}
	buf.WriteString("</" + table.Root + ">\n")
	return buf.Bytes(), nil
}

// neutralizeFormula prefixes a cell a spreadsheet would evaluate as a formula, since names and notes
// come from clients; a signed number such as a phone number is left as it is.
func neutralizeFormula(value string) string {
	if value == "" || !strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return value
	}
	if _, err := strconv.ParseFloat(strings.TrimPrefix(value, "+"), 64); err == nil {
		return value
	}
	return "'" + value
}

func writeFields(buf *bytes.Buffer, indent string, columns, values []string) error {
	for index, column := range columns {
		if values[index] == "" {
			buf.WriteString(indent + "<" + column + "/>\n")
			continue
		}
		buf.WriteString(indent + "<" + column + ">")
		if err := xml.EscapeText(buf, []byte(values[index])); err != nil {
			return err
		}
		buf.WriteString("</" + column + ">\n")
	}
	return nil
}

// validate refuses a record or line whose width differs from its columns, which would shift every
// value after it into the wrong column.
func (t Table) validate() error {
	for index, record := range t.Records {
		if len(record.Values) != len(t.Columns) {
			return fmt.Errorf("dataexport: record %d has %d values for %d columns",
				index, len(record.Values), len(t.Columns))
		}
		if len(record.Children) > 0 && t.Children == nil {
			return fmt.Errorf("dataexport: record %d has lines but the table declares none", index)
		}
		for childIndex, child := range record.Children {
			if len(child) != len(t.Children.Columns) {
				return fmt.Errorf("dataexport: record %d line %d has %d values for %d columns",
					index, childIndex, len(child), len(t.Children.Columns))
			}
		}
	}
	names := append(append([]string{t.Root, t.Record}, t.Columns...), t.childNames()...)
	for _, attribute := range t.Attributes {
		names = append(names, attribute.Name)
	}
	for _, name := range names {
		if !validName(name) {
			return fmt.Errorf("dataexport: %q is not a valid element name", name)
		}
	}
	return nil
}

func (t Table) childNames() []string {
	if t.Children == nil {
		return nil
	}
	return append([]string{t.Children.Group, t.Children.Record}, t.Children.Columns...)
}

// validName keeps element names to lowercase ASCII words joined by underscores.
func validName(name string) bool {
	if name == "" || strings.Trim(name, "_") != name {
		return false
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_') {
			return false
		}
	}
	return name[0] < '0' || name[0] > '9'
}
