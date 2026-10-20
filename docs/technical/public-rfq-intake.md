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
supply CUIT/CUIL before receiving a session. An existing CUIT/CUIL cannot be claimed through a
new Google subject: the customer must first authenticate to the existing account and explicitly
link Google. Completing registration and consuming its capability happen in one transaction.

For development, CUSTOMER_GOOGLE_LOCAL_ORIGIN may point to an HTTP localhost origin, for example
http://localhost:3001. It controls the callback and outgoing portal links and is rejected in
production. WEB_GOOGLE_FLOW_TTL_SECONDS bounds the browser flow cookie. API provider, signup,
recovery and verification timeouts are listed in apps/api/.env.example.

## Optional recovery email

Password registration can save an unverified recovery email and send a one-use verification link.
Verification enables recovery without gating portal access. Forgot password requires both CUIT/CUIL
and the matching stored verified address. It never sends a recovery link to a caller-selected new
address. Its response is identical for matching, unknown and unverified addresses.

VERIFY_EMAIL and RESET_PASSWORD capabilities have independent purposes and configurable expiration,
are stored hashed, and are consumed atomically. Resetting a password revokes existing sessions and
outstanding reset links in the same transaction. Link GETs do not consume capabilities. Google-only
customers can continue using their previously linked Google identity.

## History authorization and immutable quotations

The product's access rule is exact CUIT/CUIL matching within the authenticated supplier. Registration
therefore grants access to earlier matching requests. CUIT/CUIL is treated as a declared identifier;
this flow does not verify fiscal ownership. Queries match a request's associated commercial client
or the retained, normalized web declaration. They never infer identity from a loose name. Customer
credentials do not create, associate, merge or edit commercial client profiles.

GET /orders provides bounded pages; GET /orders/{rfqId} checks the same supplier and CUIT/CUIL
before returning the original request and a successful immutable delivery reference. Draft contents
and AI results are not returned. Until a quotation has been sent, the projection reports RECEIVED.
Sent versions remain available while the seller edits another version internally.

Every seller send creates a WEBAPP publication alongside the required WhatsApp attempt and any
selected email delivery. The seller's phone requirement remains unchanged. Publication commits
with delivery outcomes and the backend-controlled SENT transition; no browser push or notification
subscription flow is included. An external provider failure does not withdraw a web publication.
Publications do not duplicate seller messages in the conversation.

Legacy /quotes/{token} links first discover only public supplier routing through
GET /v1/public/quote-sends/{token}/site. A different hostname redirects to the registered supplier.
After login, the API checks authorization before resolving the immutable delivery. Both legacy
links and history open /orders/{rfqId}; the original delivery reference is retained in the URL so
an old link keeps its original expiration and version. The old unauthenticated quote read and
action endpoints return 401. A token alone no longer authorizes private content or actions.

Customer responses are shared across deliveries of the same frozen version, so a response from
WhatsApp and a visit from history show the same decision. Existing expiry, archive checks,
idempotency, quote locking and deterministic state transitions remain enforced.

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
