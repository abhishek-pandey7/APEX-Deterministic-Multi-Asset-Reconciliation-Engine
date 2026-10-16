# 0005. Determinism through partitioning and canonical order

Status: Accepted

## Context

An auditor may ask for a past day to be re-run. If the same inputs can produce different matches,
the reconciliation can't be defended, even when each answer is individually plausible. Parallel
code is the usual source of non-determinism: shared candidate pools, scheduling-dependent ordering,
and in Go specifically, randomised map iteration.

## Decision

- **Partition by (account, currency).** An entry can only ever match within its own partition, so
  partitions share no state. Each is processed by exactly one goroutine; there are no locks on the
  matching path.
- **Canonical order everywhere.** Inside a partition, entries are sorted by (value date, amount,
  external id) before any loop whose outcome depends on order. Partition results are merged in
  sorted key order. Any map is iterated through sorted keys.
- **Deterministic tie-breaks.** Equal candidates are ordered by external id, never by arrival order.
- **Integer scores** (ADR 0012), because Go may fuse floating-point multiply-adds differently on
  different CPUs.
- **Proof, not intention.** Every evaluation re-runs the data shuffled on one worker and compares
  the result hash; a unit test does the same with 1–13 workers; CI runs the race detector.

## Alternatives considered

| Option | Why not |
|---|---|
| A shared worker pool over all entries with locks | Results depend on scheduling: two workers can claim the same candidate in either order. |
| Sequential processing | Deterministic but leaves cores idle; partitioning gives both. |
| Accept non-determinism and log the outcome | Logging what happened doesn't make it reproducible, and a different rerun undermines every result. |

## Consequences

- The determinism check caught a real bug while building N-to-M matching: multi-leg matches
  listed their legs in input order. It was fixed before it reached `main`.
- Partitioning rules out matches across accounts or currencies. That is correct for cash
  reconciliation; FX matching across currencies would need a different partition key.
