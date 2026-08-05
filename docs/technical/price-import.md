# Price import

The backoffice updates branch prices from a spreadsheet at `/settings/prices`. The operation
is always scoped to one branch and needs an `ADMIN` user.

## Flow

1. The screen works on the shell's active branch, chosen once in the header switcher, and
   downloads its prices in force with **Exportar precios**. `GET /v1/product-prices/export`
   returns an `.xlsx` with a pre-filled `Precios` sheet and an `Instrucciones` sheet.
2. The administrator edits that file — or prepares a compatible `.xlsx` or `.csv` — and
   uploads it on the same screen.
3. `POST /v1/product-prices/import/preview` parses the file and returns every row with the
   product it matched, the price in force, the proposed value and any validation errors. It
   writes nothing.
4. The screen allows confirming when at least one row is valid and explains that invalid rows will
   be skipped.
5. `POST /v1/product-prices/import/confirm` revalidates the content and, in the same transaction,
   closes the current price periods and inserts replacements for every valid row.

Existing quotes are untouched. Every `quote_item` keeps the price and minimum-price snapshots
taken when its version was built; repricing a quote is a separate, explicit action.

## File format

The first sheet of an `.xlsx`, or the body of a `.csv`, needs these columns:

| Column          | Required | Description                                              |
| --------------- | -------- | -------------------------------------------------------- |
| `codigo`        | yes      | Product code, unique within the account.                 |
| `precio`        | yes      | Sale price above zero, two decimals.                     |
| `precio_minimo` | no       | The discount engine's floor; never above the sale price. |

The exported file adds an informational `producto` column. The import ignores it and matches
each row by `codigo` alone. The export carries only active products that already have a price
in force for the branch, so dropping a row before importing is how you leave a product alone.

Currency is not a spreadsheet field. A replacement price preserves the product's current
currency; the first price for a product defaults to `ARS`.

The equivalent English headers are accepted too: `code`, `price` and `min_price`. CSV files may
use a comma or a semicolon as the separator.

Rows with duplicate codes, unknown or inactive products, invalid amounts, or a minimum above the
sale price are reported and skipped. They do not block valid rows in the same file.

## Configuration

`PRICE_IMPORT_MAX_BYTES` caps the upload in bytes, defaulting to `5242880` (5 MiB). That caps
the compressed size only, so each XLSX entry is additionally bounded when it is decompressed.

CSV/XLSX parsing, row mapping, workbook generation, and the upload boundary use the shared
spreadsheet contract described in [Shared spreadsheet layer](spreadsheets.md).

## Supporting endpoints

`GET /v1/branches` returns the active branches the user can reach and feeds the header
switcher. The branch travels in `X-Branch-Id`, where the middleware revalidates access before
the import runs — and it is named **explicitly** on all three calls rather than inherited from
the active branch, so a preview confirmed after a switch still writes where it was prepared. An
administrator reaching several branches who has selected none is asked to pick one; the screen
offers nothing to run until they do, because the API refuses a price operation with no branch.
