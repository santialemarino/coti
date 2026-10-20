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
when the seller confirms it. Other existing contact fields are preserved. A non-null fiscal
identifier is unique within the supplier; creating another profile with that identifier returns
409 before contacting a delivery provider. Profiles are never merged automatically.

The delivery service validates the selected profile and account-owned tags before contacting any
provider. A quote-scoped advisory lock serializes deliveries. The successful publication transaction
creates or loads the client, applies the confirmed tag set, associates both quote and RFQ, records
delivery outcomes, and moves the quotation to SENT. Failure rolls back those database changes.
A completed resend retains its profile; another profile cannot be substituted on a SENT quote.

Migration `00055_quote_send_client_association.sql` adds a normalized selection fingerprint to each
send. A replay with a different selection is refused, while a repeated successful operation does
not create another profile. The retired standalone association PUT returns 409; there is no client
association action after customer acceptance.

## Profiles and accepted sales

Profiles and tags are account-scoped. Every repository access includes the account; request and
quotation operations additionally enforce branch scope. PostgreSQL RLS remains enabled. Profile
lists and details continue projecting accepted sales within the caller's branch and seller reach.
Associating a client at SENT does not turn the quotation into an accepted sale.

Tags retain their existing account-owned many-to-many model and replacement semantics. The send
form preserves the chosen profile's current tags and allows selecting existing reusable tags.
Profile tag editing and tag creation remain available through their existing endpoints.

The backoffice maps API snake_case to camelCase in its fetch boundary. The send dialog uses the
shared form controls, CUIT/CUIL input, phone input, validation states, and semantic design tokens.

## Verification

Service and PostgreSQL integration tests cover validated selections, branch and account isolation,
atomic publication and association, pre-acceptance customer history, rollback, and replay safety.
Frontend tests cover prefill, exact and ambiguous matches, required identifiers, API mapping,
retirement of the post-acceptance card, and the combined send action.

## Commercial contact collections

The customer portal's `/profile` page manages the authenticated customer's own commercial profile.
`GET` and `PUT /v1/customer/sites/{hostname}/profile` authorize by the session's supplier and exact
CUIT/CUIL. Client IDs and fiscal identifiers supplied in a body cannot select another profile.
A missing profile is created on a successful customer save or authenticated request declaration;
merely reading it does not create records. Credentials, Google subjects, recovery addresses and
verification challenges remain separate and are never updated by these operations.

The PUT body contains `name` and `contacts`, an array of `{kind, value, primary}`. Supported kinds
are `WHATSAPP` and `EMAIL`. Empty collections are valid. Addresses are trimmed and lowercased;
phone numbers use E.164. Duplicate normalized values within one profile and type are rejected.
Every populated type has exactly one primary destination. Deleting a primary requires selecting
a replacement unless the whole type is cleared. Contact values may be shared across profiles.
There is no declared count, multiple-contact switch, or business cap on the collection.

Services own the replacement transaction and serialize commercial contact edits per supplier.
The repository retains existing contact IDs and origins for unchanged values, adds new values,
removes omitted values, and projects the selected primaries into `client.phone` and `client.email`.
Immutable delivery destinations remain unchanged. New request forms prefill the current commercial
name and primaries after branch selection. Sending still accepts WhatsApp, email, or both and
always publishes the quotation online.

Migration `00056_unique_client_tax_id.sql` adds the supplier-scoped unique fiscal index. Existing
duplicates stop the migration without deleting or merging data. Migration `00057_client_contacts.sql`
adds account-scoped, RLS-protected contacts and retained observations. Existing scalar contacts
become primaries, with their original spelling retained in `original_value`; the scalar values
are preserved during migration. An insert trigger seeds collections for existing profile creators.

## Deterministic intake and review

| Event                                                                                    | Validation and action                                                                                                                                             | Seller review                                 |
| ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| Verified WhatsApp text or retained Gmail message from a new sender                       | Normalize channel sender; create an independent profile if there is no existing fiscal candidate or ambiguity; retain the original declaration. CUIT is optional. | No                                            |
| Explicit first-line single-name introduction, such as `Hola, soy Pepe`                   | Accept the complete narrow pattern; do not treat a name as identity. Longer or ambiguous prose stays only in the original message.                                | Only on conflict                              |
| One explicitly labeled CUIT/CUIL in message text                                         | Normalize and checksum-validate; use an exact supplier-scoped lookup. Never infer it from a name or an unlabeled number.                                          | On a conflicting or existing fiscal candidate |
| Known contact with a missing fiscal identifier                                           | Fill it only when there are no existing request/quote associations that could expose earlier history.                                                             | Required when earlier associations exist      |
| Authenticated portal save or request declarations                                        | Validate and save the customer's own commercial data; adding a contact does not require seller approval.                                                          | No                                            |
| New sender with an existing fiscal candidate, shared contact, or conflicting declaration | Retain a pending observation; do not overwrite commercial fields or attach the request.                                                                           | Yes                                           |
| AI-produced or inferred customer fields                                                  | The contact detector consumes no AI output. Existing AI proposal validation and seller approval remain required.                                                  | Yes before materialization                    |

WhatsApp detection runs after reservation and conversation routing, within the same intake
transaction. Gmail detection runs alongside the retained message and attachments. Neither changes
conversation routing or sends an automatic reply. Web request enrichment follows durable reception.
Every observation keeps its source event, branch, contact, declared name/CUIT, original text and
decision provenance. Replaying an event cannot recreate a removed contact. Repeated pending cases
retain each message but present one pending suggestion per branch, candidate and contact.

`GET /v1/client-contact-suggestions` lists pending cases within the seller's branch reach;
`PUT /v1/client-contact-suggestions/{suggestionId}` accepts `ASSOCIATE`, `SEPARATE`, or `MERGE`.
Association adds the reviewed contact without replacing the current primary. Separation preserves
the original declaration and, if necessary, creates an independent profile without copying an
already-owned CUIT. Later messages respect that separation. A confirmed merge can absorb a
provisional profile without a fiscal identifier into the canonical fiscal profile. It preserves
both profiles and their original data, unions contacts and tags, moves commercial request, quote
and notification associations, and records the source's `merged_into` link. It refuses source
requests or quotes outside the caller's branch/seller reach. Repeated decisions are idempotent;
changing an already-resolved decision returns a conflict.

Pending cases remain accessible from Clients and its contextual column. Narrow screens retain
the Clients page entry. Authenticated customers see a generic review notice in their own profile;
no outbound notification is sent. Commercial decisions never link credentials or establish control
of a phone/email. History retains its separately approved supplier-and-CUIT access rule.
