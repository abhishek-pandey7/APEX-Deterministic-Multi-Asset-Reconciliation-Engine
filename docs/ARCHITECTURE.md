# Architecture

APEX reconciles two views of the same economic reality, the internal ledger (books and records) and the
custodian (the bank holding the assets), and reports every difference as a *break* that an operations
analyst can resolve.

Two properties shape every decision below:

1. **Determinism.** The same inputs, rule version and configuration must always produce byte-identical
   match results. Auditors rerun past days, so a reconciliation that can't be reproduced is wrong even if
   its answer is right.
2. **Auditability.** Every match and every break state change records who or what produced it, when, under
   which rule version, and from which input files.

## System overview

```
Custodian CSV/SFTP ─┐
Internal ledger ────┤
                    ▼
        Go ingestion (file hash, per-custodian mapping, validation)
                    ▼
        Postgres: source_files, cash_entries, positions
                    ▼
        Go matching engine: one goroutine per (account, currency) partition
          1. Exact      hash lookup
          2. Tolerance  sorted amount index + binary search, business-day window
          3. N-to-M     bounded subset-sum inside the partition
          4. Fuzzy      scored candidates, proposed for human review only
                    ▼
        Postgres: match_runs, matches, breaks, break_events (append-only)
             │                          │
             ▼                          ▼
   Python AI worker              Go REST API + SSE ──► React UI
   (jobs via SKIP LOCKED:        (break queue, two-person approval,
    classify, explain,            aging, audit history)
    similar past breaks)
```

## Stack

| Component | Technology | Reason |
|---|---|---|
| Ingestion, matching, API | Go | Strict types, a single binary, cheap goroutines for per-partition parallelism and file I/O |
| Storage, audit, job queue | PostgreSQL 17 + pgvector | ACID guarantees, append-only audit tables enforced by triggers, `FOR UPDATE SKIP LOCKED` queue, vector search for similar past breaks |
| DB access / migrations | pgx v5, golang-migrate | Plain SQL with no ORM, versioned schema |
| AI worker | Python | LLM and embedding ecosystem; isolated from the deterministic path |
| Synthetic data | Python (stdlib only) | Seeded generator that writes known correct answers for evaluation |
| UI | React + TypeScript + Vite + Tailwind + TanStack Table | Virtualised grid for large break queues |
| Live progress | Server-Sent Events | One-way progress stream with no WebSocket state to manage |
| Observability | `log/slog`, Prometheus metrics | Structured logs, per-phase timings and match rates |

Redis is deliberately absent. Postgres already provides the queue and job state, and reference data fits
in process memory. Each extra component needs a reason to exist.

## Numeric representation

No floating point appears on the matching path.

| Quantity | Representation | Notes |
|---|---|---|
| Cash amount | `int64` minor units, scale from the ISO 4217 exponent | USD 2, JPY 0, BHD 3 |
| Position quantity | `int64` fixed point, scale 10⁻⁶ | Fractional shares, bond face value |
| Price | `int64` fixed point, scale 10⁻⁸ | |
| Market value | quantity × price in 128-bit arithmetic, rounded half-even to currency minor units | Avoids `int64` overflow |

Decimal strings are parsed directly into integers. A value with more fractional digits than its scale
allows is rejected, never silently rounded.

## Determinism rules

- **Partitioning.** Work is split by `(account_id, currency)`. Each partition is owned by exactly one
  goroutine, with no shared mutable state and no locks on the matching path.
- **Ordered iteration.** Go randomises map iteration order, so any loop whose result depends on order
  iterates a sorted slice of keys.
- **Stable tie-breaking.** Equal scores are broken by `(date, amount, source id)`, never by arrival order.
- **Integer scores.** Match scores are stored in integer millibits. Go may fuse floating-point
  multiply-adds on some architectures, so float scores could differ between machines.
- **Pinned inputs.** Each `match_run` stores the engine version, the rule version, the configuration
  (`jsonb`) and the SHA-256 of every input file.
- **Proof.** CI runs the engine twice on the same generated dataset and compares the output hashes, and
  `go test -race` covers the partitioned scheduler.
- **AI output is advisory.** LLM output is not deterministic, so it never creates, removes or changes a
  match. Suggestions are stored with the model id and prompt hash.

## Matching phases

The phases run from cheapest and most certain to most expensive and least certain. Each phase only sees
records that earlier phases left unmatched. Algorithm detail and alternatives are in
[ALGORITHMS.md](ALGORITHMS.md).

| Phase | Finds | Outcome |
|---|---|---|
| 1. Exact | Same account, currency, amount and value date, references agree | Auto-matched |
| 2. Tolerance | Value date within N business days, amount within an absolute/relative tolerance | Auto-matched if exactly one best candidate, otherwise proposed |
| 3. N-to-M | Splits and netting anchored by reference; fee batches by exact same-day totals | Auto-matched if the solution is unique; ambiguous batches stay open |
| 4. Fuzzy | Scored candidates using reference, counterparty, amount and date evidence | Proposed only; a human confirms |

Anything left after phase 4 becomes a break.

## Break workflow

Break state lives in `break_events`, an append-only table that a trigger protects from `UPDATE` and
`DELETE`. The current state is the latest event per break.

```
OPEN ─► ASSIGNED ─► PROPOSED ─► APPROVED ─► CLOSED
                       │
                       └─► REJECTED ─► ASSIGNED
```

Force-matches and write-offs need two people: the approver must differ from the proposer. Breaks are aged
into 0–1, 2–5 and 5+ business-day buckets, with a materiality threshold per account.
