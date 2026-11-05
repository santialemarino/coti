# Seller reports

The backoffice reports page reads `GET /v1/reports` and downloads the same report from
`GET /v1/reports/pdf`. It is an audited summary of the RFQs and quotes the caller can already see,
not a second source of truth: every number is derived from the same rows the inbox and quote
screens use, and the PDF is rendered from that same read model.

## Period and scope

`from` and `to` are inclusive Argentine calendar dates, resolved in
`America/Argentina/Buenos_Aires`. The repository receives a half-open UTC interval
`[start_at, end_at)`, so a day never leaks into the next one through a timezone conversion.

Both dates omitted means the last 30 days, including today. Supplying only one, a malformed date,
`from > to`, or a period wider than `REPORTS_MAX_RANGE_DAYS` returns `422` with code
`INVALID_INPUT`. The response echoes the resolved `range` so the page can seed its form from the
server, including the default.

The active branch comes from `X-Branch-Id`, validated against the caller's authorized branches. A
seller also sees only quotes assigned to them or still unassigned, and only within their branch
reach. With no branch selected, a report that spans several branches — an admin's whole account, or
a seller assigned to more than one — receives the `branches` breakdown, limited to the reachable
branches; an inactive branch appears only if it had orders. The report never widens the read beyond
the visibility the rest of the API enforces.

## Filters

Every filter narrows every section, the branch breakdown and the PDF alike, and they combine with
AND. Lists repeat the parameter or comma-separate it; any malformed value is `422 INVALID_INPUT`.

- `status`: the quote's current status, any of the listed ones.
- `channel`: the intake channel type of the order.
- `seller_id`: administrators only; a seller's own reach already pins it, so the parameter is ignored.
- `client_id`: the order's client.
- `tag_id`: orders whose client carries any of the listed tags.
- `branch_id`: narrows the reach to the listed branches. It is intersected with what the caller can
  reach, so a branch outside it reads nothing rather than widening the report; picking one branch
  makes the report single-branch, picking several keeps the breakdown to those. With an active
  branch (`X-Branch-Id`) the parameter is ignored: the reach is already that one branch.

The response echoes the applied narrowing under `filters`, with display names for the seller, the
client, and each branch and tag, so the page and the PDF can say what they are showing.

## What each section counts

- `kpis.rfqs_received`: RFQs received in the period.
- `kpis.quotes_sent`: first `SENT` event per quote inside the period.
- `kpis.quotes_accepted`: latest distinct `ACCEPTED` event per quote inside the period.
- `kpis.sent_accepted`: of the quotes first sent in the period, how many were accepted since, up to
  the moment of the read.
- `kpis.conversion_rate`: `sent_accepted` over `quotes_sent`, a cohort share between `0` and `1`.
  An acceptance in the period of a quote sent before it counts in `quotes_accepted` but not here, so
  the rate never exceeds `1`. It is `null` when nothing was sent.
- `kpis.accepted_amount`: sum of the commercial totals of the versions that were actually accepted.
  See below.
- `kpis.response_time`: first send minus RFQ arrival, for RFQs received in the period. Manual-entry
  channels are excluded because there was no inbound message to answer.
- `quotes_by_status`: quotes created in the period, grouped by their current status.
- `timeline`: one point per calendar day, with sent and accepted events. Days with no activity are
  present with zeroes.
- `top_products`: the five most requested products across each visible RFQ's current quote version,
  ranked by line occurrences. Every quote line counts once, regardless of its quantity. Matched
  products use the canonical catalog name; unmatched lines use the requested description, grouped
  ignoring case and surrounding spaces. Every ranking breaks ties by name, then by id, so the same
  data always yields the same order.
- `top_revenue_products`: the five matched products with the largest accepted line revenue in the
  period. Revenue is `quantity × unit_price_snapshot` summed across the lines of the resolved
  accepted version, rounded to two decimals. Unmatched lines are excluded because they have no
  product identity.
- `top_clients`: the five clients with the largest accepted amount in the period, one row per client.
  A client with no name is shown by its email, then its phone, then the order's label. Quotes with no
  client group by the order's label; anonymous quotes and zero-amount groups are omitted.
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
amount. The same resolution feeds the accepted amount, the top clients and the product revenue
ranking.

Product revenue is gross line value and subtracts no discounts: the ranking deliberately reports
`quantity × unit_price_snapshot` even when an `ITEM` discount names a single line, because a
`TOTAL` or `ITEM_SET` discount cannot be attributed to one product without inventing an allocation
rule. This means `top_revenue_products` does not sum to `kpis.accepted_amount` when the accepted
version carries discounts, and it excludes unmatched lines.

## AI confidence

The latest generation is taken per quote. Its items are split into the same levels the review
screen uses: `LOW` for `NO_MATCH`, `MEDIUM` for `AMBIGUOUS` or a `MATCHED` line below
`CATALOG_MATCH_HIGH_CONFIDENCE_PERCENT`, and `HIGH` for `MATCHED` at or above it. Lines with no
score, such as a product a person chose manually, are not counted in any bucket. The matched rate
is matched lines over the generation's total lines.

## PDF export

The page's "Descargar PDF" action calls the backoffice proxy `GET /api/reports/pdf`, which forwards
to `GET /v1/reports/pdf` with the same period, filters and branch scope as the page read. The proxy
deliberately does not fall back to an account's only branch: an admin viewing every branch must get
an all-branch PDF, matching the report itself. The endpoint reuses `SellerReportService.Get`, so
authorization, period validation, seller visibility and every metric are identical to the JSON
response. The filename is always `reporte-<from>-<to>.pdf`, with the defaulted period when none
was asked; the response is `application/pdf` and is never stored by an intermediary.

`SellerReportRenderer` renders the server-provided `domain.SellerReport`; it never queries the
database and never recomputes a metric. The document includes the Coti mark, period, generation
timestamp and scope, the six headline KPIs, the status distribution and timeline, both product
rankings, top clients, AI match confidence, the branch breakdown when the report spans branches,
and the metric definitions that could otherwise be misread. Amounts are shown in Argentine pesos,
matching the dashboard. Empty sections say so rather than printing fabricated zeroes.

## Configuration and indexes

`REPORTS_MAX_RANGE_DAYS` defaults to `366` and is capped by the code at `3660`. The high-confidence
threshold is the catalog setting, passed from the API composition root, so changing catalog
calibration moves the report with the review screen.

The report reads are backed by `00072_seller_reports.sql`: a branch/time index for RFQs, an account/time
index for quotes,
partial status and send indexes for the event reads, a partial client-action index for acceptance
resolution, and a generation index for the AI summary. They are in the consolidated reference
schema in `apps/api/database/01_create_tables.sql` as well.
