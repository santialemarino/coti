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

### Previous-period comparison

The additive `comparison` response contains the immediately preceding interval of equal calendar
day length in `America/Argentina/Buenos_Aires`. An inclusive November 10–16 request compares
against November 3–9. Query bounds are start-inclusive and end-exclusive. All date presets and
custom ranges use this same rule; exact dates are shown instead of implying a calendar month.

Both periods reuse the existing KPI aggregations and account, branch, seller, client, channel,
status and tag filters. Current KPI definitions remain unchanged. Each comparable metric returns
its previous value, absolute delta and nullable percentage as decimal strings. Percentage change
requires a positive previous value; both zero returns zero and a nonzero value over zero has no
comparison base. Missing time samples remain null. Conversion uses percentage points rather than
relative percent. Decreasing response or acceptance time is presented as an improvement.

The compared KPIs are RFQs, sends, acceptances, conversion, response time and acceptance time.
Follow-up is a current snapshot, so it is excluded. Accepted money is reported separately by currency
and is excluded from period comparisons. The PDF uses the same currency groups as the dashboard.
Even an empty current interval displays its headline numbers and previous comparison.

### Current-period aggregates

- `kpis.rfqs_received`: RFQs received in the period.
- `kpis.quotes_sent`: first `SENT` event per quote inside the period.
- `kpis.quotes_accepted`: latest distinct `ACCEPTED` event per quote inside the period.
- `kpis.sent_accepted`: of the quotes first sent in the period, how many were accepted since, up to
  the moment of the read.
- `kpis.conversion_rate`: `sent_accepted` over `quotes_sent`, a cohort share between `0` and `1`.
  An acceptance in the period of a quote sent before it counts in `quotes_accepted` but not here, so
  the rate never exceeds `1`. It is `null` when nothing was sent.
- `kpis.accepted_amounts`: commercial totals of the resolved accepted versions, grouped by their
  original `currency`, with `amount` as a decimal string. No exchange rate or implicit ARS filter is
  applied. The compatibility scalar `accepted_amount` and its `accepted_currency` are present only
  when exactly one currency resolves; both are `null` for mixed currencies or no resolved versions.
  A resolved zero is a real amount with its currency, not a missing measurement. See below.
- `kpis.response_time`: first send minus RFQ arrival, for RFQs received in the period. Manual-entry
  channels are excluded because there was no inbound message to answer.
- `kpis.time_to_acceptance`: for each quote accepted in the period, its latest acceptance minus its
  first send, as average, median and sample.
- `kpis.follow_up_pending`: quotes in scope flagged for follow-up right now. It reads the current
  flag, not the period: the flag is cleared on the next activity, so there is no history to count.
- `quotes_by_status`: quotes created in the period, grouped by their current status.
- `timeline`: the trend, one point per bucket of `granularity` — `day`, `week` (from Monday) or
  `month`. Each point carries the orders received, the first sends, the acceptances and the cohort
  conversion of that bucket's sends (`sent_accepted`, `conversion_rate`, `null` when it sent nothing).
  Buckets with no activity are present with zeroes; a partial first bucket is dated by the period's
  first day. With no `granularity` requested, periods up to 31 days are daily, up to 120 weekly, and
  longer ones monthly; the response echoes the one applied.
- `top_products`: the five most requested products across each visible RFQ's current quote version,
  ranked by line occurrences. Every quote line counts once, regardless of its quantity. Matched
  products use the canonical catalog name; unmatched lines use the requested description, grouped
  ignoring case and surrounding spaces. Every ranking breaks ties by name, then by id, so the same
  data always yields the same order.
- `top_revenue_products`: up to five matched products per currency with the largest accepted line
  revenue in the period. Each row identifies its `currency`. Revenue is
  `quantity × unit_price_snapshot` summed across the lines of the resolved
  accepted version, rounded to two decimals. Unmatched lines are excluded because they have no
  product identity.
- `top_clients`: up to five clients per currency with the largest accepted amount in the period,
  one row per client and currency.
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
acceptance that cannot be tied to a version contributes to the accepted count without inventing a
currency or amount. The same resolution feeds the accepted amounts, the top clients and product revenue
ranking.

Product revenue is gross line value and subtracts no discounts: the ranking deliberately reports
`quantity × unit_price_snapshot` even when an `ITEM` discount names a single line, because a
`TOTAL` or `ITEM_SET` discount cannot be attributed to one product without inventing an allocation
rule. This means `top_revenue_products` does not sum to its currency's accepted amount when the accepted
version carries discounts, and it excludes unmatched lines.

## Assistant decisions

`assistant` reads the conversation assistant's decision log (`handler_decision`) for the proposals
created in the period on the quotes in scope: how many the seller approved as is, edited, rejected,
resolved by hand, or has not decided yet. The log is written in two phases — the row is inserted
when the assistant proposes and completed when the seller decides — so an undecided proposal is a
proposal the seller ignored, and it stays in the denominator. `approval_rate` is approved-as-is over
every proposal, `null` when there were none.

`ai_match.version_lines` and `ai_match.version_unmatched` count the lines of the current versions
of the period's orders and those still `NO_MATCH`: what the seller is left with after reviewing the
AI's reading, next to the reading itself.

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
and the metric definitions that could otherwise be misread. Each currency has its own commercial
section with its accepted total and both monetary rankings, matching the dashboard. Currency codes
identify every monetary figure; currencies are never summed, ranked or scaled against one another.
Empty sections say so rather than printing fabricated zeroes.

## Configuration and indexes

`REPORTS_MAX_RANGE_DAYS` defaults to `366` and is capped by the code at `3660`. The high-confidence
threshold is the catalog setting, passed from the API composition root, so changing catalog
calibration moves the report with the review screen.

The report reads are backed by `00072_seller_reports.sql`: a branch/time index for RFQs, an account/time
index for quotes,
partial status and send indexes for the event reads, a partial client-action index for acceptance
resolution, and a generation index for the AI summary; `00073_handler_decision_report_index.sql`
adds the account/time index the assistant metrics read. They are in the consolidated reference
schema in `apps/api/database/01_create_tables.sql` as well.

`00090_quote_status_change_quote_report.sql` adds the complementary account/quote/status/time
index for per-quote event lookups, including prepared statements that use generic plans. The
period-oriented index remains useful for scanning events across an account. Cohort conversion
aggregates the first send and latest acceptance once per quote before counting or grouping them
into timeline buckets; acceptances after the report period still convert their original send
cohort.

### Synthetic performance check

With `TEST_DATABASE_URL` and `TEST_DATABASE_ADMIN_URL` pointing to a disposable, migrated database,
run this opt-in integration test from `apps/api`:

```bash
COTI_REPORT_BENCHMARK=1 go test -tags=integration ./internal/integration -run '^TestSellerReportSyntheticBenchmark$' -count=1 -timeout=5m -v
```

The fixture creates 10,000 RFQs, accepted quotes, versions and line items, plus 20,000 status events.
The current and previous periods each contain 5,000 RFQs. Five complete service reads use the
restricted application role and tenant transaction. The output reports every full-read duration,
its median and maximum, and separate durations for the current report and previous-period KPIs.
Fixture setup, statistics refresh and teardown are excluded from those measurements. The test
asserts both period counts and removes its own rows. Local timings describe that synthetic workload
and do not establish a production latency target.

An empty selected period keeps zero-valued KPIs and the previous-period comparison visible, with
a compact notice above them. KPI cards place comparisons in a separate footer. The PDF displays
both accepted amounts in its headline card when exactly two currencies are present; larger
currency sets retain the detailed currency sections without a combined total.
