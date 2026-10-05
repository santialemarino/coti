# Electronic invoicing (ARCA)

An accepted quote can be invoiced to ARCA, Argentina's tax agency, from the order screen. A
person always confirms: an authorized invoice cannot be withdrawn, only offset by a credit note,
and credit notes are not built yet.

## The only route ARCA offers

For A, B and C invoices the programmatic route is the **WSAA + WSFEv1** web services (RG 4291).
There is no file ARCA accepts to _issue_ them; its file imports only report invoices already
issued. Coti talks to WSFEv1 directly through the `InvoiceIssuer` port, implemented in
`internal/arca`:

- **WSAA**: a login request is signed as CMS (SHA-256) with the account's certificate and
  exchanged for a token and sign valid about 12 hours, cached in memory per certificate.
- **WSFEv1**: `FECompUltimoAutorizado` gives the last number, `FECAESolicitar` asks for the next
  one, and `FECompConsultar` reads one back. Error 10016 (the number was taken in between) is
  retried once.

Each account uploads **its own certificate and private key** in Settings → Facturación. The key is
sealed with `ARCA_CREDENTIALS_ENCRYPTION_KEY` before it is stored (`arca_credential`), and the
certificate's CUIT must be the account's. The account also needs its IVA condition, and each
branch an ARCA point of sale enabled for web services.

## The amounts

Money never comes from a model. `invoice_helpers.go` computes it from the frozen quote version:

1. Each line's subtotal, less its share of the version's discounts — item discounts over the lines
   they name, total discounts over every line, in proportion, the last line taking the rounding.
2. Added up per IVA rate (`product.vat_rate`, 21 % by default; a line with no product is 21 %).
3. Split into base and IVA per rate: divided out when the account's prices include IVA
   (`account.prices_include_vat`), added on top when they do not. Banker's rounding, as ARCA uses.

The letter follows both parties' IVA condition: a registered issuer bills **A** to a registered or
monotributo buyer and **B** to everyone else; a monotributo or exempt issuer always bills **C**,
which breaks no IVA out. A buyer with no fiscal data is a consumidor final; from
`INVOICE_UNIDENTIFIED_RECEIVER_MAX_AMOUNT` (RG 5700/2025) a B invoice must identify them.

## Never two invoices for one sale

The invoice is written as `PENDING` before ARCA is asked, and a partial unique index allows one
live (pending or issued) invoice per quote version. ARCA's answer then settles it:

| Outcome                                             | What happens                                                                        |
| --------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Authorized                                          | `ISSUED`, with number, CAE and its expiry                                           |
| Refused by ARCA (a WSAA certificate fault included) | `REJECTED`, ARCA's reasons shown verbatim; the seller can retry                     |
| ARCA unreachable, nothing authorized                | Released (`REJECTED`, `ARCA_UNAVAILABLE`), not shown as a refusal; retrying is safe |
| The request may have landed, unconfirmed            | Stays `PENDING`; the quote answers `INVOICE_IN_PROGRESS`                            |

A `PENDING` invoice older than ten request timeouts is reconciled on the next try: ARCA's newest
invoice at that point of sale is adopted if it has the same buyer and total and is not older;
otherwise the hold is released and a fresh one is asked for.

## What the invoice shows

The order screen shows the number (`A 0003-00000023`), CAE and its expiry, and ARCA's
verification link — the QR payload of RG 4892, built by `domain.InvoiceQRURL`. A printable invoice
PDF is not built yet.

## Settings

| Variable                                   | Default        | Meaning                                                |
| ------------------------------------------ | -------------- | ------------------------------------------------------ |
| `INVOICING_PROVIDER`                       | `disabled`     | `wsfe` turns invoicing on                              |
| `ARCA_ENVIRONMENT`                         | `homologation` | ARCA's test service; `production` issues real invoices |
| `ARCA_REQUEST_TIMEOUT_SECONDS`             | `30`           | Per call to WSAA or WSFEv1                             |
| `INVOICE_UNIDENTIFIED_RECEIVER_MAX_AMOUNT` | `10000000`     | Pesos from which a B invoice must identify its buyer   |
| `ARCA_CREDENTIALS_ENCRYPTION_KEY`          | —              | Seals the private keys; required when invoicing is on  |

## Not built yet

- Credit notes, and with them any correction of an issued invoice.
- A printable invoice PDF with the QR image and, for B to a consumidor final, the Ley 27.743 block.
- Factura de Crédito Electrónica MiPyME, for sales from ~ARS 5.5 million to a "Gran Empresa".
- A login ticket that survives a restart: WSAA refuses a second login while one is live, so after
  a restart invoicing waits until the previous ticket expires (up to 12 hours).
- The IVA rate in the catalog spreadsheet import and export.
