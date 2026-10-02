# Account onboarding

Onboarding is an account-scoped, admin-only setup flow shown after the first verified sign-in.
Registration remains atomic and limited to the account, first branch and its channels, first
administrator, and the onboarding record. Setup is a separate resumable concern.

## Lifecycle and API

`account_onboarding` stores the flow version, lifecycle status, and stable key of the screen to
resume. `onboarding_step_progress` records whether each visited step was completed or skipped.
Both tables carry `account_id`, are protected by RLS, and every repository query scopes by that
identifier.

The lifecycle is `IN_PROGRESS`, `COMPLETED`, or `DISMISSED`. Dismissal is not completion: it lets
the administrator enter the backoffice without losing the resume point. Existing accounts are
seeded as dismissed so deploying the feature does not interrupt their work, and the checklist
migration records every step their data already proves (see below).

The admin-only API is:

| Route                                | Purpose                                        |
| ------------------------------------ | ---------------------------------------------- |
| `GET /v1/onboarding`                 | Read lifecycle, resume point, and step state   |
| `PUT /v1/onboarding`                 | Resolve one step and store the next one        |
| `POST /v1/onboarding/complete`       | Finish setup                                   |
| `POST /v1/onboarding/dismiss`        | Leave setup without blocking the account       |
| `POST /v1/onboarding/resume`         | Continue a dismissed setup                     |
| `POST /v1/onboarding/checklist/hide` | Take the checklist card off the home screen    |
| `POST /v1/onboarding/checklist/show` | Put the checklist card back on the home screen |

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

The team screen reuses account user creation, with the same two ways in: an invitation by mail, the
default, or an initial password the administrator shares — the only way offered while mail only
reaches the log (see [authentication.md](authentication.md#invitations)). The administrator is listed
first, as themselves, and finishing with nobody else on the account resolves the step as skipped.

The completion screen reports each step as done only when it was completed; a skipped one names the
settings page where it is waiting. While onboarding is open, the protected shell sends the
administrator back to it only when the account has an active branch: onboarding sends an account
without one to Sucursales, and bouncing it back from there would loop.

## Checklist

Once the wizard is closed — dismissed, or finished with steps skipped — what is left of the setup
is a checklist of three steps: brand, catalog (`CATALOG_UPLOAD`) and team. The first branch is not
on it: registration creates that branch with its name and address, so the step only confirms data
that always exists. A skipped step is pending; only real completion counts.

Completion is read from the account's own data as well as from the wizard. `GET /v1/onboarding`
checks a brand logo or colour, an active product, and a second active user, and records each proven
step as `COMPLETED` before answering, so setup done from Configuración counts the same and stays
done if the data later changes. A dismissed onboarding with nothing left pending becomes
`COMPLETED`. The wizard's own `IN_PROGRESS` state is never changed by this.

The checklist appears in two places for an administrator while a step is pending: a card in the home
screen's empty detail pane, and a "Configuración inicial" entry in the settings rail that opens the
same checklist. "No mostrar más" stamps `account_onboarding.checklist_hidden_at` and hides only the
card; the rail entry stays as the place to see it again or bring the card back, and disappears when
every step is done. Each item links to the settings screen that does the work; "Retomar asistente"
is offered only for a dismissed wizard.

User preferences are intentionally absent from version 1. They can be introduced as another stable
step without coupling them to registration or rewriting existing progress.
