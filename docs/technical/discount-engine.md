# Discount engine

Promotions are typed rules an account defines; the engine applies them to a quote version
deterministically. The AI never computes an amount: every figure on `quote_discount.amount` comes
from the code described here. The evaluator is `internal/services/discount_engine.go` (pure, no I/O)
and the sweep that persists its result is `internal/services/discount_sweep.go`.

## Rules

A `promotion` belongs to an account and optionally one branch (`branch_id` null = every branch). It
applies while `is_active` and `valid_from <= now <= valid_to` (either bound may be null). There is no
"reactivatable" flag: deactivating and reactivating is `is_active` or the window changing.

| `condition_type`  | Targets (`promotion_condition_item`)  | Figure                          | Scope of the application |
| ----------------- | ------------------------------------- | ------------------------------- | ------------------------ |
| `PER_ITEM`        | one or more                           | `action_value`                  | `ITEM`, one per line     |
| `QUANTITY_TIERED` | one or more                           | the tier the line quantity hits | `ITEM`, one per line     |
| `ITEM_SET`        | two or more, every one present        | `action_value`                  | `ITEM_SET`, one per rule |
| `ON_TOTAL`        | none; optional `min_total` on the net | `action_value`                  | `TOTAL`                  |

A condition names exactly one of a product, a family or a subgroup, with an optional
`min_quantity` the line must reach. Tiers are inclusive on both bounds and may not overlap; the last
may be open-ended.

How an action turns into money:

- **Line rules transform the unit price:** `PERCENTAGE` takes a share of the line subtotal,
  `FIXED_AMOUNT` takes the figure off each unit (figure × quantity), and `SPECIAL_PRICE` replaces the
  unit price (subtotal − special price × quantity; nothing when the special price is higher).
- **Set and total rules act on their base:** `PERCENTAGE` of it, or `FIXED_AMOUNT` once.
  `SPECIAL_PRICE` is refused for them.

## Resolving conflicts

Discounts do not stack. Every candidate application is ranked by priority (higher first), then
the larger amount, then the promotion id and the first line id, so equal rules resolve the same way
on every run.

- A line takes at most one line or set promotion; the first ranked candidate claims it.
- An **exclusive** line or set promotion also keeps its lines out of any `ON_TOTAL` base.
- An **exclusive** `ON_TOTAL` applies alone, on the gross, and only when nothing ranked above it
  claimed a line or would itself apply to the total.
- At most one `ON_TOTAL` applies, the first ranked whose threshold the net reaches.
- `ON_TOTAL` is computed on the **net**: the subtotals of the lines no exclusive rule holds, less
  every line discount on them — the engine's own and the seller's.

## Floor and sign

The evaluator reads only what the line froze when it was valued (`unit_price_snapshot`,
`min_price_snapshot`, `subtotal`), never `product_price`. A line may come down to
`round(min_price_snapshot × quantity)` and no further; a null floor means no floor, never a floor of
zero. No application is negative, the total rule stops at the remaining headroom, and the version
total bottoms out at zero.

The floor binds the engine, not people: a seller can still price a line below it by hand.

## Rounding

One rule, shared with invoicing: money is rounded **half-up to two decimals** (`decimal.Round(2)`,
half away from zero on non-negative values), **once per application** when its amount is closed, and
the version total once more after subtracting. Nothing in between rounds.

## The sweep

`discountSweep.settle` runs inside the caller's transaction:

1. on **Aceptar materiales** (`DRAFT`/`CHANGE_REQUESTED` → `QUOTED`), right after the lines are
   priced;
2. after every seller change to a line (add, edit quantity, price or product, delete) and to a
   discount (add, edit, suppress, restore, delete).

It recomputes seller percentage discounts against their lines, runs the evaluator, and reconciles
the `AUTOMATIC` rows: an application still earned keeps its row (amount and links updated in place),
one no longer earned is deleted, a new one is inserted. Re-pricing is never a side effect: only
accepting the materials again freezes new prices.

## Origin and suppression

`quote_discount.origin` says who decided a discount:

| Origin          | Written by                      | Seller can                                       |
| --------------- | ------------------------------- | ------------------------------------------------ |
| `AUTOMATIC`     | the sweep                       | suppress and restore; not edit                   |
| `MANUAL_SELLER` | `POST /v1/quotes/:id/discounts` | edit and delete                                  |
| `AI_ADAPTATION` | the AI adaptation strategy      | suppress and restore; never touched by the sweep |

A suppressed application stays on the version (`suppressed_by_seller = true`) and the sweep does not
re-apply it: a line application is identified by promotion and line, a set or total one by
promotion. The next-ranked rule may take its place. Restoring an application the quote no longer
earns answers `409 DISCOUNT_NOT_APPLICABLE`.

## Administration API

Admin-only, account-scoped:

| Route                    | Purpose                                      |
| ------------------------ | -------------------------------------------- |
| `GET /v1/promotions`     | every rule of the account, inactive included |
| `GET /v1/promotions/:id` | one rule with conditions and tiers           |
| `POST /v1/promotions`    | create a rule                                |
| `PUT /v1/promotions/:id` | replace the whole rule                       |

A rule naming a branch or product outside the account is refused with `422`. Changing a rule reaches
a quote at its next sweep; a frozen version keeps the discounts it was sent with.
