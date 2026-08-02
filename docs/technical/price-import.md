# Price import

The backoffice updates branch prices from a spreadsheet at `/settings/prices`. The operation
is always scoped to one branch and needs an `ADMIN` user.

## Flow

1. The administrator picks the branch and downloads its prices in force with **Exportar
   precios**. `GET /v1/product-prices/export` returns an `.xlsx` with a pre-filled `Precios`
   sheet and an `Instrucciones` sheet.
2. The administrator edits that file — or prepares a compatible `.xlsx` or `.csv` — and
   uploads it on the same screen.
3. `POST /v1/product-prices/import/preview` parses the file and returns every row with the
   product it matched, the price in force, the proposed value and any validation errors. It
   writes nothing.
4. The screen only allows confirming when every row is valid.
5. `POST /v1/product-prices/import/confirm` revalidates the content and, in one transaction,
   closes the current price periods and inserts the new `product_price` rows.

Existing quotes are untouched. Every `quote_item` keeps the price and minimum-price snapshots
taken when its version was built; repricing a quote is a separate, explicit action.

## File format

The first sheet of an `.xlsx`, or the body of a `.csv`, needs these columns:

| Column          | Required | Description                                              |
| --------------- | -------- | -------------------------------------------------------- |
| `codigo`        | yes      | Product code, unique within the account.                 |
| `precio`        | yes      | Sale price above zero, two decimals.                     |
| `precio_minimo` | no       | The discount engine's floor; never above the sale price. |
| `moneda`        | no       | Three-letter ISO code; defaults to `ARS`.                |
| `condiciones`   | no       | Free-text condition, up to 255 characters.               |

The exported file adds an informational `producto` column. The import ignores it and matches
each row by `codigo` alone. The export carries only active products that already have a price
in force for the branch, so dropping a row before importing is how you leave a product alone.

The equivalent English headers are accepted too: `code`, `price`, `min_price`, `currency` and
`conditions`. CSV files may use a comma or a semicolon as the separator.

The whole file is refused for confirmation when it carries duplicate codes, unknown or
inactive products, invalid amounts, a minimum above the sale price, an invalid currency, or
conditions that are too long.

## Configuration

`PRICE_IMPORT_MAX_BYTES` caps the upload in bytes, defaulting to `5242880` (5 MiB). That caps
the compressed size only, so each XLSX entry is additionally bounded when it is decompressed.

## Supporting endpoints

`GET /v1/branches` returns the active branches the user can reach and feeds the backoffice
selector. The chosen branch travels in `X-Branch-Id`, where the middleware revalidates access
before the import runs.
