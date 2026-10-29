# Customer authentication, history and RFQ intake

## Supplier routing

The customer portal opens at the supplier's registered hostname, such as
https://supplier.coti.com.ar/. Migration 00051 adds nullable, unique account.public_hostname.
Provision DNS and TLS separately, preserve Host at the ingress, and set the account hostname
through database administration. No channel configuration screen is introduced in the backoffice.

Unknown or inactive suppliers return 404. Customer login remains available when there are no
active intake branches. The new-request page resolves active branches and WEBAPP destinations
through GET /v1/public/rfq-sites/{hostname}. Migration 00050 and branch creation provision
WEBAPP channels. WEB_PUBLIC_SITE_HOSTNAME is a server-only development override for Host;
leave it unset in a shared production deployment.

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
http://localhost:3001. It controls the callback and outgoing portal links and is rejected in
production. WEB_GOOGLE_FLOW_TTL_SECONDS bounds the browser flow cookie. API provider, signup,
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
return 409. The API generates RFQ IDs and acknowledges only durable reception. Text enters the
existing unassigned inbound pipeline. Files enter the existing attachment sweep. Provider failures
do not remove retained requests; failed storage is retryable without duplicating the RFQ.

Authorized seller detail includes public_contact, public_original_text and public_attachments.
Attachment URLs are freshly signed through the existing storage service. No customer data is
returned by public supplier metadata endpoints.

For proxy IP limits, configure WEB_TRUSTED_PROXY_HOPS on the webapp and separately trust the BFF
peer through RATE_LIMIT_TRUSTED_PROXY_CIDRS and RATE_LIMIT_TRUSTED_PROXY_HOPS on the API. Only
the trusted ingress hop's valid IP is forwarded. Without that setup, users share the BFF's allowance.
