# Client profiles and quotation delivery

## Association at publication

`GET /v1/quotes/{quoteId}/client-association` reads the current profile, the original request's
normalized declarations, exact CUIT/CUIL candidates, and reusable tags. Without a declared CUIT,
existing delivery contacts remain available as hints. Reads never create or associate a profile.

The seller includes `client_association` in `POST /v1/quotes/{quoteId}/sends`:

```json
{
  "recipient_phone": "+5491155550101",
  "client_association": {
    "tax_id": "20123456786",
    "new_client": { "name": "Example company" },
    "tag_ids": []
  }
}
```

Exactly one of `client_id` and `new_client` is selected. CUIT/CUIL and a new profile's name are
required; profile email and phone are optional. A WhatsApp or email delivery destination is required.
An existing profile's different fiscal identifier is refused; an absent identifier is filled only
when the seller confirms it. Other existing contact fields are preserved. Several exact matches
require an explicit selection, and no profiles are merged automatically.

The delivery service validates the selected profile and account-owned tags before contacting any
provider. A quote-scoped advisory lock serializes deliveries. The successful publication transaction
creates or loads the client, applies the confirmed tag set, associates both quote and RFQ, records
delivery outcomes, and moves the quotation to SENT. Failure rolls back those database changes.
A completed resend retains its profile; another profile cannot be substituted on a SENT quote.

Migration `00053_quote_send_client_association.sql` adds a normalized selection fingerprint to each
send. A replay with a different selection is refused, while a repeated successful operation does
not create another profile. The retired standalone association PUT returns 409; there is no client
association action after customer acceptance.

## Profiles and accepted sales

Profiles and tags are account-scoped. Every repository access includes the account; request and
quotation operations additionally enforce branch scope. PostgreSQL RLS remains enabled. Profile
lists and details continue projecting accepted sales within the caller's branch and seller reach.
Associating a client at SENT does not turn the quotation into an accepted sale.

Each accepted sale on `GET /v1/clients/{id}` carries `invoice_status`, read in the same query as the
sale: `PENDING` while any voucher waits for ARCA, otherwise `INVOICED` for an issued invoice no credit
note cancelled (in this server's environment or in production), `WAIVED` for «No requiere factura»,
and `UNINVOICED` for the rest. `domain.InvoiceStatusOf` is the one place that order lives. The
backoffice shows it as a column of the profile's sales table only while the account invoices, and the
whole row opens the order: the reference is the row's one link, stretched over the row, so a pointer
anywhere on it and a single keyboard stop both reach the order.

Tags retain their existing account-owned many-to-many model and replacement semantics. The send
form preserves the chosen profile's current tags and allows selecting existing reusable tags.
Profile tag editing and tag creation remain available through their existing endpoints.

The backoffice maps API snake_case to camelCase in its fetch boundary. The send dialog uses the
shared form controls, CUIT/CUIL input, phone input, validation states, and semantic design tokens.

## Fiscal data

A profile's CUIT or CUIL comes from the customer, when they create their account on the supplier's
webapp; neither the seller nor the AI loads, changes or deletes it. It lives in `client.tax_id`
(11 digits). A consumidor final's DNI lives apart in `client.dni` (7 or 8 digits, not unique: one
person may have several profiles). A DNI never makes a client registered — registered means a CUIT
— and never stands in for one where an invoice needs it.

`PUT /v1/clients/{id}/fiscal` replaces the legal name, fiscal address, IVA condition and DNI, and an
omitted field clears. It never writes `tax_id`:

- A body that carries `tax_id` is refused with **422** `TAX_ID_READ_ONLY`, so a caller still sending
  a CUIT learns it was not saved instead of reading a 200 as success.
- `dni` accepts 7 or 8 digits (separators dropped); anything else, a CUIT included, is **422**
  `INVALID_TAX_ID`.

The rest of the fiscal data stays editable by the seller, last write wins, and every invoice keeps
its own immutable copy of the receiver, which names the CUIT when there is one and the DNI otherwise.

Migration `00059_client_dni.sql` moved every 7–8 digit `tax_id` into `dni` and narrowed `tax_id` to
11 digits. It refuses to run, naming the count, if any `tax_id` is neither shape: no route ever
stored one, so such a value is hand-edited data for a person to settle rather than for the
migration to guess at.

An invoice A needs the receiver's CUIT, and an invoice B at or above
`INVOICE_RECEIVER_ID_THRESHOLD_AMOUNT` needs a CUIT or DNI. While the preview
(`GET /v1/quotes/{quoteId}/invoice`) reports `RECEIVER_CUIT_REQUIRED` or `RECEIVER_ID_REQUIRED`, it
also carries `customer_signup_url`: the supplier's customer sign-up page at
`https://<public_hostname>/signup`, or under `CUSTOMER_GOOGLE_LOCAL_ORIGIN` on a local stack. An
account with no `public_hostname` gets no link, because customer sign-up resolves the supplier by its
hostname and the shared webapp cannot register anyone; the card then says the customer site is not
configured yet (it is set through database administration, see
[public RFQ intake](public-rfq-intake.md)). With a link, the invoice card offers «Enviar link de
registro», a dialog with the link and a copy button; the seller sends it through their own channel,
because Coti has no free-text message to the client. An invoice B below the threshold goes to an
unidentified consumidor final unchanged.

The association at publication above is the one remaining place a seller sets a CUIT; it moves to
the same rule with the contact-management work.

## Verification

Service and PostgreSQL integration tests cover validated selections, branch and account isolation,
atomic publication and association, pre-acceptance customer history, rollback, and replay safety.
Frontend tests cover prefill, exact and ambiguous matches, required identifiers, API mapping,
retirement of the post-acceptance card, and the combined send action.
