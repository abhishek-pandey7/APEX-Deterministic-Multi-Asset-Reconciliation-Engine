# 0004. Fixed-point integers for money

Status: Accepted

## Context

Reconciliation compares amounts for equality and within tolerances of one minor unit. Binary
floating point cannot represent 0.10 exactly, so sums drift and equal amounts can compare unequal.
The data is multi-currency and multi-asset: JPY has no decimal places, KWD has three, share
quantities can be fractional, and prices need more precision than cash.

## Decision

Every value on the matching path is an `int64` with a fixed scale:

| Value | Scale |
|---|---|
| Cash amount | the currency's ISO 4217 exponent (USD 2, JPY 0, KWD 3) |
| Quantity | 10⁻⁶ |
| Price | 10⁻⁸ |
| Market value | quantity × price in big-integer arithmetic, rounded half to even into the currency's minor units |

Decimal strings are parsed straight into integers. More fractional digits than the scale allows is
an error, not a silent rounding (trailing zeros excepted).

## Alternatives considered

| Option | Why not |
|---|---|
| `float64` | Inexact for decimal fractions; equality and hashing of amounts become unreliable. |
| `int64` cents everywhere | Wrong for JPY (no cents) and KWD (three decimals), and for quantities and prices. |
| A decimal library (`shopspring/decimal`) | Correct, but heap-allocated values and slower comparisons in hot loops, and its rounding defaults must be audited. Integers with an explicit scale are simpler to reason about and to hash. |
| `math/big.Rat` throughout | Exact but far too slow for the matching path. Used only for the one multiplication that can overflow. |

## Consequences

- Amount comparisons and sums are exact and fast.
- The parser and formatter are fuzz-tested together (5.5M inputs, round trip must be exact).
- A currency outside the supported table is rejected rather than guessed.
- The Python generator mirrors the same rounding, and a shared test vector checks they agree.
