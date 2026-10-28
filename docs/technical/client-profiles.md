# Client profiles and verified contacts

## Identity and required data

A supplier has one active commercial profile per CUIT/CUIL. A provisional profile has no fiscal
identifier; its verified phone or email identifies its channel history. There is no provisional
boolean. New profiles require a full name and at least one phone or email. Seller-created profiles
and unknown channel senders never receive a CUIT from seller input or message declarations.

Customer registration requires a checksum-valid CUIT/CUIL, full name, credentials and at least one
contact. Registration creates or adopts the supplier's unique fiscal profile in the same transaction
as the identity and session. Only registration assigns the CUIT. Subsequent profile, quotation-send
and fiscal edits cannot assign, replace or clear it; PostgreSQL also rejects changing a non-null
client tax identifier. Credentials remain supplier-scoped and separate from commercial contacts.

## Customer profile

GET and PUT /v1/customer/sites/{hostname}/profile authorize the exact authenticated CUIT and supplier.
A request body cannot choose another profile. The customer portal's My data page manages the name,
phones, emails, legal name, IVA condition and fiscal address. Fiscal fields are optional; the full
name and at least one contact remain mandatory. Fields are added one at a time with a plus button;
there is no declared quantity or multiple-contact switch.

The contact array contains kind, value and primary. Supported kinds are WHATSAPP and EMAIL. Phones
use E.164 and emails are trimmed and lowercased. Duplicate normalized entries are rejected. Each
populated kind has exactly one primary. Removing its primary requires a replacement unless the
whole kind is removed. Scalar client.phone and client.email project those primaries for compatibility.

New customer-managed contacts remain unverified. Requesting a proof uses POST
/profile/contacts/verification; confirming uses POST /profile/contacts/verification/confirm under
the same supplier-scoped customer prefix. Email uses a random link capability; WhatsApp uses a
six-digit code through an approved authentication copy-code template. Email confirmation requires
an explicit action on /verify-contact; loading the page does not consume the capability.

Challenges store only hashes, are bound to the customer, profile and supplier, expire, enforce a
resend cooldown and count failed attempts. Invalid attempts commit their count without transferring
history. A proof for a destination owned by another fiscal profile is refused. A verified destination
can belong to only one live profile per supplier. Pending claims do not establish ownership.

Successful proof can absorb the matching provisional profile. The transaction retains the source
profile and its provenance, sets merged_into, retires its old contacts, unions commercial contacts
and tags, and moves RFQ, quotation and notification history to the fiscal profile. The registered
name prevails. Only the proved contact becomes verified; other newly imported contacts remain
pending. Successful proof also records customer-specific contact ownership and snapshots access
to the source orders before the merge. Delivery destinations already recorded on sends remain immutable. Staff cannot bypass
proof using the retired contact suggestion mutation endpoint, which returns 403.

## Deterministic channel onboarding

WhatsApp and Gmail intake identify senders by verified channel contact, never by a claimed CUIT or
an inferred name. A known contact uses its existing profile without asking for a name again. An
unknown sender immediately receives a provisional profile without CUIT, using the channel display
name, or the contact value if the provider supplies none. An explicit full-name declaration takes
precedence. The request continues processing while a fixed backend message (decision 79) asks for
the full name through the same channel. The text is the same for every unknown sender, so it reveals
no profile data. AI does not draft, decide or initiate this automatic onboarding request.

By email the prompt answers as the supplier: it goes through the platform mailer (the only address the
provider lets Coti send as) with the supplier's name as the sender name, the subject "Tu pedido a
<supplier>", Reply-To set to the branch mailbox, and In-Reply-To/References set to the customer's
Message-ID so it threads under their mail. The Gmail connection is read-only, so it cannot send from
the mailbox itself. Mail from a `no-reply`, `mailer-daemon`, `postmaster` or `bounce*` address, or
carrying `Auto-Submitted` (other than `no`), `Precedence: bulk|list|junk`, `List-Id` or
`List-Unsubscribe`, never gets a prompt. Contact verification and the registration link keep their
own subjects.

While the prompt is answerable (status SENT, within 24 hours of delivery), a short text with no
attachments from that contact is taken as the name reply: it joins the request the prompt answered,
updates the provisional name, and creates no RFQ. The rule lives in `client_contact_name_reply.go`:
after dropping greeting and closing lines («Hola», «Saludos», «Gracias») and a leading «soy», «me
llamo», «mi nombre es» or «Nombre y apellido:», exactly one line must remain, of at most 60
characters and 5 words, letters only (plus spaces, apostrophes, hyphens and periods), with no digit
and no order word (quantities, units, «necesito», «precio», common materials such as «cemento» or
«arena»). Anything else follows the normal flow and the prompt stays pending, so a short order
(«2 bolsas de cemento», «necesito arena») is never swallowed. In email, the quoted message (`>`
lines, «El … escribió:», «On … wrote:», forwarded/original-message markers, Outlook headers), the
`-- ` signature and common mobile signatures are removed before the rule runs, and automated mail is
never a reply. The thresholds are constants, not settings: raising them lets orders through as names.

Known contacts are not prompted again. Later requests do not overwrite the confirmed name; portal
requests never overwrite an existing profile name. The original request text and provider messages
are preserved. Email retains the original reply MIME without adding its name text to the extraction
queue. Event replays remain idempotent. Observations retain the source event, branch, contact,
declared name/CUIT and original text.

A durable outbox records prompt status, the time of the last status change and provider
acknowledgement. Delivery occurs after the ingestion transaction. Failed dispatch is retried on the
contact's next message. An entry left in SENDING by a crash is claimed again once it is older than
CUSTOMER_CONTACT_PROMPT_LEASE_MINUTES, so it is never stranded; a delivery still inside its lease is
not claimed twice.

Contact detection locks normalized contact keys and then fiscal/profile keys using transaction-level
PostgreSQL advisory locks, with stable ordering. Unrelated contacts of the same supplier do not
wait for each other. The account remains part of every lock key and every tenant-scoped query;
branch-scoped ingestion and quotation operations also filter the branch. Services own transactions,
repositories never commit, and tenant tables retain RLS.

## Quotation publication and invoicing

The seller includes client_association in POST /v1/quotes/{quoteId}/sends, selecting exactly one
existing client_id or new_client. A new profile requires full name and phone or email, without CUIT.
An existing profile may retain its current immutable CUIT. Validation runs before contacting delivery
providers. WhatsApp, email-only and combined sends remain available. Successful publication assigns
the same client to RFQ and quote, records delivery outcomes and moves the quote to SENT atomically.
A completed resend retains its client and idempotency fingerprint.

Only invoicing is blocked by missing required fiscal data, including registration/CUIT. The existing
invoice checks still validate legal name, IVA condition and fiscal address as applicable. Quoting,
receiving requests and sending quotations do not require the optional fiscal fields.

The seller can explicitly send a registration/profile completion link from the client detail through
a verified primary channel. POST /v1/clients/{clientId}/registration-link requires the active branch.
The link opens login with a destination of /profile, where the customer can register or access an
existing account and complete data. It carries no credentials or claimed CUIT. Customers with an
existing CUIT add and verify the provisional contact from their existing account.

## Recovery email

GET and PUT /v1/customer/sites/{hostname}/auth/recovery-email expose only the authenticated customer's
optional credential recovery email and verified state. It is stored separately from commercial emails.
Saving a new address sends a one-use /verify-email link; clearing it disables email recovery. Changing
or removing the address invalidates outstanding verification and password reset capabilities. Saving
the same verified address preserves its verification. Saving the same pending address resends proof,
subject to the customer mail rate limit. Commercial contact proof never changes credential recovery.

Phone-only signup is allowed but cannot recover a password until a recovery email is confirmed.
Recovery requires the CUIT and the matching verified email. Sellers have no recovery role. These flows
send channel messages; they do not create browser/device push notifications.

## Configuration and migrations

| Variable                                       | Default | Purpose                                         |
| ---------------------------------------------- | ------- | ----------------------------------------------- |
| CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES      | 10      | Contact proof lifetime                          |
| CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS | 60      | Minimum interval between proofs                 |
| CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS     | 5       | Failed-attempt limit                            |
| CUSTOMER_CONTACT_PROMPT_LEASE_MINUTES          | 5       | Age at which a stuck name prompt is reclaimed   |
| WHATSAPP_CONTACT_CODE_TEMPLATE                 | empty   | Approved Meta authentication copy-code template |
| WHATSAPP_CONTACT_CODE_LANGUAGE                 | es_AR   | Approved template language                      |

WhatsApp proof is unavailable until its supplier channel credentials and approved template are
configured. Email uses the configured mail adapter. Local links use CUSTOMER_GOOGLE_LOCAL_ORIGIN
when set; production uses the supplier's HTTPS hostname. Tests use fake mail/contact transports.

Migration 00060 adds the supplier-scoped unique fiscal index. Migration 00061 creates contact
collections and retained observations. Migration 00062 adds contact verification, a verified-live
unique index, challenges, onboarding outbox, composite channel scope and immutable-CUIT protection.
Legacy Argentine phones (area 11, 2xx or 3xx, written nationally or after 54) are stored in the
+549 form WhatsApp reports; a value the contact normalizer would reject stays unverified, and so
does a malformed email. Each other legacy value is trusted for channel routing with one owner per account:
the profile with a CUIT, else the most recently created one. Every other copy stays unverified
and raises a PENDING observation, with no source message or CUIT, so a seller reviews the shared
contact.
Merged sources are retired.
Migration 00063 stores customer-specific ownership and proof-gated provisional history with RLS
and composite tenant foreign keys. Legacy commercial verification does not substitute for a
customer proof; My data shows those contacts as pending until this account verifies them. No shared contact is deleted
or silently merged. Migration 00065 adds the outbox's `status_changed_at`, which bounds both the
reply window and the delivery lease. Migrations are executable; the
consolidated database schema is a reference only.

Integration tests cover proof-gated history movement, registered-name precedence, immutable data,
recovery separation, retained name replies, supplier isolation, RLS and independent-contact locks.
They also cover free-form WhatsApp and quoted email name replies, short orders that stay orders, the
24-hour reply window, automated senders that get no prompt, the supplier-named email prompt and
the reclaimed delivery lease; unit tests cover the reply rule and the email stripping case by case.

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
shared form controls, phone input, validation states, and semantic design tokens.

## Fiscal data

A profile's CUIT or CUIL comes from the customer, when they create their account on the supplier's
webapp; neither the seller nor the AI loads, changes or deletes it. It lives in `client.tax_id`
(11 digits). A consumidor final's DNI lives apart in `client.dni` (7 or 8 digits, not unique: one
person may have several profiles). A DNI never makes a client registered — registered means a CUIT
— and never substitutes for the CUIT an invoice A requires.

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

An invoice A needs the receiver's CUIT, which only the customer's registered account provides, so
a DNI never satisfies it (`RECEIVER_CUIT_REQUIRED`). An invoice B at or above
`INVOICE_RECEIVER_ID_THRESHOLD_AMOUNT` needs a CUIT or a DNI (`RECEIVER_ID_REQUIRED`), and an
invoice B below it goes to an unidentified consumidor final with no document at all. While the
preview (`GET /v1/quotes/{quoteId}/invoice`) reports either receiver issue, it also carries
`customer_signup_url`: the supplier's customer sign-up page at `https://<public_hostname>/signup`,
or under `CUSTOMER_GOOGLE_LOCAL_ORIGIN` on a local stack. An account with no `public_hostname` gets
no link, because customer sign-up resolves the supplier by its hostname; the card then says the
customer site is not configured yet (see [public RFQ intake](public-rfq-intake.md)). With a link,
the invoice card offers a copy dialog so the seller can send it through their own channel. The
client detail also offers an explicit registration-link send through a configured contact channel.

Quotation publication cannot assign a CUIT; it creates provisional profiles or selects existing ones.
