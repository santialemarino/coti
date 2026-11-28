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

## Send actions

Every send names its action as `mode` on `POST /v1/quotes/{id}/sends`; the backend enforces each
mode's state and media under the delivery lock.

| Mode           | Where                                    | Quote state | Media                                 | Expiry |
| -------------- | ---------------------------------------- | ----------- | ------------------------------------- | ------ |
| `FIRST`        | «Enviar cotización», bottom of the order | QUOTED      | all                                   | starts |
| `OTHER_MEDIUM` | «Enviar por otro medio», top card        | SENT        | only media this cycle has not reached | kept   |
| `RESEND`       | «Reenviar», top card                     | SENT        | all                                   | kept   |
| `FOLLOWUP`     | follow-up callout                        | SENT        | all                                   | kept   |

A **send cycle** starts each time the quote enters QUOTED: a reactivated quote re-enters it with the
same version, and what went out before no longer counts as used. A medium is used when a quote send
of the current version reached it in the current cycle (SENT, DELIVERED or VIEWED); failed attempts
and reminders never count. `OTHER_MEDIUM` through a used medium is **409** `CHANNEL_ALREADY_USED`.
Without `mode`, `followup_message` means `FOLLOWUP`, `other_channels_only` means `OTHER_MEDIUM`,
and otherwise the status decides between `FIRST` and `RESEND`; a mode contradicting those fields is
**400**.

A WhatsApp destination that has not written to the branch number within 24 hours gets the approved
utility template when one is configured, and otherwise the whole send is **422**
`WHATSAPP_WINDOW_CLOSED` before anything goes out (see
[WhatsApp Cloud API](whatsapp-cloud-api.md#customer-service-window)).

## Presets

`GET /v1/quotes/{id}/delivery-preset?mode=…&client_id=…` returns what the dialog preselects
(`phones`, `emails`), where it came from (`source`), and the destinations the current cycle reached
(`used_phones`, `used_emails`), which «otro medio» shows disabled.

| Mode                 | Preset, first that has destinations                                                                                                                                |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `FIRST`              | this version's earlier cycle (`VERSION`), else the quote's last sent version (`PREVIOUS_VERSION`), else the client's last sent quote in the branch (`CLIENT_LAST`) |
| `RESEND`, `FOLLOWUP` | everything this cycle reached (`VERSION`)                                                                                                                          |
| `OTHER_MEDIUM`       | none                                                                                                                                                               |

A client chosen in the dialog other than the quote's own never inherits the quote's destinations;
only its own last quote applies. With `NONE`, the dialog applies the WhatsApp-first rule: every
known contact is prefilled, WhatsApp alone is checked when it has a number, otherwise email when it
has an address, and nothing is checked without contact data. A checked medium never has an empty
field, and unchecked never means disabled. With a preset, exactly its destinations are selected and
a muted line says where they came from. Several values of one medium use a multi-select that also
accepts a new number or address; with more than one available, a hint says only the selected ones
receive it.

Each normalized destination has its own `quote_send` row, token and provider outcome. Up to twenty
destinations per medium may be selected. Reusing an operation key with changed destinations or
reminder text conflicts; retrying the same operation does not contact providers again.

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
