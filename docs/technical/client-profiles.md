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
pending. Delivery destinations already recorded on sends remain immutable. Staff cannot bypass
proof using the retired contact suggestion mutation endpoint, which returns 403.

## Deterministic channel onboarding

WhatsApp and Gmail intake identify senders by verified channel contact, never by a claimed CUIT or
an inferred name. A known contact uses its existing profile without asking for a name again. An
unknown sender with an explicitly labeled full name creates a provisional profile without CUIT.
Otherwise the original message and request are retained and a fixed backend message asks for the
full name through the same channel. The accepted response starts with the full-name label shown
in the prompt. AI does not draft, decide or initiate this automatic onboarding request.

A name-only reply to a pending request completes its identity without creating another RFQ. The
original request text and provider messages are preserved. Email retains the original reply MIME
without adding its name text to the extraction queue. Event replays remain idempotent. Observations
retain the source event, branch, contact, declared name/CUIT and original text.

A durable outbox records prompt status and provider acknowledgement. Delivery occurs after the
ingestion transaction. Failed dispatch is retried on subsequent intake/replay. An uncertain SENDING
entry needs operational reconciliation before retry, to avoid duplicate customer messages.

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
| WHATSAPP_CONTACT_CODE_TEMPLATE                 | empty   | Approved Meta authentication copy-code template |
| WHATSAPP_CONTACT_CODE_LANGUAGE                 | es_AR   | Approved template language                      |

WhatsApp proof is unavailable until its supplier channel credentials and approved template are
configured. Email uses the configured mail adapter. Local links use CUSTOMER_GOOGLE_LOCAL_ORIGIN
when set; production uses the supplier's HTTPS hostname. Tests use fake mail/contact transports.

Migration 00056 adds the supplier-scoped unique fiscal index. Migration 00057 creates contact
collections and retained observations. Migration 00058 adds contact verification, a verified-live
unique index, challenges, onboarding outbox, composite channel scope and immutable-CUIT protection.
Existing contacts are treated as trusted and merged sources are retired. Conflicting live contacts
stop the migration without deleting or silently merging records. Migrations are executable; the
consolidated database schema is a reference only.

Integration tests cover proof-gated history movement, registered-name precedence, immutable data,
recovery separation, retained name replies, supplier isolation, RLS and independent-contact locks.
