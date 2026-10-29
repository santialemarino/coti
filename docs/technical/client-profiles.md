# Client profiles and verified contacts

## Identity and required data

A supplier has one active commercial profile per CUIT/CUIL. A provisional profile has no fiscal
identifier; its verified phone or email identifies its channel history. There is no provisional
boolean. New profiles require a full name and at least one phone or email. Seller-created profiles
and unknown channel senders never receive a CUIT from seller input or message declarations.

Customer registration requires a checksum-valid CUIT/CUIL, full name, credentials and at least one
contact. Registration creates or adopts (see below) the supplier's unique fiscal profile in the same transaction
as the identity and session. Only registration assigns the CUIT. Subsequent profile, quotation-send
and fiscal edits cannot assign, replace or clear it; PostgreSQL also rejects changing a non-null
client tax identifier. Credentials remain supplier-scoped and separate from commercial contacts.

## Google sign-up

A first Google sign-in completes registration on /google-complete with the full name, the CUIT/CUIL
and at least one contact, like a password registration. The Google address is never copied to the
profile on its own: the screen offers it as an unchecked option, and only when Google's ID token
says `email_verified`. When the customer checks it, the address is saved as a verified contact,
because Google already proved control of it; the API refuses the option for an address Google did
not verify. Otherwise the customer enters a WhatsApp or another email, which stays unverified until
a code confirms it.

A chosen Google address goes through the same step as a confirmed code, inside the registration
transaction: it merges the provisional profile that holds the address verified, counts toward
`uq_verified_client_contact`, records the customer's ownership and becomes the main contact under
the usual default. It is the primary email even when the customer also typed another one. On an
adopted earlier profile it proves ownership exactly when the profile already had that address (see
below). An address already verified on another CUIT/CUIL's profile is refused with 409
`CONTACT_IN_USE`, and the registration is not created.

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
to the source orders before the merge. Delivery destinations already recorded on sends remain
immutable. No staff route merges into a registered profile: the seller merges only provisional
profiles (see [Seller actions on provisional profiles](#seller-actions-on-provisional-profiles)).

The source's other contacts keep routing. An inbound message from one of them attaches to the fiscal
profile, with no new provisional profile and no name question. The marker is the source's retired,
verified contact row together with `merged_into`; no column records it. Routing gives no ownership:
the contact stays pending on the fiscal profile, has no `customer_contact_ownership` row, and its
new orders reach the customer only through the profile's own history. Routing yields to a live
verified contact: once anyone in the supplier proves that destination, messages go to the profile
that proved it. Routing stops when the customer removes the pending copy from their profile. When
several merges retired the same destination, the latest one routes.

Every merge, whichever path ran it, also fills the target's empty DNI, legal name, IVA condition and
fiscal address from the source without overwriting a value the target holds, repoints any profile
already merged into the source so `merged_into` always names a live profile, settles any open name
question to the source's contacts (so a late reply cannot rename the target), and writes a
`client_merge_audit` row (source, target, `merged_by`, `merged_at`; `merged_by` is NULL when the
customer's own proof merged them).

## Seller actions on provisional profiles

A provisional profile (no CUIT) is the seller's to tidy; a registered one belongs to its customer.

- **Rename.** `PUT /v1/clients/{id}/name` sets the name of a provisional profile, for one that kept a
  WhatsApp display name, and settles any open name question to its contacts so a late reply cannot
  overwrite the correction. A registered profile answers **403** `CLIENT_REGISTERED`. The client detail
  shows the name with an inline editor, or, on a registered profile, a line saying the customer
  manages it.
- **Merge.** `POST /v1/clients/{id}/merge` with `source_client_id` merges the source into the path
  profile, which survives. `GET /v1/clients/{id}/merge-candidates` lists the profiles a merge would
  accept. Both profiles must be live, have no CUIT (**403** `CLIENT_REGISTERED`), and be within the
  caller's reach: every order in the caller's branches and, for a seller, no quote of another seller
  (**403** `CLIENT_OUT_OF_REACH`). An already merged profile is **409** `CLIENT_MERGED`, and another
  account's is **404**. The source's orders, tags and notifications move; its live contacts retire
  and the target gets a copy of each that keeps its channel verification, so they route to the
  target directly and a customer who later proves one brings the whole merged history in. Both profiles
  are locked with the same per-profile key inbound routing takes, so a message arriving mid-merge
  routes to the survivor.
- **Coincidencias.** `GET /v1/client-contact-suggestions` lists the PENDING observations in the
  caller's branches, such as the duplicate legacy contacts migration 00062 raised, with both
  profiles' names and whether either is registered. `PUT /v1/client-contact-suggestions/{id}` takes
  `MERGE`, which runs the same merge into the candidate (or into the source when `keep_client_id`
  names it; any other id is **422**), or `SEPARATE`, which dismisses the case; either marks the
  observation decided with the seller and time. A side merged since the case was raised is settled on
  the profile it ended up in, and a case whose two sides already share one survivor is just marked
  merged. With a CUIT on either side the backoffice offers only the dismissal.
- **Old links.** `GET /v1/clients/{id}` on a merged profile answers with the profile it ended up in,
  and the backoffice redirects the old URL there.

## Adopting an earlier profile

A CUIT is public, so registering one that an earlier seller-entered profile already carries links
the identity to that profile without showing or changing it. The profile counts as proved once the
customer verifies a contact it had before the identity existed — the ownership row with
`includes_profile_history`, the same proof that opens the profile's earlier orders. A profile created
with or after the identity is proved from the start.

Until then the profile keeps its name, fiscal data, contacts and primaries, and the seller sees it
unchanged. `client_contact.declared_by` records which identity entered each contact. GET /profile
returns `ownership_unproved: true`, the name the customer entered (held on `customer_identity.name`)
and only the contacts it declared, ordered by value with the first of each kind shown as primary, so
neither tells which of them the profile already had. No fiscal data is returned. PUT /profile writes
the name to the identity and replaces only the customer's own contacts: an earlier contact it omits
is kept. No customer path (PUT, a portal request, a merge on proof) sets a primary on the profile or
writes `client.phone`/`client.email` before proof, since the seller quotes and routes through them.
Fiscal data is refused with 403 `PROFILE_OWNERSHIP_UNPROVED`. A verification code goes only to a
contact the customer declared, so the owner's phone or email cannot be probed by a code request
alone. A customer can still declare the owner's value and request a code to it, within the limits
under [Abuse limits](#abuse-limits). To prove ownership the customer
declares a contact the profile already had and verifies it; from then on GET and PUT work on the
whole profile, which keeps its earlier name until the customer changes it.

## Order history

GET /orders lists a request when any of these holds:

- the customer authored it in the portal;
- it belongs to a profile the customer proved (`includes_profile_history`), with its whole past;
- it belonged to a provisional profile merged into theirs on proof, which the merge snapshots;
- it arrived from a contact the customer proved, at or after that proof.

The last rule compares the inbound message's `provider_received_at`, the provider's arrival time,
with `customer_contact_ownership.verified_at`. A phone or email that changes hands therefore opens
only what its new owner sent: an order from a recycled company number stays with the customer who
had it then. Ownership is timed by the first proof; repeating the proof keeps that time, and
removing the contact deletes the row, so adding it again starts a new period.

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
`List-Unsubscribe`, `X-Autoreply` or `X-Autorespond`, a null `Return-Path` or a `multipart/report`
body (a bounce), never gets a prompt. Contact verification and the registration link keep their
own subjects.

A message only counts as the name reply when nothing else claims it. A live quote conversation
takes it first, so «Confirmo» answers the quote. Otherwise the prompt must be answerable: status
SENT, delivered less than 24 hours ago, and no exchange since, meaning no later message from that
contact and no quote sent to it. The message must have no attachments and pass the rule in
`client_contact_name_reply.go`. Greeting and closing lines («Hola», «Saludos», «Gracias») and a
leading «soy», «me llamo», «mi nombre es» or «Nombre y apellido:» are dropped. Exactly one line must
then remain, of at most 60 characters, with two to five name words (particles such as «de», «la»,
«del», «van» do not count), made of letters (plus spaces, apostrophes, hyphens and periods). Its
first word must be a known given name: the rule is anchored on an allow-list, `given_names.txt`
(common Spanish and Argentine given names, lowercase and unaccented, embedded in the binary). It must
contain no digit, link or address, no order or confirmation word (quantities, units, «necesito»,
«precio», «confirmo», «cancel…», «paso», «factura», «foto», «ya», number words and edit verbs such as
«dos», «más», «agregá» or «sumale», common materials and brands such as «cemento» or «Klaukol»), and
no word of the supplier's own catalog (product names and synonyms). A
match joins the request the prompt answered, updates the provisional name and creates no RFQ.
Anything else follows the normal flow and the prompt stays pending.

The rule is anchored rather than exhaustive because its two errors do not cost the same: taking an
order change as a name loses it, while missing a name only leaves one extra RFQ draft the seller can
discard. So a reply that does not start with a known given name, such as «Agregá dos», «Todo bien»,
a nickname or an initial, is an ordinary message: it follows the normal flow and the question stays
pending. Given names that are also everyday words (days, months, «Franco», «Ángel», «Victoria»,
«Vale») are left out of the list on purpose. A name missing from the list is added to the file. In email, the quoted message
(`>` lines, «El … escribió:», «On … wrote:», forwarded/original-message markers, Outlook headers and
the rule of `_`, `-` or `=` above them), the `-- ` signature and common mobile signatures are removed
before the rule runs, and automated mail is never a reply. The thresholds are constants, not
settings: raising them lets orders through as names.

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

## Main contact

Each registered profile has one main contact, the one account recovery preselects. It is always a
live contact the profile's customer verified. The customer's explicit choice is stored as
`client_contact.is_main` (a partial unique index allows one per profile, and a check refuses it on an
unverified or retired row); without a choice, the main contact is the first WhatsApp the customer
verified, else the first contact, so a WhatsApp verified later becomes the default without a write.
PUT /v1/customer/sites/{hostname}/profile/main-contact accepts only a contact the customer verified
(404 otherwise); GET /profile marks it `main`. Removing it from the profile falls back to the default.

## Account recovery

Recovery sends a one-use code to a verified contact; there is no separate recovery address. The
three public routes under /v1/customer/sites/{hostname}/auth/recovery are:

- `POST /options` with the CUIT/CUIL returns the customer's verified contacts, masked
  (`+54 9 ••• ••••-0101`, `j•••@gmail.com`), the main one first. `available` is false for a channel
  no code can reach, and `preselected` marks the first available one.
- `POST /code` with the CUIT/CUIL and a listed contact sends a six-digit code, stored hashed in
  `client_contact_challenge` with purpose `RECOVER` and bound to the supplier, the customer and the
  destination. It expires after `CUSTOMER_RECOVERY_TTL_MINUTES` and counts toward the same
  cooldown, per-supplier destination cap and cross-supplier spam cap as a verification proof.
- `POST /reset` with the code and a new password checks the password policy first, redeems the
  code (constant-time compare, `CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS` tries, single use, and
  the contact must still be verified), replaces the password, revokes every session and the other
  outstanding codes, and returns a new session. A wrong code is a destination failure, so it counts
  toward the per-supplier lockout. A Google-only account gains a password this way.

The CUIT/CUIL only selects the account: nothing is granted without the code, and sellers have no
role in it. **An unknown or disabled CUIT/CUIL answers exactly like an account with no verified
contact: an empty list.** A cap or lockout refusal is the same 429 `ACCOUNT_LOCKED` a verification
proof gets. What the list reveals is only what a masked list inherently does, that an
account with at least one verified contact exists. Each route is limited per caller address and per
CUIT/CUIL named (the reset per code), on the route limiter.

A WhatsApp code outside the 24-hour window needs Meta's authentication template, so while
`WHATSAPP_CONTACT_CODE_TEMPLATE` is empty WhatsApp contacts are listed but not available; the rule
lives in one function (`codeDeliverable`) and switches on with the template.

## Abuse limits

Declaring a stranger's phone or email and requesting codes for it would make Coti a spam relay, and
several accounts sharing one victim's destination could brute-force its 6-digit code. Four limits
stop that:

- **Route rate limits.** Requesting and confirming a proof (`RATE_LIMIT_CONTACT_VERIFICATION_MAX`)
  and saving the profile (`RATE_LIMIT_PROFILE_MAX`) are each counted twice per window, once by
  customer and once by client address, and answer 429 `RATE_LIMITED` past either.
- **Codes per destination, per supplier.** At most `CUSTOMER_CONTACT_VERIFICATION_DESTINATION_MAX`
  proofs go from one supplier to one destination per
  `CUSTOMER_CONTACT_VERIFICATION_DESTINATION_WINDOW_HOURS`.
- **Failures per destination, per supplier.** `CUSTOMER_CONTACT_VERIFICATION_FAILURE_MAX` wrong proofs
  against one destination at one supplier inside `CUSTOMER_CONTACT_VERIFICATION_LOCKOUT_MINUTES`,
  across every challenge, lock it at that supplier: no new code is sent and even the right one is
  refused until the failures age out. Each wrong proof is a `client_contact_challenge_failure` row.
- **Codes per destination, across suppliers.** At most `CUSTOMER_CONTACT_VERIFICATION_SPAM_MAX` proofs
  reach one destination per `CUSTOMER_CONTACT_VERIFICATION_SPAM_WINDOW_HOURS` from all suppliers
  together. It is anti-spam only, so it sits well above the per-supplier cap.
- **Contacts per profile.** PUT /profile and signup refuse a save that takes a profile past
  `CUSTOMER_PROFILE_CONTACTS_MAX` contacts with 422 `CONTACT_LIMIT`. A profile a merge already took
  past the cap can still be saved with the contacts it has.

The daily cap and the lockout count per supplier because a code only works at the supplier that sent
it: pooling them across suppliers would buy nothing against brute force and would let an attacker at
one supplier lock the real owner out of every other. Within one supplier the attacker and the owner
share the counters, so an attacker who declares the owner's destination there can still exhaust them
for that supplier; that is the accepted trade-off.

The caps are read in the database, so they hold across API instances. The counts come from
`contact_destination_activity`, a `SECURITY DEFINER` function that sees every account's rows and
returns only three numbers, under a transaction-scoped advisory lock on the destination so concurrent
requests cannot both pass. A refused request or confirm answers 429 `ACCOUNT_LOCKED`, the same as the
per-customer resend cooldown, so the caller cannot tell whether another supplier asked for the
destination.

The per-address rate limit only tells customers apart when the API trusts the webapp as a proxy
(`RATE_LIMIT_TRUSTED_PROXY_HOPS` and `RATE_LIMIT_TRUSTED_PROXY_CIDRS`) and the webapp forwards the
visitor's address (`WEB_TRUSTED_PROXY_HOPS`). Without them every customer shares the webapp server's
bucket, as customer login already does.

## Configuration and migrations

| Variable                                               | Default | Purpose                                            |
| ------------------------------------------------------ | ------- | -------------------------------------------------- |
| CUSTOMER_CONTACT_VERIFICATION_TTL_MINUTES              | 10      | Contact proof lifetime                             |
| CUSTOMER_CONTACT_VERIFICATION_COOLDOWN_SECONDS         | 60      | Minimum interval between proofs                    |
| CUSTOMER_CONTACT_VERIFICATION_MAX_ATTEMPTS             | 5       | Failed-attempt limit                               |
| CUSTOMER_CONTACT_VERIFICATION_DESTINATION_MAX          | 5       | Proofs one supplier sends a destination per window |
| CUSTOMER_CONTACT_VERIFICATION_DESTINATION_WINDOW_HOURS | 24      | Window of the per-destination proof cap            |
| CUSTOMER_CONTACT_VERIFICATION_FAILURE_MAX              | 10      | Wrong proofs that lock a destination at a supplier |
| CUSTOMER_CONTACT_VERIFICATION_LOCKOUT_MINUTES          | 60      | Window the failures count over                     |
| CUSTOMER_CONTACT_VERIFICATION_SPAM_MAX                 | 20      | Proofs one destination receives from all suppliers |
| CUSTOMER_CONTACT_VERIFICATION_SPAM_WINDOW_HOURS        | 24      | Window of the cross-supplier cap                   |
| CUSTOMER_PROFILE_CONTACTS_MAX                          | 10      | Contacts a customer can save on a profile          |
| RATE_LIMIT_CONTACT_VERIFICATION_MAX                    | 5       | Proof requests or confirms per window              |
| RATE_LIMIT_PROFILE_MAX                                 | 10      | Profile saves per window                           |
| CUSTOMER_CONTACT_PROMPT_LEASE_MINUTES                  | 5       | Age at which a stuck name prompt is reclaimed      |
| CUSTOMER_RECOVERY_TTL_MINUTES                          | 15      | Account recovery code lifetime                     |
| WHATSAPP_CONTACT_CODE_TEMPLATE                         | empty   | Approved Meta authentication copy-code template    |
| WHATSAPP_CONTACT_CODE_LANGUAGE                         | es_AR   | Approved template language                         |

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
or silently merged. Migration 00064 adds `client_contact.declared_by` and attributes existing
customer-entered contacts to the identity that entered them. Migration 00065 adds the outbox's
`status_changed_at`, backfilled from `created_at`, which bounds both the reply window and the
delivery lease. Migration 00067 drops the separate recovery email and its link challenges, adds the
challenge purpose and the main-contact flag. Migration 00068 adds `client_merge_audit` with RLS.
Migrations are executable; the
consolidated database schema is a reference only.

Integration tests cover proof-gated history movement, contact history bounded by the ownership
period, registered-name precedence, earlier-profile
adoption hidden until proof, immutable data,
account recovery by contact (masking, attempts, single use, WhatsApp availability, supplier
isolation, the main-contact choice), retained name replies, supplier isolation, RLS and independent-contact locks.
They also cover free-form WhatsApp and quoted email name replies, short orders that stay orders, a
quote reply that stays in its conversation, refusals after a quote or for a catalog word, the
delivered-prompt and 24-hour conditions, auto-replies and automated senders, the supplier-named
email prompt and the reclaimed delivery lease; unit tests cover the reply rule and the email
stripping case by case, and a scripts test covers the 00065 backfill.

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
its own immutable copy of the receiver, which names the CUIT when there is one, else the DNI, and no
document at all for an unidentified consumidor final below the threshold described next.

Migration `00059_client_dni.sql` moved every 7–8 digit `tax_id` into `dni` and narrowed `tax_id` to
11 digits. It refuses to run, naming the count, if any `tax_id` is neither shape: no route ever
stored one, so such a value is hand-edited data for a person to settle rather than for the
migration to guess at.

An invoice A needs the receiver's CUIT, which only the customer's registered account provides, so
a DNI never satisfies it (`RECEIVER_CUIT_REQUIRED`). An invoice B at or above
`INVOICE_RECEIVER_ID_THRESHOLD_AMOUNT` needs a CUIT or a DNI (`RECEIVER_ID_REQUIRED`), and an
invoice B below it goes to an unidentified consumidor final with no document at all. An invoice C,
which an issuer that is not IVA-registered bills to everyone, follows the same threshold rule as B.
While the
preview (`GET /v1/quotes/{quoteId}/invoice`) reports either receiver issue, it also carries
`customer_signup_url`: the supplier's customer sign-up page at `https://<public_hostname>/signup`,
or under `CUSTOMER_GOOGLE_LOCAL_ORIGIN` on a local stack. An account with no `public_hostname` gets
no link, because customer sign-up resolves the supplier by its hostname; the card then says the
customer site is not configured yet (see [public RFQ intake](public-rfq-intake.md)). With a link,
the invoice card offers a copy dialog so the seller can send it through their own channel. The
client detail also offers an explicit registration-link send through a configured contact channel.

Quotation publication cannot assign a CUIT; it creates provisional profiles or selects existing ones.
