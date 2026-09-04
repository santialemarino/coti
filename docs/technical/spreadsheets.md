# Shared spreadsheet layer

Spreadsheet imports and exports use `internal/utils/spreadsheet` instead of implementing CSV,
XLSX, ZIP, or worksheet XML handling inside a product feature. The package exposes two
declarative contracts.

## Imports

An import declares a `spreadsheet.Schema`. Each column has a logical key, accepted header aliases,
and a required flag. `spreadsheet.Read` then:

1. selects the CSV or XLSX reader from the filename;
2. reads the first XLSX worksheet or the CSV body into the same internal representation;
3. normalizes headers and maps them to the schema's logical keys;
4. skips empty rows while preserving the source row number; and
5. returns `[]spreadsheet.Row`, where every row contains its number and a key/value map.

The feature maps those logical values into its raw input type and owns all domain validation. For
example, price imports declare `code`, `price`, and `min_price`; catalog imports add their product
and taxonomy columns. A future import adds another schema and mapper without another CSV or XLSX
parser.

CSV accepts comma and semicolon separators. XLSX supports shared strings, inline strings, numeric
cells, and a workbook whose first worksheet is not stored as `sheet1.xml`. Each decompressed XLSX
entry is bounded to protect the API from compressed expansion.

## Exports

An export declares a `spreadsheet.Workbook` made of sheets, rows, column widths, filters, list
validations, and workbook-defined names. `spreadsheet.Write` owns the ZIP package, relationships,
styles, cell escaping, and worksheet XML.

Feature code supplies only the workbook content. Price export declares `Precios` and
`Instrucciones`; catalog export also declares its hidden `Listas` sheet and validation ranges. A
future export can add sheets or validations through the same contract.

## Reviewed flow

The HTTP handlers use one upload helper, so every spreadsheet preview applies its configured
multipart limit and returns the same `413` error shape. Preview parses and validates without
writing. Confirmation rebuilds and revalidates the reviewed rows inside the same tenant-scoped
transaction that persists the valid rows. Invalid rows remain visible in preview and do not block
valid rows from being confirmed.

The upload limits remain feature-specific because their operational needs may differ:
`CATALOG_IMPORT_MAX_BYTES` and `PRICE_IMPORT_MAX_BYTES`.
