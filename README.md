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
| Matching engine (`internal/match`) | Exact, tolerance and N-to-M phases done; fuzzy next |
| Break classification (`match.Classify`) | Done: six categories with a reason per break, 99.8–100% accurate |
| Stored runs (`internal/recon`, `apex run`) | Done: runs, matches, breaks and their audit events saved in one transaction; 770k rows in 17 s |
| HTTP API (`internal/api`, `apex serve`) | Done: runs, live progress over SSE, break review workflow |
| Evaluation (`internal/eval`, `apex eval`) | Done: scores runs against ground truth, CI quality gate |
| Position reconciliation | Planned |
| Web UI | Next |
| AI break worker | Planned |

## Quick start

Needs Go, Python 3.12+ and Docker.

```
make db-up          # Postgres + migrations
make db-test        # audit rule tests in SQL
make check          # go vet, go test -race, generator tests
make gen            # data/small: about 22k cash rows with truth files
make load           # ingest data/small into Postgres
make run            # reconcile everything loaded and store the run
make serve          # API on localhost:8080
make test-int       # tests against generated data and the database
make eval           # match data/small and score it against the truth file
```

The database listens on host port 55432, so it doesn't collide with a local Postgres.

Without `make`, see the [Makefile](Makefile) for the underlying commands.

## Results so far

`apex eval` with the exact, tolerance and N-to-M phases:

| | small (44k rows) | medium (770k rows, denser) |
|---|---|---|
| Throughput (16 workers) | about 1.2M rows/s | about 0.9M rows/s |
| Wrong auto matches | 0 of 21,925 | 2 of 371,582 |
| Exact, timing, rounding, splits | 100% | 100% |
| Netted many-to-many | 100% | 99.4% |
| Fee batches (amount evidence only) | 77.9% | 45.0% |
| Breaks left open | 100% | 99.6–100% |
| Break category accuracy | 100% (1,360) | 99.8% (24,114) |
| Mangled references outside the date window | 0% (fuzzy phase next) | 0% |

Fee batches carry no reference, so a batch is matched only when an exhaustive search finds
exactly one combination and no other batch competes for its entries. On busy days that is often
not provable, and the batch is left open for review rather than guessed. Every run is also checked
for determinism: the same data, shuffled and matched on one worker, must hash the same.

## API

`apex serve` listens on `localhost:8080`.

| Method and path | Purpose |
|---|---|
| `GET /api/runs` | Recent runs with their stats and output hash |
| `POST /api/runs` | Start a run in the background (`{"file_ids": [...]}` optional); returns a job |
| `GET /api/jobs/{id}/events` | The job's progress as Server-Sent Events: `load`, `match`, `classify`, `save`, `done`, then `summary` or `failed` |
| `GET /api/runs/{id}/breaks` | One page of breaks, filtered by `category`, `status`, `account`, plus counts |
| `GET /api/breaks/{id}` | A break with its entries and full audit history |
| `POST /api/breaks/{id}/events` | Move a break through the workflow (`{"status": "PROPOSED", "reason": "..."}`), acting as the `X-Actor` header |

Invalid transitions and self-approvals are refused by the database's own triggers and come back
as `409`, so no client, including this API, can skip the four-eyes check.

```
curl -X POST localhost:8080/api/breaks/95/events -H 'X-Actor: ana' -d '{"status":"ASSIGNED"}'
```

## Layout

```
cmd/apex/           command-line entry point (ingest, run, serve, eval, layouts)
internal/api/       HTTP API and progress streaming
internal/domain/    normalised cash entries and positions
internal/eval/      scoring against ground truth
internal/ingest/    file layouts and parsing
internal/match/     matching engine
internal/money/     fixed-point amounts, quantities, prices
internal/recon/     a full run: load, match, classify, store
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
