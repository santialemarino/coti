# Client profiles and accepted sales

US-22 adds the client history as a projection of the quoting flow. It is not a separate CRM
module: an accepted quote becomes part of a profile only after a seller confirms the association.

## Lifecycle

Sending a quote stores each destination in `quote_send`; it does not create a `client`, update
contact data, or set `quote.client_id` or `rfq.client_id`. Once the quote reaches `ACCEPTED`, the
backoffice requests association context. The API returns the most recent WhatsApp and email
destinations as hints plus account clients that match either contact exactly after normalization.
The seller can also search the account-scoped client directory by name, phone, or email when the
sale arrived through a different contact. Both reads have no side effects.

The seller then chooses one existing suggestion or provides at least one field for a new client.
`PUT /v1/quotes/{quoteId}/client-association` locks the accepted quote and, in one tenant
transaction:

1. loads the selected account client or creates the confirmed profile;
2. validates and replaces that profile's selected tags;
3. sets the same `client_id` on the quote and its source RFQ.

The service rejects quotes outside `ACCEPTED`, client or tag IDs from another account, and a body
that selects both or neither association option. There is no fuzzy match, automatic merge, or
automatic contact overwrite. Selecting a directory result sends only its existing `client_id` and
the seller-confirmed tag set; delivery destinations remain in `quote_send`.

## Tags

Tags belong to the account and profiles use the existing `client_tag` many-to-many table. Migration
`00027_default_client_tags.sql` adds `Recurrente` and `Obra grande` to existing accounts; account
registration seeds the same defaults in its creation transaction.

Sellers can create reusable tags inline. The service trims the name, collapses repeated whitespace,
and relies on `uq_tag_account_name` for case-insensitive idempotency. Both association and profile
editing use replacement semantics for the selected tag set. The existing `tag.color` column is not
part of this workflow.

## HTTP surface

| Method | Route                                     | Purpose                                                    |
| ------ | ----------------------------------------- | ---------------------------------------------------------- |
| `GET`  | `/v1/clients`                             | Account profiles with branch-visible accepted-sale summary |
| `GET`  | `/v1/clients/{clientId}`                  | Contact, tags, and visible accepted sales                  |
| `PUT`  | `/v1/clients/{clientId}/tags`             | Replace a profile's account-owned tags                     |
| `GET`  | `/v1/tags`                                | List reusable account tags                                 |
| `POST` | `/v1/tags`                                | Create or return a normalized reusable tag                 |
| `GET`  | `/v1/quotes/{quoteId}/client-association` | Return current association, exact matches, hints, and tags |
| `PUT`  | `/v1/quotes/{quoteId}/client-association` | Confirm the client and tag set for an accepted quote       |

Money remains a decimal string at the API boundary. The backoffice maps snake_case responses to
camelCase in `lib/api/client-profiles.ts`; browser writes travel through authenticated BFF routes.

## Scope and visibility

Profiles and tags are account-wide. Every repository query includes `account_id`, and request work
runs under the tenant RLS role. Accepted-sale counts and history are additionally constrained by
the caller's branch reach and seller assignment. An active branch is mandatory when reading or
confirming an individual quote association.

## Verification

The service tests pin normalization, accepted-only association, transactional quote/RFQ updates,
and the no-write suggestion contract. PostgreSQL integration tests cover send without implicit
client creation through confirmed association and profile history. Backoffice tests cover API
mapping and the seller confirmation boundary.
