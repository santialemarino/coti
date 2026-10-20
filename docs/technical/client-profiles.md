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
required; profile email and phone are optional. The WhatsApp delivery recipient remains required.
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
