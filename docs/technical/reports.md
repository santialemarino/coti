# Seller reports

The backoffice reports page reads `GET /v1/reports`. It is an audited summary of the RFQs and
quotes the caller can already see, not a second source of truth: every number is derived from the
same rows the inbox and quote screens use.

## Period and scope

`from` and `to` are inclusive Argentine calendar dates, resolved in
`America/Argentina/Buenos_Aires`. The repository receives a half-open UTC interval
`[start_at, end_at)`, so a day never leaks into the next one through a timezone conversion.

Both dates omitted means the last 30 days, including today. Supplying only one, a malformed date,
`from > to`, or a period wider than `REPORTS_MAX_RANGE_DAYS` returns `400`. The response echoes the
resolved `range` so the page can seed its form from the server, including the default.

The active branch comes from `X-Branch-Id`, never from a query parameter. It is validated against
the caller's authorized branches. A seller also sees only quotes assigned to them or still
unassigned, and only within their branch reach; an admin with no branch selected sees every branch
and receives the `branches` breakdown. The report never widens the read beyond the visibility the
rest of the API enforces.

## What each section counts

- `kpis.rfqs_received`: RFQs received in the period.
- `kpis.quotes_sent`: first `SENT` event per quote inside the period.
- `kpis.quotes_accepted`: latest distinct `ACCEPTED` event per quote inside the period.
- `kpis.conversion_rate`: accepted over sent events in the period. Because the two are event counts,
  a quote sent before the period and accepted inside it can make this ratio exceed `1`. The UI shows
  it as an event ratio, not a cohort percentage.
- `kpis.accepted_amount`: sum of the commercial totals of the versions that were actually accepted.
  See below.
- `kpis.response_time`: first send minus RFQ arrival, for RFQs received in the period. Manual-entry
  channels are excluded because there was no inbound message to answer.
- `quotes_by_status`: quotes created in the period, grouped by their current status.
- `timeline`: one point per calendar day, with sent and accepted events. Days with no activity are
  present with zeroes.
- `top_products`: the five most requested products across each visible RFQ's current quote version.
  Matched products use the canonical catalog name; unmatched lines use the requested description.
- `top_clients`: the five clients with the largest accepted amount in the period. Anonymous quotes
  and zero-amount groups are omitted.
- `ai_match`: the latest AI generation per quote, its matched-line rate, and the confidence split of
  scored lines.

## Which version an acceptance refers to

A `quote_status_change` to `ACCEPTED` does not carry the version the client saw. The amount comes
from the first of these that resolves:

1. a customer `client_action` of type `ACCEPT` recorded within five minutes of the transition;
2. the latest successfully sent version that existed at acceptance time;
3. `quote.current_version_id`, but only when no version was created after the acceptance.

The fallback is deliberately narrow. A later version is not charged to the client, and an
acceptance that cannot be tied to a version contributes to the accepted count but zero to the
amount. The same resolution feeds the accepted amount and the top clients.

## AI confidence

The latest generation is taken per quote. Its items are split into the same levels the review
screen uses: `LOW` for `NO_MATCH`, `MEDIUM` for `AMBIGUOUS` or a `MATCHED` line below
`CATALOG_MATCH_HIGH_CONFIDENCE_PERCENT`, and `HIGH` for `MATCHED` at or above it. Lines with no
score, such as a product a person chose manually, are not counted in any bucket. The matched rate
is matched lines over the generation's total lines.

## Configuration and indexes

`REPORTS_MAX_RANGE_DAYS` defaults to `366` and is capped by the code at `3660`. The high-confidence
threshold is the catalog setting, passed from the API composition root, so changing catalog
calibration moves the report with the review screen.

The report reads are backed by `00059_seller_reports.sql`: branch/time indexes for RFQs and quotes,
partial status and send indexes for the event reads, a partial client-action index for acceptance
resolution, and a generation index for the AI summary. They are in the consolidated reference
schema in `apps/api/database/01_create_tables.sql` as well.
