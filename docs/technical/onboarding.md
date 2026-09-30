# Account onboarding

Onboarding is an account-scoped, admin-only setup flow shown after the first verified sign-in.
Registration remains atomic and limited to the account, first branch, manual-entry channel, first
administrator, and the onboarding record. Setup is a separate resumable concern.

## Lifecycle and API

`account_onboarding` stores the flow version, lifecycle status, and stable key of the screen to
resume. `onboarding_step_progress` records whether each visited step was completed or skipped.
Both tables carry `account_id`, are protected by RLS, and every repository query scopes by that
identifier.

The lifecycle is `IN_PROGRESS`, `COMPLETED`, or `DISMISSED`. Dismissal is not completion: it lets
the administrator enter the backoffice without losing the resume point. Existing accounts are
seeded as dismissed so deploying the feature does not interrupt their work.

The admin-only API is:

| Route                          | Purpose                                      |
| ------------------------------ | -------------------------------------------- |
| `GET /v1/onboarding`           | Read lifecycle, resume point, and step state |
| `PUT /v1/onboarding`           | Resolve one step and store the next one      |
| `POST /v1/onboarding/complete` | Finish setup                                 |
| `POST /v1/onboarding/dismiss`  | Leave setup without blocking the account     |
| `POST /v1/onboarding/resume`   | Continue a dismissed setup                   |

## Current flow

The current registry uses stable keys rather than numeric positions: welcome, brand, first branch,
catalog upload, catalog review, team, and completion. The progress bar is derived from the registry,
so inserting another screen requires adding its stable key and copy instead of changing persisted
positions. `flow_version` is available for future migrations when a new flow cannot safely share
the existing order.

The brand screen does not ask for the account data collected at registration. It shows that data as
read-only context and updates the optional brand colour and logo. A chosen logo is uploaded through
the same account-logo path Cuenta uses, and its stored URL is saved on the account with the colour.

The first-branch screen edits the branch already created during registration. Default expiry days are
the suggested validity period for new quotes, not a branch expiry date, and remain editable per quote.

Catalog upload uses the production preview and confirmation endpoints. Nothing is written during
preview, and invalid rows are never silently imported. The same import component remains available at
`/settings/catalog`, so dismissing onboarding cannot make the initial catalog operation unreachable.

The team screen reuses account user creation. It creates a real user with an initial password; there
is no invitation token or invitation email. The UI states that the administrator must share the
password securely instead of presenting the operation as an invitation. The administrator is listed
first, as themselves, and finishing with nobody else on the account resolves the step as skipped.

The completion screen reports each step as done only when it was completed; a skipped one names the
settings page where it is waiting. While onboarding is open, the protected shell sends the
administrator back to it only when the account has an active branch: onboarding sends an account
without one to Sucursales, and bouncing it back from there would loop.

User preferences are intentionally absent from version 1. They can be introduced as another stable
step without coupling them to registration or rewriting existing progress.
