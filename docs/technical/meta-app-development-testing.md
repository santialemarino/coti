# Meta App development access and phone testing

This guide lets Coti contributors test the shared WhatsApp Cloud API integration while the Meta
app remains in **Development** mode. It does not require a production phone number, WhatsApp
Business, an eSIM, business verification, or publishing the app.

It is for the shared development Meta app and the shared Coti development environment. It is not
an onboarding guide for customers or production operators.

## What Development mode permits

The Meta app may remain in Development mode for all team testing. The test phone number is owned
by Meta and can exchange messages only with phone numbers explicitly added as test recipients.
Each contributor tests from their own personal WhatsApp account after verifying that number in
Meta. A personal WhatsApp account is enough.

Development mode is therefore appropriate for the following loop:

1. A registered team phone sends a text to the Meta test number.
2. Coti creates a reviewable RFQ draft from that message.
3. A seller creates and sends a quote to that same registered phone.
4. The teammate opens the public quote link and can accept, reject, or request a change.

It is not appropriate for arbitrary customer numbers. Registering a production number, completing
business verification, publishing the app, and enabling access for real customers are intentionally
outside this development workflow.

## Roles and least-privilege access

There are three independent kinds of access. Grant only the one that a teammate needs.

| Activity                                                               | Coti account                      | Meta app role                   | Business portfolio asset access                | Secret access                    |
| ---------------------------------------------------------------------- | --------------------------------- | ------------------------------- | ---------------------------------------------- | -------------------------------- |
| Use Coti's backoffice and inspect RFQs                                 | Required                          | Not required                    | Not required                                   | Never                            |
| Send and receive test messages from an already registered phone        | Required for the backoffice steps | Not required after registration | Not required                                   | Never                            |
| Register or verify a personal test phone                               | Not required                      | Tester or higher                | Usually not required                           | Never                            |
| Inspect the Meta app and test setup                                    | Not required                      | Tester or Developer             | Not required                                   | Never                            |
| Configure webhooks, test recipients, or app settings                   | Not required                      | Developer or Administrator      | Required when Meta asks for the WhatsApp asset | Never                            |
| Manage people, assets, the WhatsApp Business Account, or app ownership | Not required                      | Administrator                   | Full control                                   | Never                            |
| Change Coti environment variables or rotate credentials                | Infrastructure access             | Administrator only when needed  | Full control when needed                       | Restricted infrastructure access |

Use **Tester** for a teammate who only needs to exercise the integration. Use **Developer** when
they must inspect or adjust the development configuration. Reserve **Administrator** and business
portfolio **Full control** for a small set of owners: those permissions can change assets, remove
people, and alter the app's configuration.

Giving someone access to the Meta app is not the same as giving them a Coti login. Create or invite
their Coti user separately when they need the backoffice.

## Add a teammate to the Meta app

An existing Meta app administrator performs these steps:

1. Open [Meta for Developers](https://developers.facebook.com/apps/), choose **Coti Development**,
   and open **App roles**.
2. Add the teammate's Facebook account or email as a **Tester**. Choose **Developer** only if they
   need to configure the development integration.
3. Ask them to accept the invitation while logged into that same Meta account. They may need to
   complete Meta's developer-account or security prompts first.
4. If they need to manage the WhatsApp asset, open **Meta Business Suite → Settings → People**,
   add them to the business portfolio, and assign only the necessary app and WhatsApp-account
   assets. Do not grant Full control merely to send test messages.

The labels in Meta's dashboard change frequently. Look for **App roles**, **People**, or **Assign
assets**; do not create a second Meta app or a second business portfolio for an ordinary teammate
test.

## Register a personal phone as a test recipient

Do this once for every phone that will send to or receive from the Meta test number:

1. The teammate opens **Coti Development → WhatsApp → API Setup** (the onboarding screen may call
   it **Step 1: Try it**).
2. Under the test number, choose the option to add a recipient or test phone number.
3. Enter their personal WhatsApp number in international format, for example `+54...`.
4. Enter the verification code Meta sends to that WhatsApp account.
5. Confirm that the number now appears in Meta's test-recipient list.

Use the same registered phone for both directions of the test. Do not enter an actual customer,
supplier, or employee who has not agreed to be a development tester. Meta may impose a recipient
limit on the test number; remove an inactive tester before adding another one when that happens.

No team member needs to give Coti access to their personal WhatsApp account. The verification code
only proves that they control the phone number; Coti communicates with Meta's test number through
the Cloud API.

## Shared development endpoint

The Meta app has one callback URL for the shared app. It must point to a publicly reachable Coti
development API:

```text
https://<shared-development-api>/v1/webhooks/whatsapp
```

Do not replace that callback with a personal localhost URL or a personal tunnel while other people
are testing. Coordinate with the team if the shared public endpoint must change, then update Meta
and the corresponding development environment together. The callback verifies with the shared
verify token and signs inbound events with the app secret; neither value belongs in this document,
a chat, a ticket, or a screenshot.

The Coti API must be running with its WhatsApp configuration, its AI provider keys, and the branch
channel configured before testing. See [WhatsApp Cloud API](whatsapp-cloud-api.md) for the service
configuration and callback contract.

## End-to-end test procedure

### 1. Test inbound WhatsApp to RFQ

1. From a registered test phone, send a simple text such as `Necesito 10 bolsas de cemento
Avellaneda 50 kg` to the Meta test number.
2. Wait for the webhook delivery to reach the shared development API.
3. Open Coti's backoffice and verify that the designated branch received a new reviewable RFQ.
4. Confirm that the requested materials are present as AI proposals. The seller remains responsible
   for validating catalog matches, quantities, and prices.

Only inbound **text** messages create RFQs in this release. Images, audio, documents, and delivery
status events are acknowledged but do not create an RFQ through the Meta connector yet.

### 2. Test outbound quote delivery

1. In the backoffice, finish the RFQ review, accept materials, and generate a quote.
2. In the send dialog, choose WhatsApp and use the registered tester's phone number as the
   recipient. For an RFQ that came from WhatsApp, Coti pre-fills that sender's number.
3. Send the quote and verify that the phone receives a text with the public quote link.
4. Open the link and verify that the quote details render.

The number must be registered in Meta's test-recipient allow-list. A failed delivery with Meta code
`131030` usually means that the number is not registered there. Coti retries the Argentine mobile
representation once in development; do not alter stored phone numbers by hand to work around it.

### 3. Test a customer-requested change

1. Use a **newly sent** public quote link. A public action is idempotent, so a link whose change
   request was already submitted cannot be used to create another one.
2. Choose the public change-request action and send, for example, `Agregar 5 bolsas de cal
hidratada 25 kg`.
3. Reload the quote in Coti. It moves to `CHANGE_REQUESTED` and opens a mutable next version.
4. Verify that the draft contains `cal hidratada 25 kg`, quantity `5`, and unit `bolsa`.
5. Review the catalog match, accept materials to obtain current deterministic prices, then send a
   new version only after seller review.

The change handler can add, remove, replace, and change the quantity of materials. It records a
seller-reviewable proposal; it never prices the change, approves it, or contacts the customer on
its own. An unclear request creates the review state without inventing a material change.

## Credentials and configuration

Treat every value below as a secret unless it is explicitly marked as an identifier:

| Value                                                      | Classification | Who may use it                           |
| ---------------------------------------------------------- | -------------- | ---------------------------------------- |
| Meta App ID, phone number ID, WhatsApp Business Account ID | Identifiers    | Developers who configure the integration |
| App Secret                                                 | Secret         | Restricted infrastructure owners only    |
| Access token                                               | Secret         | Restricted infrastructure owners only    |
| Webhook verify token                                       | Secret         | Restricted infrastructure owners only    |
| `CHANNEL_CONFIG_ENCRYPTION_KEY`                            | Secret         | Restricted infrastructure owners only    |

Never put a secret in Git, a pull-request description, Notion, a screenshot, browser code, or a
frontend environment variable. Contributors who only test the flow do not need any of these
values. If a token is pasted into an unsafe location, revoke and replace it in Meta before further
testing.

## Pull request scope

This development integration pull request includes:

- Meta-signed WhatsApp webhook verification and idempotent inbound text intake.
- Branch routing by Meta `phone_number_id`, preserving the sender context for quote delivery.
- Outbound WhatsApp Cloud API quote links using encrypted, branch-specific channel credentials.
- Development test-recipient support, including the Argentine mobile-number retry for Meta's
  recipient allow-list behavior.
- AI-assisted interpretation of public quote change requests into reviewable add, remove, replace,
  and quantity-update draft operations.
- Audit persistence for the AI change proposal, usage metering, database migrations, automated
  tests, and the technical documentation needed to operate the feature.

This pull request deliberately excludes:

- Publishing the Meta app, registering a production number, business verification, billing, or
  communication with arbitrary real customers.
- A backoffice screen for editing WhatsApp channel credentials; configuration remains the guarded
  administrator API.
- Inbound media extraction through Meta, message-template management, a shared Meta inbox, and
  delivery-status processing beyond acknowledgement.
- Autonomous customer communication, autonomous quote approval, and AI pricing. Seller review and
  the deterministic pricing flow remain mandatory.

## Troubleshooting

| Symptom                                                       | Check first                                                                                                                                                             |
| ------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The teammate cannot see the app                               | Confirm that they accepted the Meta app-role invitation with the intended Facebook account.                                                                             |
| Meta cannot send to the tester's phone                        | Register and verify that exact phone in the test-recipient list.                                                                                                        |
| A WhatsApp message creates no RFQ                             | Confirm the shared API is running, the callback URL is public and unchanged, `messages` is subscribed, and the branch channel uses the test number's `phone_number_id`. |
| Meta rejects callback verification                            | Verify the callback URL and verify token against the shared environment. Do not disclose the token while debugging.                                                     |
| The quote shows no matched product after a requested addition | Ensure that the product is active in the destination branch catalog. A `NO_MATCH` line is retained for seller resolution; it is not discarded.                          |
| A second change request does nothing                          | Use a new quote delivery link; the original public action is intentionally idempotent.                                                                                  |
