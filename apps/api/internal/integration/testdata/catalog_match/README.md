# Catalog match evaluation

What `TestCatalogMatchEvaluation` measures catalog matching against: a real corralón catalog and
client lines labeled with what a seller would accept. The matching calibration in `internal/config`
was settled on it — see [catalog.md](../../../../../../docs/technical/catalog.md#calibration).

```bash
pnpm eval:catalog-match          # the text-only matcher
pnpm eval:catalog-match:review   # plus the language model's review of flagged lines
```

Both need the integration database variables (`TEST_DATABASE_URL`, `TEST_DATABASE_ADMIN_URL`,
addressed as `127.0.0.1` rather than `localhost`, as the testing skill explains) and
`AI_EMBEDDINGS_PROVIDER=openai` with its key in `apps/api/.env`; the review also needs
`AI_LLM_PROVIDER`, and reviews up to 30 lines per order, since the API's own default leaves the
review off. That cap is set by the script itself, so a different one goes on the command line
(`CATALOG_MATCH_REVIEW_MAX_LINES=10 pnpm eval:catalog-match:review`), not in `apps/api/.env`. A run seeds its own account,
embeds the whole catalog, and removes both when it ends. It costs a few cents and takes one to three
minutes, and it varies by a line or two between runs: the embedding model is not deterministic, so
re-run before trusting a one- or two-line move.

The run fails when any line is matched to a wrong product with confidence — `OVERCONFIDENT`, the
right product decided where the labels leave the choice open, is reported but does not fail it — or
when a set's accuracy falls under `CATALOG_MATCH_EVAL_MIN_ACCURACY` (default `0.88`). It logs every
line it did not get right and writes the full report, with the calibration it ran on, to
`.artifacts/catalog-match-eval/report.json`. The calibration is read from `apps/api/.env` as the API
reads it, so a candidate setting is evaluated by setting it there or on the command line.

## The files

- `catalog.json` — 879 products of a real building-materials catalog: name, description and unit
  only. No codes, prices or stock.
- `tuning.json` — 154 lines the calibration was tuned against.
- `holdout.json` — 45 lines kept apart while tuning, to measure whether the settings generalize.

Each line carries the statuses a seller would accept for it and the products that would be a right
answer:

```json
{ "line": "vigueta 4.50", "statuses": ["MATCHED"], "products": ["VIGUETA PRET 4.50"] }
```

`statuses` says how settled the line should come out: `["MATCHED"]` when the line names one
product, `["AMBIGUOUS"]` when it fits several and does not say which (`cemento` against five
cements), `["NO_MATCH"]` when the catalog does not carry it, and more than one when a seller would
take either. `products` lists every product that answers the line; it is empty when none does, and
a line whose status allows `NO_MATCH` may still list the products a seller would accept as a
substitute.

## How a line is graded

| Outcome             | Meaning                                                                      |
| ------------------- | ---------------------------------------------------------------------------- |
| `CORRECT`           | An allowed status, led by an accepted product unless it matched nothing      |
| `OVERCAUTIOUS`      | Flagged harder than it needed to be, with the right product among its offers |
| `OVERCONFIDENT`     | `MATCHED` on an accepted product where the labels leave the choice open      |
| `WRONG_LEADER`      | Rightly flagged, but led by a product that is not the answer                 |
| `MISSED`            | No accepted product among its candidates at all                              |
| `CONFIDENTLY_WRONG` | `MATCHED` on a product that is not an answer — the error a seller cannot see |

## Adding lines

Write the line the way a client would, and label it before running anything: a label chosen after
seeing the output measures the matcher's opinion rather than the seller's. Every product name has
to exist in `catalog.json` exactly. New lines go to `tuning.json`; `holdout.json` only grows with
lines nobody has tuned against, and a calibration change is judged on it without editing it.
