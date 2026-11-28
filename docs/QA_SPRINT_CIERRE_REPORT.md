# Closing sprint independent QA report

Date: 27 November 2026. Branch: `feat/closing-sprint`.

## Executive assessment

**Release recommendation: NO-GO pending the browser retest described below.**
The five features have real implementations across their relevant layers. The audit corrected
**three P1 defects and four P2 defects**, including currency reporting and report-query performance
found during independent verification. No outstanding P0 or P1 code defect was identified.
The complete Go integration run now passes; earlier timed-out runs are not presented as passes.
Frontend and production-build results are recorded in the verification ledger.

Real HTTP flows were exercised against an isolated PostgreSQL database, the running API, actual
LLM extraction and embedding adapters, and authenticated customer sessions. Automatic quotation,
manual review/edit/send, rejection, replay protection and new-version publication were observed.
Production deployment, commits, pushes, Notion changes and real customer contact were not performed.
A successful API flow does not certify browser interaction or layout.

## Sources and initial repository state

- Implementation base: `dev` at `ab117efa`.
- Audited implementation: `3f6ef8eb` (`feat: implement closing sprint workflows`).
- Previous validation report: `691f64de` (`docs: record closing sprint validation results`).
- Original implementation: 67 files, 2,751 additions and 179 deletions against the base.
- The initial tracked working tree was clean. User-owned untracked `output/` and `tmp/` were preserved.
- The requested `docs/COTI_SPRINT_CIERRE_PLAN_IMPLEMENTACION.md` was absent. Its original was
  recovered from `C:/Users/zunin/Downloads/COTI_SPRINT_CIERRE_PLAN_IMPLEMENTACION.md`.
- The plan explicitly labels its detailed acceptance criteria as proposals. All five Notion card
  fetches returned `object_not_found`; their contents could not be independently revalidated.
  Available internal decisions, state rules, technical documentation and executable migrations
  were used to establish compatible behavior.
- `docs/internal/domain/estados.md` authorizes the initial automatic valuation exception:
  complete initial materials may become `QUOTED`; seller review and sending remain manual.
- The plan explicitly prohibits combining currencies without a documented conversion mechanism.
  Existing ARS-only invoicing does not restrict quotation currencies. Reports therefore preserve
  original currencies without inventing conversion rates or silently omitting USD.

The agent-workflow, API, frontend, UX/motion and testing conventions were applied. PDF rendering
was checked with the PDF skill. Both skill trees remain byte-identical. No unrelated changes were
reverted. The internal results mirror was updated locally and remains gitignored.

## Assessment by task

| Task                               | Initial state                                                                                                                                   | Verified outcome and limit                                                                                                                                                                                                                                                                        |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Automatic quotation                | Shared deterministic valuation, eligibility and transaction existed; a concurrent price commit could still produce a stale snapshot.            | Fixed the price clock. Real-database replacement/first-price regressions pass. Live AI customer intake produces a mutable `QUOTED` offer; edit and explicit publication work. Missing price retains the line in `DRAFT`, with no send. Browser interaction remains unverified.                    |
| Previous-period reports            | Equal-length Argentine calendar comparison existed. Existing money aggregates combined currencies; synthetic load exposed repeated event scans. | Comparisons and filter scope pass regression tests. Currency-aware JSON, dashboard and PDF are implemented. Cohort query and migration 90 remove the measured repeated-scan pathology. Full service timing is measured below; it is not a production SLO. Browser layout remains unverified.      |
| Portal rejection feedback          | Optional comment reused transactional version-bound client actions.                                                                             | HTTP authorization, length validation, normalization, rejection, replay and historical version binding pass. Frontend cancel/action/history tests run. A later version can be edited and published without altering the earlier rejection. Actual browser round trip remains blocked.             |
| Operational setup and price alerts | Readiness used real account/branch/catalog/price/staff data, but action links lost branch context.                                              | Explicit authorized branch links and destination resolution fixed. Navigation regressions pass. Live removal of one current price exposes only the affected branch's gap; restoring it clears the gap and leaves an already frozen offer unchanged. Browser alert interaction remains unverified. |
| Demo catalog and benchmark         | Real seed, backfill and matcher existed; the local-only guard trusted the URL authority instead of the driver's effective target.               | Effective-driver target validation fixed. Live embedding backfill and benchmark executed on synthetic local data. All positive retrievals succeed; one negative remains conservatively ambiguous, explicitly reported below.                                                                      |

## Findings and corrections

### QA-01 — P1: automatic valuation could snapshot an obsolete price

The shared `priceInForce` predicate used PostgreSQL `now()`, fixed at transaction start.
Draft generation begins its transaction before catalog locking. A price committed while it waited
could therefore be ignored even after the required locks were acquired. A first price could also
be missed. Before correction, the regression observed `10` instead of replacement `20`, and no
first price. The predicate now uses `statement_timestamp()` at both validity boundaries.

The existing lock protocol, deterministic discount engine, batched catalog reads and parameterized
SQL remain in use. The clock predicate predates the sprint; the new automatic transaction exposed
its defect. Replacement and first-price regressions were observed red before and green after.

### QA-02 — P1: demo protection allowed a remote effective database target

The wrapper checked `new URL(url).hostname`, while pg also accepts query-string connection
overrides. A local-looking URL containing `?host=remote.example` could connect remotely with the
owner role. The guard now inspects an unconnected pg client's resolved connection parameters.
Both roles must target the same explicit local host, port and database. Unit and actual command
regressions reject the override before seeding. The failing pre-fix assertion was reproduced;
no remote connection was made. This defect was introduced by the sprint wrapper.

### QA-03 — P2: readiness actions lost the affected branch

Generic catalog/price URLs relied on the global branch selection, which could be absent or refer
to another branch. Route builders now carry `branch_id`. Both administrator destination pages
validate it against active authorized branches and reject unknown, inactive, empty and repeated
values. The header reflects explicit page scope; selecting another branch removes the override
and retains unrelated query parameters. The API still validates branch input independently.

Navigation tests cover explicit scope, selection fallback and invalid inputs. A throwaway-copy
mutation that ignored the explicit price branch failed on the intended branch assertion.
This was introduced by the new readiness navigation; no authorization bypass was found.

### QA-04 — P2, pre-existing: monetary reports combined currencies

Accepted totals and client/product revenue aggregates lacked a currency dimension; the dashboard
formatted their result as ARS. Currency grouping now runs through repository, domain, DTO, OpenAPI,
frontend mapper, dashboard and PDF. `accepted_amounts` contains decimal-string totals by currency.
The legacy scalar and its explicit `accepted_currency` are present only for one resolved currency;
both are null for mixed or unavailable amounts. A real resolved zero retains its currency.
Unresolved accepted versions remain in acceptance counts without inventing zero-valued money.

Client and product monetary rankings return five entries per currency. UI and PDF group them by
currency and never compare their magnitudes across currencies. Existing accepted-version
resolution, filters and tenant scope remain intact. No FX mechanism was added. Repository tests
cover mixed/single/zero/unresolved amounts, per-currency ranking limits and account/branch isolation;
handler, mapper, UI and PDF tests cover propagation. A mutation restoring a combined ARS scalar
failed the new mixed-currency regression. Actual authenticated HTTP and PDF evidence is recorded
below. This defect predates the comparison feature; the six comparison metrics remain non-monetary.

### QA-05 — P2: report queries repeatedly scanned status history

The conversion timeline used a correlated acceptance lookup for each sent quote. The 10,000-order
fixture exposed 5,000 repeated scans and over one million event-buffer hits. A shared grouped CTE
now reads each quote's earliest send and latest acceptance once, preserving the existing cohort
rule including acceptance after the report period. Tests cover acceptance before, equal to and
after first send, later acceptance and re-sends.

Prepared generic plans also degraded per-quote response-time lookups. Migration
`00090_quote_status_change_quote_report.sql` adds the complementary partial index on
`(account_id, quote_id, new_status, changed_at)` for SENT/ACCEPTED events. The period-oriented index
remains available. The consolidated reference contains the identical index. Fresh migrations,
84 down/up, real query plans and the full-service benchmark were executed. Both repeated-scan query forms already exist in the `ab117efa` base. The sprint adds a
previous-period KPI read, increasing exposure to the existing cost; it did not introduce the
original timeline query or generic-plan index weakness. This corrects measured query behavior without changing KPI
formulas, report authorization or transaction ownership.

## Architecture and acceptance checks

Code inspection and automated regression evidence establish the following boundaries:

- Automatic and manual quotation share deterministic valuation and discounts. AI extraction uses
  the existing forced schema; it does not calculate totals, approve an offer or initiate delivery.
  Initial automatic quotation remains mutable. Unmatched, ambiguous, incompatible or unpriced
  lines are retained for review; the one-RFQ/one-quote constraint and transactional creation remain.
- Services own transactions; repositories do not commit. SQL is parameterized and account/branch
  scope is preserved, with the restricted RLS role used in real tests. Catalog reads are batched;
  report aggregation does not introduce per-row repository calls. Money crosses the API as decimal
  strings, and backend state transitions remain authoritative.
- Comparisons use the immediately preceding equal number of Argentine calendar days with
  half-open timestamps and unchanged filters/permissions. Missing samples and zero-base percentage
  changes remain null; conversion uses percentage points and response-time direction is inverted
  appropriately. Follow-up snapshots and monetary totals are not given misleading comparisons.
- Rejection feedback uses the server's 512-character limit and whitespace normalization. Customer
  authorization, send/version ownership, first-action idempotency and transition/comment atomicity
  are preserved. Customer feedback remains distinct from internal notes and immutable history.
- Readiness is administrator-only and reflects active authorized catalog/branch/price/staff data.
  Empty mandatory data remains pending, technical failures propagate, inactive products are
  excluded, and optional channels do not reduce mandatory readiness progress.
- Frontend boundary mapping remains snake_case to camelCase. New presentation reuses shared UI,
  semantic color/type tokens and translations; no separate design system or dark variant was added.
  Tests cover rendering and interactions in the simulated DOM. Focus, responsive layout and motion
  in an actual browser remain the explicitly unexecuted checks below.

## Executed verification

Commands use `pnpm.cmd` because the local PowerShell execution policy rejects its `.ps1` shim.
Logs are under the OS temporary directory. A skipped or interrupted command is not a pass.

| Verification                                                           | Actual result                                                                                                                                                                                                                                                                                                                                                                 |
| ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Final `pnpm.cmd check`                                                 | Passed: API build/vet and route generation/type checks for both web apps.                                                                                                                                                                                                                                                                                                     |
| Final `pnpm.cmd lint`                                                  | Passed: all four tasks. The subsequent test-only harness edit passed scoped ESLint.                                                                                                                                                                                                                                                                                           |
| Final production `pnpm.cmd build`                                      | Passed: `pnpm.cmd build --force`, all four tasks, zero cached tasks; 1 minute 49.79 seconds. API, shared UI CSS, backoffice and portal artifacts were rebuilt.                                                                                                                                                                                                                |
| Full Go: `go test -tags=integration ./... -count=1 -timeout=20m -json` | Passed in one uninterrupted run: 25 packages, 1,775 top-level tests; 368 integration and 204 repository top-level tests. Integration package 562.815 seconds. No failure events.                                                                                                                                                                                              |
| Explicit Go skips                                                      | Five: Windows POSIX-permission assertion, unconfigured private Neon bucket, opt-in live catalog evaluation, demo benchmark and report benchmark. The latter two benchmarks were executed separately. No live Neon result is claimed.                                                                                                                                          |
| Full shared UI suite                                                   | Passed: 24 files, 126 tests, default timeout, one worker; 178.12 seconds.                                                                                                                                                                                                                                                                                                     |
| Full backoffice suite                                                  | Passed: 217 files, 1,778 tests, two workers, default timeouts; 1,212.60 seconds in the isolated copy.                                                                                                                                                                                                                                                                         |
| Full customer portal suite                                             | Passed: 44 files, 203 tests, two workers, default timeouts; 211.93 seconds in the isolated copy.                                                                                                                                                                                                                                                                              |
| Full scripts                                                           | Passed: 84 tests in 16 suites, zero failures or skips; 160.196 seconds, both local database roles, one test worker.                                                                                                                                                                                                                                                           |
| Migrations                                                             | Disposable databases built through all 84 migrations. Migration 84 down removes the index and up restores the exact reference definition.                                                                                                                                                                                                                                     |
| OpenAPI                                                                | Regenerated all three committed specification files for nullable/currency fields; independent regeneration succeeded and SHA-256 hashes remained unchanged for all three files.                                                                                                                                                                                               |
| Price/navigation/money regressions                                     | Pass within full suites or the focused runs described above. Red-before/green-after evidence and throwaway mutations are distinguished from ordinary passes.                                                                                                                                                                                                                  |
| Global `pnpm.cmd format:check`                                         | Earlier full command failed on 984 files, including unrelated baseline formatting. No blanket rewrite was performed.                                                                                                                                                                                                                                                          |
| Changed-file formatting and whitespace                                 | Passed for all audit web/script/docs files and changed/new Go files; final report formatting and whitespace checks also passed.                                                                                                                                                                                                                                               |
| Real browser                                                           | Not executed: local frontend startup was repeatedly rejected by automatic approval (`blocked by policy`), including after renewed user authorization. Browser-host inventory was unavailable. The native Chrome fallback then stopped Computer Use because it could not identify the current Windows browser URL confidently enough to enforce policy. No UI pass is claimed. |

The Go run records 3,173 successful test events including subtests; these overlap the top-level
count and are not additional independent tests. After completion, all 69 public application/reference
tables were counted in the integration database. Only migration-owned product families (13) and
subgroups (52) remained; application fixtures were fully removed. The benchmark database was
independently checked with the same result.

Earlier full integration attempts reached 15- and 30-minute limits; their partial results were not
complete passes. Earlier broad frontend attempts encountered timing failures and EBUSY while
loading dependencies in OneDrive. They remain recorded as failed/incomplete attempts, superseded
only where a complete final run actually passed. To isolate file-locking effects, dependencies were
physically copied from the offline cache into an external temporary checkout; no downloads,
security exclusions or production source changes were required. Independent SHA-256 comparison confirmed all 917 frontend/configuration files identical to the
workspace, including the added navigation test and edited undo-history test. The hash manifest is
`coti-qa-resume-all-frontend-hashes.json` in the temporary directory. Final commands and counts are recorded above.

The CUIT/DNI undo-history stress test keeps the same 102 paste events, 150 undo events and final
value assertion. Its edit caches the mounted input and avoids repeated document queries inside
those loops; no timeout or behavioral assertion was relaxed. It ran in 365 ms in the isolated full
suite; the prior loaded OneDrive run timed out at 6,109 ms. These differ in both environment and
query overhead, so their timing difference cannot be attributed exclusively to the test edit.

## Live API and customer-flow verification

The environment uses a disposable local pgvector/PostgreSQL 16 container, the restricted
`coti_app` NOBYPASSRLS role and synthetic demo/customer data. Real extraction and embedding
providers received only synthetic input. Mail uses the console adapter; no real customer message
was sent. WEBAPP delivery is the successful publication channel. EMAIL attempts correctly report
failure because quotation email transport is not configured in this fixture; SMTP delivery is
not certified by these checks.

Observed HTTP outcomes:

1. Administrator readiness returns 200; unauthenticated access returns 401; unavailable branch
   access returns 403. Existing seller-only readiness denial was verified in the initial run.
2. Customer-authenticated text intake and replay both return 201 and resolve to one authored order.
   Actual LLM extraction and semantic matching produce a `MATCHED`, mutable `QUOTED` offer for two
   cement bags, ARS 19,000, without a send. Seller edit to three bags produces ARS 28,500.
3. Explicit seller publication returns 201, freezes the version and reaches `SENT` through WEBAPP.
   Replaying the same idempotency key returns 200 with the same send/delivery identifiers.
4. An authenticated customer can read the active delivered offer. Anonymous access and a staff JWT
   used as a customer token return 401. An unrelated manual request cannot be resolved as an authored
   customer order (404).
5. A 513-character rejection is rejected with 422 and leaves the offer SENT. A valid comment is
   trimmed, stored once and transitions to REJECTED. Replaying with ACCEPT returns the original
   rejection and timestamp, preserving first-action semantics.
6. Seller EDIT reactivation creates a mutable second version. Editing, accepting materials and
   publishing it works; the old rejection remains attached only to the original version. The new
   SENT version was reserved for browser verification and has not been consumed for that purpose.
7. Temporarily closing one local product's current branch price makes only that branch report one
   missing price. New real customer intake retains the matched line unpriced in DRAFT and sends
   nothing. Restoring the price clears the readiness gap without changing the frozen SENT offer.
8. Customer-authenticated acceptance of ARS 19,000 and USD 100 was verified through real quotation/send/action endpoints. The November report with an explicit QA client filter returns two acceptances, null scalar amount/currency and separate ARS 19,000/USD 100 groups; the same client appears once per currency. The branch-wide report correctly includes another seeded client's ARS 178,800, yielding ARS 197,800/USD 100. The USD product's original ARS 8,900 price/minimum ARS 7,832 was restored and verified before rechecking the frozen USD result.

Request/session tokens remain in a private temporary state file, not in this report or git.
Rejected harness requests (missing delivery destination, attempting an already-associated client
operation, or reading an array as an envelope) were corrected before the final assertions and are
not misclassified as product defects.

PDF tests include mixed currencies and a maximum NUMERIC(14,2) USD amount. The generated pages
were rendered to PNG with Poppler and visually inspected for currency labels, totals and clipping.
Local artifacts include `output/pdf/seller-report-currencies.pdf` and `seller-report-usd.pdf`.
The actual HTTP PDF (`output/pdf/seller-report-http-currencies.pdf`) is 78,992 bytes and four pages. Text extraction confirms ARS 19,000 and USD 100; all four rendered pages were visually checked. A sanitized request/result ledger is saved in the temporary directory as `coti-qa-currency-http-ledger.json`. An initial global-ARS expectation omitted seeded acceptances; the corrected assertion explicitly verifies both the seeded global total and the isolated client total.

## Independently measured demo benchmark

`node scripts/demo-catalog.mjs --apply --benchmark` ran with synthetic data and the real embedding
provider in the initial audit. The clean seed has 20 active Villa Bosch products, all with current
1536-dimensional `text-embedding-3-small` vectors. The prior report's 25 products included local
additions and are not the clean-seed baseline. Resume-time live backfill also completed for the
fresh 20-product fixture; the following benchmark results are from the independently measured run,
not a claim that the entire dataset was rerun during resumption.

Dataset `demo-v1`: 15 queries, 12 positives and three negatives. Catalog hash:
`722943c472ae6d2d056be08b39052ab089debcb51862f198b196c832d9a9ed62`.

| Metric                                 | First pass    | Repeat pass   |
| -------------------------------------- | ------------- | ------------- |
| Top-1 / recall at K                    | 12/12 / 12/12 | 12/12 / 12/12 |
| MRR                                    | 1.0           | 1.0           |
| Correct final decisions                | 14/15         | 14/15         |
| Confident false positives on negatives | 0/3           | 0/3           |
| Provider/search errors                 | 0             | 0             |
| Total latency p50                      | 589.948 ms    | 695.386 ms    |
| Total latency p95                      | 1922.924 ms   | 1037.394 ms   |

`unknown-cement-size` returns AMBIGUOUS for `cemento 25 kg`, while its label is NO_MATCH.
It remains reviewable and cannot trigger automatic valuation. Labels and thresholds were not
changed to improve the score. Raw artifact: `.artifacts/demo-catalog-benchmark/report.json`.

These are first/repeat measurements, not controlled cold/warm runs. The LLM reviewer is disabled
for this benchmark. The sample is small and no approved quality/latency threshold or comparable
baseline was supplied. The result demonstrates the real demo path, not general production quality.

## Synthetic report performance

`TestSellerReportSyntheticBenchmark` creates 10,000 RFQs, accepted quotes, frozen versions and
items plus 20,000 status events in one synthetic tenant. Current November and previous equal-length
periods each contain 5,000 orders. The actual `SellerReportService.Get` runs five times with the
restricted tenant role and validates both counts. Owner access prepares and removes fixtures;
setup, ANALYZE and cleanup are excluded from request timings.

The first audit measured 118.002, 80.240, 30.115, 25.218 and 65.362 seconds per complete read
(median 65.362, maximum 118.002). That exposed a real capacity concern, even though local concurrent
load made it unsuitable for a controlled production percentile. Failed draft fixture attempts were
not endpoint measurements.

After QA-05, the measured complete reads were **4.510647, 4.492714, 4.521677, 4.775887 and
5.270550 seconds**: median **4.521677**, maximum **5.270550**. Current-report portions were
3.539010, 3.455636, 3.121838, 3.338205 and 3.849677 seconds; previous-period KPI portions were
0.928089, 1.007283, 1.371372, 1.420063 and 1.410482 seconds. The benchmark passed, including cleanup,
in 122.545 seconds. Full Go/frontend verification shared the host during this final sample.

Comparable query-plan checks directly verified the mechanisms:

| Plan measurement                 | Before        | After      |
| -------------------------------- | ------------- | ---------- |
| Timeline execution               | 17,649.654 ms | 121.178 ms |
| Prepared response-time execution | 11,633.040 ms | 167.735 ms |
| Prepared event-buffer hits       | 1,065,000     | 25,001     |

Raw sanitized evidence is under `.artifacts/report-benchmark/`: `report.json`,
`timeline-before.json`, `timeline-after.json`, `response-time-prepared-before.json` and
`response-time-prepared-after.json`. It includes fixture counts, role, migration and cleanup.
These local measurements do not establish a production latency objective or claim an isolated
runner. No arbitrary pass threshold was added. Representative deployment capacity remains an
operational measurement, rather than an assertion inferred from this fixture.

Reproduce from `apps/api` with both test database URLs pointing to a disposable migrated database:

```bash
COTI_REPORT_BENCHMARK=1 go test -tags=integration ./internal/integration -run '^TestSellerReportSyntheticBenchmark$' -count=1 -timeout=10m -v
```

## Change manifest and operating impact

The audit changes current-price reads, guarded demo commands, branch navigation and report money/
query behavior. Migration 84 must run before deploying the optimized report. The API's money
contract now includes currency groups and nullable legacy scalars; backend, generated OpenAPI and
the in-repo frontend consumer were changed together. No runtime environment variable, enum, table
or money formula was introduced. `COTI_REPORT_BENCHMARK` is a test-only opt-in.

Technical docs for catalog, accounts/branches, reports and the closing sprint describe the current
behavior and verification commands. The complete audit working-tree manifest follows; generated
OpenAPI and regression fixtures are included. No files were staged, committed, pushed or deployed.

```text
M apps/api/database/01_create_tables.sql
M apps/api/docs/docs.go
M apps/api/docs/swagger.json
M apps/api/docs/swagger.yaml
M apps/api/internal/delivery/http/dto/seller_report.go
M apps/api/internal/delivery/http/handler/seller_report_handler_test.go
M apps/api/internal/delivery/http/handler/seller_report_handler.go
M apps/api/internal/domain/seller_report.go
M apps/api/internal/integration/auto_quote_input_lock_test.go
A apps/api/internal/integration/seller_report_benchmark_test.go
M apps/api/internal/pdf/report_renderer_test.go
M apps/api/internal/pdf/report_renderer.go
M apps/api/internal/repository/product_price_repo.go
A apps/api/internal/repository/seller_report_currency_test.go
M apps/api/internal/repository/seller_report_repo_test.go
M apps/api/internal/repository/seller_report_repo.go
M apps/api/internal/services/seller_report_service_test.go
A apps/api/migrations/00090_quote_status_change_quote_report.sql
M apps/backoffice/app/(protected)/_components/branch-switcher.test.tsx
M apps/backoffice/app/(protected)/_components/branch-switcher.tsx
M apps/backoffice/app/(protected)/reports/_components/report-kpis.test.tsx
M apps/backoffice/app/(protected)/reports/_components/report-kpis.tsx
M apps/backoffice/app/(protected)/reports/_components/report-summary.tsx
M apps/backoffice/app/(protected)/reports/_components/report-top-clients.tsx
M apps/backoffice/app/(protected)/reports/page.test.tsx
M apps/backoffice/app/(protected)/reports/page.tsx
M apps/backoffice/app/(protected)/settings/catalog/page.tsx
M apps/backoffice/app/(protected)/settings/onboarding/_components/operational-readiness.test.tsx
M apps/backoffice/app/(protected)/settings/onboarding/_components/operational-readiness.tsx
A apps/backoffice/app/(protected)/settings/onboarding/readiness-navigation.test.tsx
M apps/backoffice/app/(protected)/settings/prices/page.tsx
M apps/backoffice/components/tax-id-input.test.tsx
M apps/backoffice/config/routes.ts
M apps/backoffice/lib/api/reports.test.ts
M apps/backoffice/lib/api/reports.ts
M apps/backoffice/translations/es.json
A docs/QA_SPRINT_CIERRE_REPORT.md
M docs/technical/accounts-and-branches.md
M docs/technical/catalog.md
M docs/technical/closing-sprint.md
M docs/technical/reports.md
M scripts/demo-catalog.mjs
M scripts/lib/demo-catalog.mjs
M scripts/lib/demo-catalog.test.mjs
```

The original implementation manifest below is reproduced by
`git diff --name-status ab117efa..691f64de`. Audit changes can be inspected with `git diff 691f64de`
plus the listed new files.

<details>
<summary>Original implementation files (A = added, M = modified)</summary>

```text
M	README.md
M	apps/api/cmd/api/main.go
M	apps/api/docs/docs.go
M	apps/api/docs/swagger.json
M	apps/api/docs/swagger.yaml
M	apps/api/internal/delivery/http/dto/onboarding_dto.go
M	apps/api/internal/delivery/http/dto/seller_report.go
M	apps/api/internal/delivery/http/handler/onboarding_handler.go
M	apps/api/internal/delivery/http/handler/seller_report_handler.go
M	apps/api/internal/delivery/http/router.go
A	apps/api/internal/domain/onboarding_readiness.go
M	apps/api/internal/domain/seller_report.go
A	apps/api/internal/integration/auto_quote_input_lock_test.go
A	apps/api/internal/integration/demo_catalog_benchmark_test.go
M	apps/api/internal/integration/quote_delivery_test.go
A	apps/api/internal/integration/rfq_auto_quote_test.go
M	apps/api/internal/integration/rfq_pipeline_test.go
A	apps/api/internal/integration/testdata/catalog_match/demo.json
A	apps/api/internal/repository/onboarding_readiness_repo.go
A	apps/api/internal/repository/onboarding_readiness_repo_test.go
M	apps/api/internal/repository/product_price_repo.go
M	apps/api/internal/repository/seller_report_repo.go
A	apps/api/internal/services/onboarding_readiness.go
A	apps/api/internal/services/onboarding_readiness_test.go
M	apps/api/internal/services/onboarding_service.go
A	apps/api/internal/services/quote_auto.go
A	apps/api/internal/services/quote_auto_multiple_test.go
A	apps/api/internal/services/quote_auto_test.go
M	apps/api/internal/services/quote_delivery_service_test.go
M	apps/api/internal/services/quote_service.go
M	apps/api/internal/services/rfq_service.go
A	apps/api/internal/services/seller_report_comparison.go
A	apps/api/internal/services/seller_report_comparison_test.go
M	apps/api/internal/services/seller_report_service.go
M	apps/api/internal/services/seller_report_service_test.go
M	apps/backoffice/app/(protected)/_components/settings-nav-items.test.ts
M	apps/backoffice/app/(protected)/_components/settings-nav-items.ts
M	apps/backoffice/app/(protected)/no-route-loading-state.test.ts
A	apps/backoffice/app/(protected)/reports/_components/report-kpis.test.tsx
M	apps/backoffice/app/(protected)/reports/_components/report-kpis.tsx
M	apps/backoffice/app/(protected)/reports/_components/report-summary.tsx
M	apps/backoffice/app/(protected)/reports/page.test.tsx
M	apps/backoffice/app/(protected)/reports/page.tsx
M	apps/backoffice/app/(protected)/rfqs/[id]/_components/rfq-detail-view.test.tsx
M	apps/backoffice/app/(protected)/rfqs/[id]/_components/rfq-detail-view.tsx
A	apps/backoffice/app/(protected)/settings/onboarding/_components/operational-readiness.test.tsx
A	apps/backoffice/app/(protected)/settings/onboarding/_components/operational-readiness.tsx
M	apps/backoffice/app/(protected)/settings/onboarding/page.tsx
M	apps/backoffice/lib/api/onboarding.test.ts
M	apps/backoffice/lib/api/onboarding.ts
M	apps/backoffice/lib/api/reports.test.ts
M	apps/backoffice/lib/api/reports.ts
M	apps/backoffice/translations/es.json
M	apps/webapp/app/api/public-quote-action/route.ts
M	apps/webapp/app/no-route-loading-state.test.ts
M	apps/webapp/app/quotes/[token]/_components/quote-actions.tsx
M	apps/webapp/app/quotes/[token]/_components/quote-selection-acceptance.test.tsx
M	apps/webapp/translations/es.json
M	docs/technical/accounts-and-branches.md
A	docs/technical/closing-sprint.md
M	docs/technical/reports.md
M	docs/technical/rfq-pipeline.md
M	package.json
A	scripts/demo-catalog.mjs
A	scripts/demo-catalog.test.mjs
A	scripts/lib/demo-catalog.mjs
A	scripts/lib/demo-catalog.test.mjs
```

</details>

## Manual-checklist follow-up: branch-preserving catalog shortcut

While preparing the user's manual checklist, review found a remaining QA-03 navigation path:
the price page's individual-price catalog shortcut used the generic catalog URL. It now carries
the validated effective branch, including an explicit readiness branch. An all-branch view still
opens the account catalog without fabricating branch scope. The technical branch documentation
was updated.

The focused regression failed on the intended assertion before the fix: expected
`/settings/catalog?branch_id=two`, received `/settings/catalog` (one failure, eleven passes).
After correction, all twelve navigation tests passed in 21.26 seconds, with default timeouts.
The initial test draft required an assertion API and absent-branch mock correction; those draft
failures were not counted as product regressions or successful verification.
`pnpm.cmd check` passed again (27.607 seconds); scoped ESLint passed. The final `pnpm.cmd build` also passed all four tasks (one cached) in 1 minute 18.837 seconds.

The 217-file/1,778-test full backoffice run above precedes this two-file follow-up. It was not
rerun in full; the affected navigation suite, type checks, scoped lint and production build were
rerun. No backend, monetary calculation, shared CSS class or customer-portal code changed here.
The manual browser checks remain unexecuted and the release recommendation remains NO-GO.

## Remaining verification and environment

The audit-owned disposable container `coti-qa-resume-20261127` (loopback PostgreSQL port 55432) and local API (port 58000) are retained for the pending browser check. Its integration and performance databases contain only migration reference data; `coti_qa` contains the synthetic live-flow fixture. Existing developer containers, databases, `.env` files and user-owned files were preserved. Frontend startup is still rejected by automatic approval after renewed user authorization. Manual startup was requested, but the native Chrome fallback also ended with an explicit security-policy stop: the tool could not identify the current browser URL in Windows with sufficient confidence. No further Computer Use was attempted. The fixture is retained for a later authorized/manual browser check; no UI pass is inferred from preparation.

Manual startup later opened ports 53000 and 53001, but the original QA command's
`--hostname 127.0.0.1` caused a portal rewrite to loop through the unscoped path and redirect
to the default landing at port 3000. HTTP checks observed that 307 on the prefixed login too.
Installed Next 16.1.5 helpers independently reproduced the cause: NextURL normalizes loopback to
localhost, so getRelativeURL treats the rewrite as external against a 127.0.0.1 startup origin;
a localhost startup origin returns the internal relative path. The corrected manual command uses
`--hostname localhost`, `WEB_PORTAL_BASE_URL=http://localhost:53001` and
`WEB_LANDING_URL=http://localhost:53000`, preserving the API origin on port 58000. A user-side
restart is required; no successful browser check or post-restart HTTP pass is claimed here.
The portal login identifies the synthetic customer by CUIT/CUIL `20123456786`, not email.

1. Obtain a working local frontend/browser session and run the portal rejection through seller
   history, automatic quotation review/edit/send, branch-specific alert resolution, report layout
   and PDF download. Verify focus, keyboard use, loading/error recovery, narrow screens and reduced
   motion on those pages. Automated and HTTP evidence does not certify these interactions.
2. Keep the benchmark limits explicit: conservative missing-size review, a small evaluation set,
   no production percentile and no supplied acceptance SLO. No threshold was invented to force a GO.
3. Resolve the unrelated repository formatting baseline separately. This audit preserved those files.

The code corrections and executed checks are complete to the extent documented above. The browser
check remains an unexecuted acceptance check under the requested release standard, so an
unconditional production GO is not supported by the evidence.

## User manual feedback: resend cycle, failed intake, and report presentation

The user supplied portal/report screenshots and reported that the setup/price-alert flow passed.
The remaining feedback produced these follow-ups:

- **QA-06 (P1, corrected):** a RESEND reactivation retained the frozen version and therefore
  incorrectly reused the preceding cycle's customer rejection. Customer decisions now belong to
  the version's send cycle, delimited by transitions into QUOTED. Deliveries within a cycle share
  the first decision, including cross-channel replay; a new cycle can receive a new answer.
  Stale unanswered links cannot close the reactivated quote. Old versions retain their snapshot;
  an authorized historical order link now offers a clear route to the current publication.
- **External provider outage (recovered in QA):** both reported two-bag requests were durably
  received, but Anthropic returned HTTP 400 with insufficient-credit errors during extraction.
  The user explicitly selected OpenAI for QA. A schema-forced Responses API adapter was added
  behind the existing generation port; production defaults and local .env files were preserved.
  The running QA process uses OpenAI/gpt-4.1-mini and the existing configured key.
- **QA-07 (P2, corrected):** the report's empty state occupied excessive space and comparisons
  lacked visual separation. A compact status notice now precedes the KPIs; cards have stronger
  value hierarchy, consistent spacing, and separate comparison footers. PDF cards use aligned
  headers and show both accepted currencies directly when two currencies are present.
- **Date-range result verified:** the user's 2026-11-01 through 2026-11-16 range contains zero
  synthetic customer movements. HTTP 200 confirmed zero received/sent/accepted values and the
  preceding range 2026-10-16 through 2026-10-31. QA activity is on November 27; historical
  movements were not fabricated to fill the empty range.

### Executed recovery and verification

The two original RFQs were recovered in the disposable QA database, without creating new RFQs.
Recovery checked FAILED status and absence of a quote, appended FAILED-to-RECEIVED history,
and replayed each original authenticated public request with its existing request id. Both
returned HTTP 201 and became GENERATED with exactly one QUOTED quote, quantity 2.00,
MATCHED Cemento Loma Negra 50 kg, total ARS 19000.00. The original failure history remains.
This was an operator recovery of the synthetic fixture, not a newly added automatic retry flow.

Actual authenticated customer HTTP reads also confirmed that the preceding v2 send remains
ACTIVE/REJECT while the latest v2 send is ACTIVE with no customer response. No real customer
contact was initiated; outbound mail remains console-only in QA.

Verification for this follow-up:

- Full non-integration Go suite passed. The affected provider/config/service packages were
  rerun after adding adapter regressions; the final cycle/version-pinning integration regressions
  passed against PostgreSQL with the restricted application role and owner fixtures.
- Integration coverage includes rejecting a quote, reactivating and resending the same frozen
  version, a fresh acceptance, independent first-answer replay in each cycle, cross-delivery
  historical replay, stale unanswered rejection, and preserved older-version snapshots.
- OpenAI adapter tests use an HTTP test server and assert strict JSON Schema, explicit credentials,
  no provider-side storage, text/image/PDF mapping, token caps, usage accounting across retries,
  refusal, incomplete/malformed/trailing output, permanent/transient errors, and invalid inputs.
  Live extraction of the two original requests separately verified the configured QA provider.
- Backoffice report tests: 2 files / 25 tests passed. Customer order detail: 1 file / 3 tests passed,
  including historical/current navigation and refusal of a delivery belonging to another order.
- pnpm check passed (three tasks, 33.375 seconds); scoped ESLint and selective Prettier checks
  passed. The production build passed all four tasks without cache in 1 minute 18.002 seconds.
  A subsequent portal-only rebuild includes the corrected UTF-8 customer notice.
- PDF renderer tests passed. The mixed-currency PDF was rendered to four PNG pages with Poppler
  and visually inspected; values remain separated by currency without overlap or clipping.

The audit-owned API remains on port 58000 with the synthetic fixture. Task frontend processes
on ports 53000/53001 were stopped before rebuilding to prevent mixed Next.js artifacts.
Manual restart commands use hostname localhost and API_INTERNAL_URL=http://127.0.0.1:58000.
The earlier automatic Next.js startup rejection and browser security-policy stop still limit
agent-executed UI verification. The user was asked to restart the compiled applications.
The changed screens still require user browser retesting; no keyboard, narrow-screen, or reduced-
motion pass is inferred from unit tests or PDF rendering. The release recommendation remains
NO-GO pending that acceptance evidence, while the setup/price-alert flow is recorded as user PASS.

The manual startup initially failed because the copied PowerShell text lost environment-variable
names and statement separators. Two local ASCII/CRLF launchers were prepared under
`output/qa/start-portal.cmd` and `output/qa/start-backoffice.cmd`; the user ran them successfully.
Subsequent HTTP checks returned 200 for the portal's /c/qa-cierre/login, backoffice /login, and
API /health. Both frontend servers are listening on localhost (53001 and 53000 respectively).
This confirms startup and route reachability; the changed UI still awaits the user's visual retest.
The final portal notice was checked as valid UTF-8, its three detail-page tests passed again,
and its optimized build completed. The PowerShell wrapper recorded Next's existing relative
Turbopack-root warning as a NativeCommandError; the native Next build nevertheless completed
compilation, type checks, prerendering and BUILD_ID generation, and the resulting server returned 200.
A final provider review also normalized OpenAI input-token counts to exclude separately recorded
cache reads; provider/config tests and scoped go vet passed after that adjustment.
