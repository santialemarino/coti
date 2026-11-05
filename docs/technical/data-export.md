# Data export

The backoffice downloads the account's records for another system — a migration, an ERP, an
accountant — at `/settings/export`. It is on demand, never a sync, and needs an `ADMIN` user.

## Endpoint

`GET /v1/exports/{entity}?format=csv|xml` with `entity` one of `quotes`, `catalog`, `clients` or
`invoices`. Optional parameters:

| Parameter   | Applies to                | Meaning                                                           |
| ----------- | ------------------------- | ----------------------------------------------------------------- |
| `branch_id` | quotes, catalog, invoices | Only that branch's quotes, prices or invoices. Clients ignore it. |
| `from`      | quotes, invoices          | First day, `YYYY-MM-DD` in Argentina, inclusive.                  |
| `to`        | quotes, invoices          | Last day, inclusive.                                              |

Quotes are filtered by creation, invoices by issue date. An unknown entity, format, malformed date,
reversed period or branch outside the account is `422 INVALID_INPUT`; a seller gets `403`. The read
runs in the caller's tenant transaction, so row level security keeps it inside the account even
before the explicit `account_id` predicates. The file is named after the entity in Spanish, the branch and
the day (`cotizaciones-centro-2026-11-05.csv`), sent with `Cache-Control: private, no-store`.

## What each entity carries

| Entity   | One record per                     | Nested lines                                          |
| -------- | ---------------------------------- | ----------------------------------------------------- |
| quotes   | quote, with its current version    | `items`: the version's lines, unmatched ones included |
| catalog  | product                            | `sucursales`: availability, stock and current price   |
| clients  | client not merged into another     | `contactos`: contacts that have not been retired      |
| invoices | invoice or credit note, any status | none                                                  |

A quote's `descuentos` is its lines' subtotal minus its total, which is how the version total is
defined. An invoice's `entorno` says whether it was authorized in production or in ARCA's test
environment, which has no fiscal validity.

## Format

Field names are Spanish snake_case, like the catalog and price spreadsheets. Values follow the
API's rules: money and quantities are decimal strings with a dot (`1234.50`), timestamps are
RFC 3339 in Argentina (`2026-11-05T10:00:00-03:00`), dates are `YYYY-MM-DD`, booleans are `SI` /
`NO`, and enums carry the backoffice's Spanish label (`Aceptada`, `Responsable inscripto`).

- **CSV** is UTF-8 with a byte order mark (so Excel reads accents), comma-separated, CRLF line
  ends. A record with lines is one row per line, repeating the record's columns; a record without
  lines is one row with the line columns empty. Line columns carry a prefix (`item_`,
  `sucursal_`, `contacto_`). A cell a spreadsheet would evaluate as a formula (`=`, `+`, `-`, `@`)
  is prefixed with `'`, except a plain signed number such as a phone number.
- **XML** has one root (`cotizaciones`, `catalogo`, `clientes`, `comprobantes`) carrying
  `exportado_el` and the filters applied (`sucursal`, `desde`, `hasta`) as attributes, one
  element per record, and the lines nested under their group element. An empty value is an empty
  element, so every record has the same shape.

The writer is `internal/utils/dataexport`, which knows nothing about the entities; the service
builds each entity's table in `internal/services/data_export_tables.go`.
