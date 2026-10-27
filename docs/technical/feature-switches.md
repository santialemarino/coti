# Feature switches

Some behaviours can be turned off without changing code. Where each switch lives follows from who
can judge it.

| Level                       | For                                                                                              | Examples                                                              |
| --------------------------- | ------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| **Global** (env)            | Anything technical, which a supplier cannot evaluate, and anything whose value is not yet proven | The LLM review of matching; interpretation examples; providers        |
| **Account** (settings page) | A supplier's own business preference                                                             | The brand on the quote; invoicing with Coti                           |
| **Branch**                  | Only what genuinely differs between branches of one account                                      | Quote validity (`branch.default_expiry_days`), channels, price, stock |
| **None**                    | Correctness and measurement: a switch there could only break something                           | The `ai_usage` ledger, the learned-match guard                        |

Two rules follow:

- **A new feature that spends on a model, or that is still experimental, ships behind a global
  switch**, so it can be turned off with a configuration change and no deploy of code.
- **A switch moves to the account settings only when a supplier has a real reason to want
  something different**, never in advance. Matching and the catalog are account-scoped, so a
  matching switch is never per branch.

## The switches today

| Switch                                         | Default | Off means                                                      |
| ---------------------------------------------- | ------- | -------------------------------------------------------------- |
| `CATALOG_MATCH_REVIEW_MAX_LINES`               | `0`     | Flagged lines are left to the seller; no review calls          |
| `QUOTE_CORRECTION_MAX_INTERPRETATION_EXAMPLES` | `3`     | Extraction runs without past corrections, and no lookup        |
| `AI_LLM_PROVIDER` and its two siblings         | off     | The capability refuses; see [ai-providers.md](ai-providers.md) |

The review is off because its measured value does not yet repay its cost
([catalog.md](catalog.md#the-review-trade-knowledge-the-catalog-text-does-not-carry)). Interpretation
examples are on but unproven: they add uncached input to every extraction, and `ai_usage` is where
their cost is read before deciding.

Per account, set by an administrator on its settings page:

| Switch                                                      | Default                                                                 | Off means                                                                                                                                                                         |
| ----------------------------------------------------------- | ----------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| «Facturación electrónica» (`account.invoicing_enabled`)     | off; on for accounts that had ARCA set up or an invoice when it shipped | Nothing invoicing-related shows, and issuing is refused; issued invoices stay readable. See [accounts-and-branches.md](accounts-and-branches.md#invoicing-with-coti)              |
| «Facturar al marcar cobrado» (`account.invoice_on_payment`) | off                                                                     | «Facturar también» starts unticked when a sale is marked paid; each invoice is confirmed either way. See [accounts-and-branches.md](accounts-and-branches.md#invoicing-with-coti) |
