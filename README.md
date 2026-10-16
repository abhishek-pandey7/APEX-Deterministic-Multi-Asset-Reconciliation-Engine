# APEX: Deterministic Multi-Asset Reconciliation Engine

APEX reconciles an internal ledger's cash and positions against custodian feeds and turns every
difference into a break an operations analyst can resolve.

- **Deterministic.** The same inputs, rule version and configuration always produce byte-identical
  results, so a past day can be rerun for an auditor.
- **Auditable.** Break history is an append-only event log, and the database itself enforces the state
  machine and the two-person approval rule.
- **Measured.** A seeded generator injects known breaks and writes the correct answer for each, so the
  engine is evaluated on precision and recall.
- **AI where it helps.** Matching is deterministic. AI only classifies and explains the breaks left over,
  and never changes a match.

## Status

| Area | State |
|---|---|
| Fixed-point money (`internal/money`) | Done: ISO 4217 exponents, float-free parsing, half-even market value, fuzz tested |
| Postgres schema (`migrations/`) | Done: ingestion, runs, matches, breaks, append-only audit, job queue |
| Synthetic data (`tools/apexgen`) | Done: 11 cash and 8 position scenarios with ground truth |
| Ingestion (`internal/ingest`, `cmd/apex`) | Done: layouts per source, row validation, file hashing, COPY loading |
| Matching engine (`internal/match`) | Exact and tolerance phases done; N-to-M and fuzzy next |
| Evaluation (`internal/eval`, `apex eval`) | Done: scores runs against ground truth, CI quality gate |
| Position reconciliation | Planned |
| API, SSE and UI | Planned |
| AI break worker | Planned |

## Quick start

Needs Go, Python 3.12+ and Docker.

```
make db-up          # Postgres + migrations
make db-test        # audit rule tests in SQL
make check          # go vet, go test -race, generator tests
make gen            # data/small: about 22k cash rows with truth files
make load           # ingest data/small into Postgres
make test-int       # tests against generated data and the database
make eval           # match data/small and score it against the truth file
```

The database listens on host port 55432, so it doesn't collide with a local Postgres.

Without `make`, see the [Makefile](Makefile) for the underlying commands.

## Results so far

`apex eval` on the 770k-row medium dataset (seed 42), exact and tolerance phases only:

| | |
|---|---|
| Throughput | about 2M rows/s on 16 workers |
| Wrong auto matches | 0 of 320,456 |
| Recovered: exact, timing (1–2 business days), rounding (≤ 0.99) | 100% |
| Breaks left open: missing on either side, amount mismatch | 99.6–100% |
| Not yet recovered: splits, netting, many-to-many, mangled references | 0% (N-to-M and fuzzy phases) |

Every run is checked for determinism: the same data, shuffled and matched on one worker, must
produce the same result hash.

## Layout

```
cmd/apex/           command-line entry point (apex ingest, apex eval, apex layouts)
internal/domain/    normalised cash entries and positions
internal/eval/      scoring against ground truth
internal/ingest/    file layouts and parsing
internal/match/     matching engine
internal/money/     fixed-point amounts, quantities, prices
internal/store/     Postgres persistence
migrations/         versioned Postgres schema (golang-migrate)
tests/sql/          database-level tests
tools/apexgen/      synthetic data generator (Python, stdlib only)
docs/               architecture and algorithm research
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md): system design, stack, numeric representation, determinism rules
- [Algorithms](docs/ALGORITHMS.md): blocking, assignment, subset-sum, Fellegi–Sunter, evaluation, with references
- [Generator](tools/apexgen/README.md): scenarios and output files
