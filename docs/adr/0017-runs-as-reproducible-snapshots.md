# 0017. Runs stored as reproducible snapshots

Status: Accepted

## Context

A stored reconciliation must answer, months later: which inputs, which rules, which engine, and
would running it again give the same answer? Saving 336k matches and 51k breaks must also be fast
and atomic.

## Decision

- Each run records the engine version, a **rule version** (a hash of every setting that can change a
  result; the worker count is excluded because it never does), the full configuration as JSON,
  the input file ids, and the **output hash** of the result.
- Matches, legs, breaks, break legs and every break's initial `OPEN` audit event are written in one
  transaction. Ids are reserved from the sequences in a single round trip so that every table can
  be bulk-loaded with `COPY`; the audit triggers still check each row.
- A failure after the run is recorded marks it `FAILED` rather than leaving it `RUNNING`.
- Each run is a snapshot: its own matches and its own breaks.

## Alternatives considered

| Option | Why not |
|---|---|
| Insert rows one by one with `RETURNING id` | Hundreds of thousands of round trips. The full medium run saves in about 15 s with `COPY`. |
| Store only the breaks | Loses the evidence for why everything else was considered settled. |
| Update one live set of breaks in place | Conflicts with append-only audit (ADR 0006), and a re-run would silently rewrite history. |
| Carry breaks forward between runs | The right next step for daily operation: close yesterday's break when today's run matches it. Not built yet, so each run's breaks start fresh. |

## Consequences

- Reproducibility is checked, not assumed: the same CSV files matched in memory (`apex eval`) and
  matched from Postgres (`apex run`) produce the same output hash, and a test runs the same files
  twice and compares.
- Saving dominates run time (15 of 17.5 s for 770k rows).
- Test runs leave rows behind, because matches and events can't be deleted by design.
