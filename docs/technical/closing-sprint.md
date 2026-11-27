# Closing sprint implementation and QA

The scope is the five tasks in the implementation document supplied on 27 November 2026.
Work is on `feat/closing-sprint`, based on `dev` at `ab117efa`. The source document was read
from Downloads; it was absent from `docs/`. Its acceptance criteria are proposals. Original
Notion cards could not be read: the connector returned `object_not_found`, and browser access
was stopped because the automation tool could not verify the current URL. No Notion status,
deployment, shared branch or outbound customer message was changed.

## Implementation decisions

| Task                    | Implementation and principal files                                                                                               | Decisions and compatibility                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rejection feedback      | Portal `quote-actions.tsx`, seller `rfq-detail-view.tsx`, existing `QuoteDeliveryService` and `ClientActionRepository`           | Optional free text uses the existing 512-character `client_action.comment`. The customer action pins the send and version; rejection and feedback share the existing transaction. Seller history preserves earlier versions. Internal version notes are not customer feedback. No new commercial categories are assumed.                                                                                                                                                                                                               |
| Automatic quotation     | `quote_auto.go`, `quote_service.go`, `rfq_service.go`, `product_price_repo.go`, API composition root                             | Explicitly authorized in this conversation. A complete initial generation can enter `QUOTED` in its creation transaction. It reuses manual valuation and the discount sweep, locks its catalog inputs, and leaves the version editable. Existing `MATCHED` decisions, positive quantities, compatible units, active branch availability and current prices are required. Zero prices remain valid because the existing domain allows them. Mixed currencies stay in manual review. No automatic send or seller approval is fabricated. |
| Previous-period reports | `seller_report_comparison.go`, shared repository aggregations, report DTO/mapper, report KPI cards                               | The previous interval has the same number of Argentine calendar days and ends where the current interval begins. Both use the same authorization and filters. Rates use percentage points; undefined bases and absent samples remain null. Follow-up is a current snapshot and is not compared. Accepted money is not compared because the existing aggregate has no explicit currency scope. PDF behavior is preserved.                                                                                                               |
| Operational setup       | `onboarding_readiness*.go`, admin `GET /v1/onboarding/readiness`, existing setup settings page and navigation                    | Diagnostics come from current account, branch, catalog, price and staff data. The wizard remains resumable, but its historical completion flags do not determine operational readiness. Empty catalogs are pending; inactive products are excluded. Channels are optional. Missing prices are counted per branch, using the existing validity predicate. Failures propagate to the error boundary.                                                                                                                                     |
| Demo and benchmark      | `scripts/demo-catalog.mjs`, `scripts/lib/demo-catalog.mjs`, `testdata/catalog_match/demo.json`, `demo_catalog_benchmark_test.go` | Reuses the fixed synthetic demo seed, catalog embedding service, pgvector index and production hybrid search/matcher. The wrapper refuses production and remote databases, defaults to dry-run and scopes the seed and backfill to the demo account. Embedding batches resume using existing timestamps and provider retry policy. No alternative vector infrastructure or calibration changes were introduced.                                                                                                                        |

No schema changes or new configuration variables are required. OpenAPI is regenerated for the
additive report response and readiness endpoint. Existing consumers can ignore the new fields.

## Reproducing the demo

Run from the repository root, with the existing local `.env` files and PostgreSQL available:

```bash
pnpm demo:catalog --dry-run --benchmark
pnpm demo:catalog --apply
pnpm demo:catalog --apply --benchmark
pnpm demo:catalog --apply --skip-embeddings
```

Only `--apply` writes data. `--benchmark` calls the real embedding provider only with `--apply`.
The fixed account is `a0000000-0000-4000-8000-000000000001`. The index is the existing shared
pgvector index; rebuilding it is a local maintenance operation. No application server is needed.
The report is `.artifacts/demo-catalog-benchmark/report.json` and contains the catalog hash,
dataset version, expected codes and resolved IDs, model, dimension, commit, environment,
per-query results, denominators, errors and latency measurements.

The backfill writes only missing or stale vectors, outside provider transactions and in bounded
batches. Restart the same command after a provider failure; completed batches are preserved.
Product edits invalidate vectors through the existing `updated_at`/`embedding_updated_at`
policy. Changing the configured model requires the existing explicit refresh command:

```bash
cd apps/api
go run ./cmd/catalog-embed --account a0000000-0000-4000-8000-000000000001 --refresh-all
```

Model provenance is recorded in the benchmark, rather than adding a second vector-versioning
schema. Neither embedded text nor the report contains product prices or provider credentials.

## Measured demo result

The local run used 25 active branch products, including the existing demo additions, and
`text-embedding-3-small`, 1536 dimensions. Nine pending vectors were generated in one batch;
afterward all 25 were current. The dataset is `demo-v1`, evaluation-only, with 15 cases:
12 positives (including ambiguity) and 3 negatives. No benchmark-specific tuning was performed.

| Metric                                | First pass | Repeat pass |
| ------------------------------------- | ---------- | ----------- |
| Retrieval top-1 accuracy              | 12/12      | 12/12       |
| Recall at configured K                | 12/12      | 12/12       |
| MRR                                   | 1.0        | 1.0         |
| Correct final decisions               | 14/15      | 14/15       |
| False positive decisions on negatives | 0/3        | 0/3         |
| Manual review outcomes                | 5/15       | 5/15        |
| Provider/search errors                | 0          | 0           |
| Total latency p50                     | 494.920 ms | 388.093 ms  |
| Total latency p95                     | 732.128 ms | 1604.785 ms |

The remaining case is `cemento 25 kg`: the matcher returns `AMBIGUOUS` instead of the labeled
`NO_MATCH`, so it remains flagged and cannot trigger automatic quotation. This is a conservative
miss, not a confidently wrong match. First/repeat passes do not flush PostgreSQL, OS or provider
caches and are not controlled cold/warm measurements. The language-model reviewer is disabled
for this benchmark. Multi-item extraction is exercised separately by the RFQ suite. There is
no approved quality threshold or comparable baseline, so these numbers do not certify one.

## Validation and remaining manual QA

Each task's targeted service, frontend and PostgreSQL tests was executed before moving to the
next task. Coverage includes rejection retry/version binding, automatic/manual valuation parity
and absence of sends, zero/missing prices, calendar boundaries and filter preservation, live
readiness refresh and tenant isolation, demo command protection and metric denominators.

| Task                    | State       | Task-specific evidence                                                                                                                                               | Remaining QA                                              |
| ----------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| Rejection feedback      | Implemented | Portal action and seller history tests; Go delivery tests; PostgreSQL version binding, length limit, trimmed comment and idempotent replay. Passed.                  | Authorized portal-to-seller browser flow.                 |
| Automatic quotation     | Implemented | Eligibility and multi-product unit tests; real RFQ/manual valuation parity, including multi-line promotion; input-lock concurrency test. Passed.                     | Browser review/edit/send flow.                            |
| Previous-period reports | Implemented | Calendar/leap-year, null/zero/percentage-point and filter tests; PostgreSQL aggregation regressions; mapper, KPI and empty-period page tests. Passed.                | Responsive layout, PDF and production-scale query timing. |
| Operational setup       | Implemented | Admin/scoping/error service tests; live PostgreSQL readiness, zero/expired price and inactive/foreign product tests; mapper, navigation and component tests. Passed. | Browser navigation and reload after configuration edits.  |
| Demo and benchmark      | Implemented | Script safeguards and repeated seed isolation passed; real provider benchmark completed with 14/15 final decisions correct.                                          | Controlled cold/warm timing and broader evaluation data.  |

| Validation                                  | Result                                                                                                                                                                                                                                                               |
| ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pnpm check`, `pnpm lint`, `pnpm test:api`  | Passed. API build/vet and both frontend type checks passed.                                                                                                                                                                                                          |
| Production builds                           | All four packages passed, including the shared UI CSS rebuild.                                                                                                                                                                                                       |
| Script suite with both local database roles | 83 tests passed, including migrations, demo idempotency and environment guards.                                                                                                                                                                                      |
| PostgreSQL integration                      | All 369 integration test functions passed. The repository suite passed separately against a disposable database rebuilt through all 83 migrations.                                                                                                                   |
| Shared UI suite                             | 126 tests in 24 files passed.                                                                                                                                                                                                                                        |
| Portal suite                                | The full attempt passed 197 tests across 41 files and encountered dependency `EBUSY` locks plus a Windows path assertion. The path assertion now normalizes separators; the affected path, contact-verification and routing suites passed targeted reruns (7 tests). |
| Changed frontend behavior                   | Targeted portal, seller detail, report, setup and mapper tests passed incrementally. The final report/detail regression rerun passed 61 tests.                                                                                                                       |
| Formatting                                  | Changed files passed selective Prettier and Go formatting. The global check reports 994 files, including pre-existing formatting and local temporary files; unrelated files were left untouched.                                                                     |

The initial repository run found the demo's existing vector index where a planner test expected
no index. Rebuilding an isolated test database allowed the complete repository suite to pass
without altering the demo index. Multi-line automatic/manual promotion parity also covers the
discount rows in fixture cleanup.

Windows/OneDrive intermittently locks dependency files with `EBUSY`; affected portal tests were
rerun individually. The two existing route-loading assertions now normalize Windows separators.
The complete backoffice attempt passed 1,736 tests in 212 files. Three suites could not import
dependencies and one worker could not start, all with `EBUSY`. The four affected files passed
separate reruns: 28 tests. This covers all 216 backoffice files, but a single uninterrupted green
full-suite command is not claimed. There were no assertion failures in the backoffice run.

Browser automation was unavailable; no visual or complete browser E2E pass is claimed. Manual QA:

1. Send a quote through Mailpit, open its authorized customer link, cancel rejection, then reject
   with a reason. Reload both portal and seller detail; verify feedback and version history.
2. Submit complete and ambiguous requests. Verify `QUOTED` versus `DRAFT`, mutable versions,
   promotion totals, seller review and manual sending.
3. Compare a report with prior activity, a zero-current period and no prior base. Check exact
   ranges, filters, point differences, time direction, narrow screens and PDF export.
4. Open operational setup as an administrator, change branch, remove/restore availability or
   a price, and reload. Verify per-branch counts, optional channels and error handling.
5. Repeat the demo command; verify stable products, no pending vectors and the raw report.

No production-scale report benchmark or controlled browser accessibility/motion inspection is
claimed. Both remain release QA limits rather than fabricated passes.
