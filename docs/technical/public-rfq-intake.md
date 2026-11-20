# Customer authentication, history and RFQ intake

## Supplier routing

Every supplier's portal lives under one shared origin, at `<origin>/c/<label>`, for example
https://portal.coti.com.ar/c/corralon-san-martin. The label lives inside the unique
`account.public_hostname` as `<label>.<base-host>`, an internal routing identifier that needs no DNS
record. The API builds it from CUSTOMER_PORTAL_BASE_URL; the webapp resolves the same identifier
from the path's label and WEB_PORTAL_BASE_URL, so the two must name the same origin.

**The supplier comes from the path, never from the Host header.** The proxy (`lib/portal/proxy.ts`)
rewrites `/c/<label>/…` to the route and sets `x-coti-portal-slug`, replacing any value a caller sent.
Host cannot be trusted for this: when a server action redirects (sign-in, sign-up, sign-out), Next
re-renders the target through an internal request to its own origin, which carries the server's
host rather than the customer's.

**Every account has a portal from the day it registers.** Registration derives a default label from
the account name — lowercased, accents folded (`ñ` → `n`, `á` → `a`), anything else a hyphen,
capped at 63 characters — and takes `-2`, `-3`… on a collision. A name that leaves nothing usable or
only a reserved word becomes `corralon-<first 6 characters of the account id>`. The onboarding
«portal» step and Account settings show that label prefilled and still editable. Accounts created
before this, or holding the bare base host, get theirs from `cmd/portal-backfill` (see
deployment.md).

**Nothing outside `/c/<label>` serves a page.** The portal root and every other path answer a 307 to
WEB_LANDING_URL, never a not-found page. The exceptions are the entry points that carry their own
supplier: `/quotes/<token>` (delivery links already sent, which resolve the supplier by token and
redirect into its portal), the webapp's own `/api/*` routes (the shared Google callback restores the
label from its state), and Next's assets, icons, robots and manifest, which the proxy never sees. An
unknown but well-formed `/c/<label>` reaches the branded not-found page; a malformed one is a bare 404.

Links the API sends all point inside a portal: quotation links `/c/<label>/quotes/<token>`, signup
`/c/<label>/signup`, contact verification `/c/<label>/verify-contact?challenge=…&token=…`, the
registration link `/c/<label>/login?next=/profile`, and the Google callback
`<origin>/api/customer/google/callback`. The backoffice's RFQ detail still shows the legacy
`<WEB_WEBAPP_URL>/quotes/<token>` form, which the exception above keeps working.

Unknown or inactive suppliers return 404. Customer login remains available when there are no
active intake branches. The new-request page resolves active branches and WEBAPP destinations
through GET /v1/public/rfq-sites/{hostname}. Migration 00050 and branch creation provision
WEBAPP channels. Locally the shared origin is http://localhost:3001 and a supplier whose hostname is
`sanmartin.localhost` opens at `http://localhost:3001/c/sanmartin/login`.

## Customer credentials and sessions

Migration 00052 creates customer_identity, customer_session and customer_auth_challenge with
account-scoped row-level security. A customer identity is separate from app_user and from the
commercial client profile. Active CUIT/CUIL and Google subject are unique within one supplier.
Identifiers are normalized and checksum-validated.

Passwords reuse the backend's bcrypt implementation and password policy. An opaque random
32-byte session secret is stored only as its SHA-256 hash. The webapp stores the raw secret in a
host-only HttpOnly, SameSite=Lax cookie, with Secure and the \_\_Host- prefix in production. The
absolute expiration comes from the API; CUSTOMER_SESSION_TTL_DAYS defaults to 30. The API
checks supplier, expiration, revocation and disabled identity on each customer request. Staff
JWTs and customer sessions are separate; neither establishes the other's permissions.

Customer endpoints live under /v1/customer/sites/{hostname}. Registration starts a session;
login verifies credentials and rechecks them under a row lock before issuing a session. Failed
attempt counters and lockout settings are independent of staff authentication. Logout revokes
the persisted session. The webapp uses server actions for credential forms and forwards the
customer token server-side only. Mutation proxies reject mismatched Origin and cross-site fetches.

## Google sign-in

The external adapter lives in internal/customeridentity and is wired only in cmd/api. Configure
CUSTOMER_GOOGLE_CLIENT_ID and CUSTOMER_GOOGLE_CLIENT_SECRET together on the API. Register
the exact redirect URI https://{supplier-hostname}/api/customer/google/callback for each supplier
in the Google OAuth client. Google sign-in is unavailable until these credentials are configured.

The webapp uses an authorization-code flow with random state, nonce and PKCE S256. A short-lived
host-only HttpOnly cookie binds the callback to that browser and its allowed local destination.
The API exchanges the one-use code and verifies the provider signature, issuer, audience,
expiration and nonce through go-oidc. Google access tokens are discarded. Accounts are linked by
the stable Google subject, never by a matching email address.

An unknown subject receives a hashed, short-lived GOOGLE_SIGNUP capability. The customer must
supply CUIT/CUIL, a full name and at least one contact before receiving a session; the Google
address counts only when the customer chooses it (see [Client profiles](client-profiles.md#google-sign-up)). An existing CUIT/CUIL cannot be claimed through a
new Google subject: the customer must first authenticate to the existing account and explicitly
link Google. Completing registration and consuming its capability happen in one transaction.

For development, CUSTOMER_GOOGLE_LOCAL_ORIGIN may point to an HTTP localhost origin, for example
http://localhost:3001. Register `http://localhost:3001/api/customer/google/callback` with Google.
Local sign-in retains state, nonce and PKCE in the starting supplier's host-only cookie. The shared
localhost callback relays the provider response to that supplier's callback, which verifies the
original cookie before exchanging the code or setting any credential cookie. The relay accepts
only HTTP single-label `*.localhost` origins on the same port and is disabled in production.
Development Host handling preserves the supplier when Next normalizes the request URL to localhost.
Outgoing customer, verification and quotation links use the registered `*.localhost` supplier with
the local origin's port. This override is rejected in production, which uses each registered HTTPS
hostname and its exact Google redirect URI. WEB_GOOGLE_FLOW_TTL_SECONDS bounds the browser flow cookie. API provider, signup,
recovery and verification timeouts are listed in apps/api/.env.example.

## Account recovery

A forgotten password is recovered with a one-use code sent to one of the account's verified
contacts; there is no separate recovery address. Resetting the password revokes existing sessions in
the same transaction. Google-only customers can keep using their linked Google identity, or gain a
password the same way. See [Client profiles](client-profiles.md#account-recovery).

WhatsApp and a visit from history show the same decision. Existing expiry, archive checks,
idempotency, quote locking and deterministic state transitions remain enforced.

The authenticated `/profile` page manages optional commercial contact collections through
`GET`/`PUT /v1/customer/sites/{hostname}/profile` and the main contact through
`PUT /v1/customer/sites/{hostname}/profile/main-contact`. It never modifies passwords or Google links. See [Client profiles](client-profiles.md) for intake provenance,
uniqueness, primary-contact compatibility and proof-gated history transfers.

## Request intake

Authenticated POST /rfq-channels/{channelId}/requests accepts JSON text or multipart fields and
one file. The server verifies the branch belongs to the customer's supplier and requires the
request CUIT/CUIL to equal the authenticated identifier. Name is required; email and phone are
optional. Text and file are exclusive. Existing RFQ and attachment format and size limits apply.
The old unauthenticated request POST returns 401.

A UUID v4 request_id identifies retries of the same payload. A durable inbound_channel_message
reservation and payload fingerprint serialize retries under an advisory lock. Changed payloads
return 409. The API generates RFQ IDs and acknowledges only durable reception. The authenticated
customer profile is associated with the retained RFQ before processing or file
storage starts. Generated quotes inherit that profile, and retries repair missing associations
without replacing an existing seller selection or changing contact ownership. Text enters the
existing unassigned inbound pipeline. Files enter the existing attachment sweep. Provider failures
do not remove retained requests; failed storage is retryable without duplicating the RFQ.

Authorized seller detail includes public_contact and public_attachments; original_text carries the order as received on every channel.
Attachment URLs are freshly signed through the existing storage service. No customer data is
returned by public supplier metadata endpoints.

For proxy IP limits, configure WEB_TRUSTED_PROXY_HOPS on the webapp and separately trust the BFF
peer through RATE_LIMIT_TRUSTED_PROXY_CIDRS and RATE_LIMIT_TRUSTED_PROXY_HOPS on the API. Only
the trusted ingress hop's valid IP is forwarded. Without that setup, users share the BFF's allowance.

## Self-service portal addresses

The administrator-only endpoints are GET /v1/account/portal, GET
/v1/account/portal/availability?subdomain=label, and PUT /v1/account/portal. The last accepts
subdomain, expected_hostname, and confirm_change; the server builds the hostname from
CUSTOMER_PORTAL_BASE_URL. Availability reveals only a boolean, includes inactive accounts in the
reservation check, and does not reserve a name. The existing unique public_hostname index arbitrates
concurrent saves. A stale expected_hostname or a unique collision returns 409 without changing the
account; the browser preserves the input. Reads and writes are account-scoped under RLS; availability
is an explicit cross-account public routing discovery operation.

Labels are trimmed, lowercased, limited to one DNS label of at most 63 ASCII letters, digits and
internal hyphens. Infrastructure names including www, api, admin, auth, app, backoffice, portal,
mail, smtp, cdn, static, docs, dev, staging, status, support, coti and localhost are reserved.

Changing a managed hostname requires explicit acknowledgement. The old hostname stops resolving
this supplier; old links must be resent with the new origin. Quote tokens, customer credentials,
request history and stored sessions are retained in the same account. Host-only cookies do not move
to the new hostname: customers sign in again. An old cookie never authenticates another supplier,
even if the freed name is subsequently claimed, because sessions are validated within the resolved
account. There is no automatic alias, redirect or cross-domain session transfer. Any pending Google
OAuth or contact verification flow must be restarted; each new production Google callback URI must
be registered separately before using Google login. The editor warns before committing this change.
An inherited hostname outside the configured base cannot be replaced through this endpoint; migrate
its links, callback registration and customer access in a separately coordinated operation.

Outgoing signup, contact verification and quotation links resolve the supplier hostname. RFQ detail
uses that hostname too. WEB_WEBAPP_URL remains a compatibility fallback only for services instantiated
without supplier routing; the API wires supplier routing and never directs a supplier without a
hostname to another account's shared portal. Customer sessions stay separate from staff JWTs.
In development, a registered bare `localhost` hostname keeps HTTP and the configured local port.
Supplier `.localhost` hostnames use `/c/<supplier>` on the configured shared origin. The webapp
preserves that supplier prefix when an old quotation link redirects to history or contact guidance.

Authenticated request authors are associated with their commercial profile before processing.
Migration 00080 restores missing links only from persisted customer authorship within the same
account. An unverified contact declaration alone never claims a profile.
