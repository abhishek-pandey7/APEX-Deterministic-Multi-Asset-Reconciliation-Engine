# 0014. Bounded subset search for N-to-M, with provable uniqueness

Status: Accepted

## Context

One ledger entry can settle as several custodian lines (a split), several fees can post as one
line (a batch), and netted trades can be many-to-many. Finding subsets whose totals agree is subset
sum, which is NP-complete; J.P. Morgan AI Research formalise reconciliation exactly this way
(arXiv:2508.19218).

## Decision

- **Bounded enumeration with pruning**: at most 4 entries per side and 24 candidates per search.
  Values are searched largest first, skipping values that overshoot and stopping when the largest
  remaining values can't reach the target. A worst-case search takes microseconds.
- **Reference-anchored first**: lines carrying a ledger entry's reference form a split, or a netted
  group together with same-day ledger entries, with an exact total.
- **Batches by amount only, under strict rules**: an exact total, every leg on the same value date,
  no specific identifier in the batch line, a unique combination, and no other batch competing for
  the same entries.
- **Uniqueness must be provable**: if candidates exceed the cap, the search can't be exhaustive, so
  nothing is matched.

## Alternatives considered

| Option | Why not |
|---|---|
| Dynamic programming over sums | Exact and polynomial in the sum range, but minor-unit sums run to billions; a dense table is impractical and the sparse version gives no benefit at these group sizes. |
| Integer programming (CPLEX, OR-Tools) | Optimal when it converges, but the JPM benchmark shows it timing out on most instances with 25+ items. |
| Meet-in-the-middle | Halves the exponent, but with groups capped at 4 the direct search is already fast. |
| Resolve batches one at a time | Measured: an early wrong pick consumed a real member, and a later batch then "uniquely" matched a stand-in. Batches are now found together and contested ones dropped. |
| Capping candidates and still auto-matching | "Unique among 24 capped candidates" isn't unique; this caused 570 wrong auto-matches on the dense dataset. |
| Propose ambiguous amount-only groups | Proposals consumed entries that should have stayed breaks. Ambiguous batches now stay open. |

## Consequences

- Wrong auto-matches on the 770k-row dataset went 570, then 7, then 2.
- Splits and netting are recovered at 99–100%. Fee batches drop to 45% on busy days, where
  uniqueness can't be proven, and are left as open breaks. That trade is deliberate: an open break
  costs an analyst minutes; a wrong auto-match is a misstatement nobody reviews.
- Splits without any reference aren't attempted.
