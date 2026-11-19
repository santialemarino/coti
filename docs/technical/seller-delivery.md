# Seller delivery and follow-up

Approval and initial sending appear at the bottom of the order. Copying, sending through another
medium, customer outcomes and reactivation share the upper action row. The original request is
collapsed on every channel and lifecycle state; images and audio open inside that section.
Structured manual orders retain their entered lines as source text. Legacy requests without
retained content show an unavailable state.

## Recipients

The seller selects a client once. Creating a provisional client uses a separate dialog prefilled
from the inbound name and contact; publication creates the proposed profile in the existing send
transaction. Portal orders retain their authenticated customer association. Active contacts are
batch-loaded with the directory and association context.

Every available contact is prefilled. WhatsApp alone is selected initially when it has a number;
otherwise email is selected if present, and neither is selected without contact data. Multiple
contacts of one kind use a multi-select, initially selecting the primary contact.

`POST /v1/quotes/{id}/sends` accepts `recipient_phones` and `recipient_emails` arrays and retains the
legacy singular fields. Each normalized destination has its own `quote_send` row, token and provider
outcome. Up to twenty destinations per medium may be selected. Reusing an operation key with changed
destinations or reminder text conflicts; retrying the same operation does not contact providers again.

The other-medium action sets `other_channels_only`. Every successfully used medium of the current
version is disabled in the UI and checked again under the delivery lock before contacting providers.
Failed attempts, earlier versions and follow-up messages do not consume that option.

## Reminders

`POST /v1/quotes/{id}/followup-draft` renders an account template without AI, persistence or delivery.
Only an unarchived `SENT` quote is eligible. The response includes `message` and
`template_configured`. The account administrator edits `followup_template` below the AI preference
in Account. Null means it has never been customized; unrelated account edits preserve null.
Supported parameters are `{cliente}` and `{cotizacion}`. Once a template is saved, the reminder
dialog no longer offers its discovery link, even if the saved text equals the default.

The seller reviews the rendered message, optionally edits it for that order, and confirms its
recipients. Successful recipients from the original quote operation are preselected. Delivery uses
`followup_message` on the existing sends endpoint. `quote_send.purpose` distinguishes `QUOTE` and
`FOLLOWUP`; `message` retains the confirmed reminder for replay validation and conversation history.
A reminder creates no new version or portal publication, does not rerun evaluation, and retains
the original expiry. A successful delivery counts as activity and clears the follow-up flag.

## Failed requests

`PUT /v1/rfqs/{id}/archive` with an `archived` boolean handles a `FAILED` source without a quote.
Account, selected branch and seller reach are checked before changing `rfq.archived_at`. Generated
quotes retain their existing archive endpoints. The list combines both flags. Archived requests
appear last in the sidebar as a count, hidden until Show is pressed. Restoring retains the error state.

Migration 00080 adds these fields, expands operation uniqueness to include the destination and repairs
missing portal associations using persisted authenticated authorship within the same account.
An unverified contact declaration alone never claims a profile. Rollback refuses incompatible data
if an operation already includes several destinations of one channel.
