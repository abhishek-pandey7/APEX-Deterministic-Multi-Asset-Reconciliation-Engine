# Architecture decision records

Each record explains one decision: the problem, what was chosen, the alternatives that were
considered and why each was rejected, and what the choice costs. Records are never rewritten
after the fact; a changed decision gets a new record that supersedes the old one.

| # | Decision | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-go-for-the-engine-and-api.md) | Go for the engine and the API | Accepted |
| [0003](0003-postgres-for-storage-audit-and-queue.md) | PostgreSQL for storage, audit and the job queue | Accepted |
| [0004](0004-fixed-point-integers-for-money.md) | Fixed-point integers for money | Accepted |
| [0005](0005-determinism-by-partitioning.md) | Determinism through partitioning and canonical order | Accepted |
| [0006](0006-audit-rules-in-the-database.md) | Audit rules enforced by the database | Accepted |
| [0007](0007-plain-sql-migrations-and-pgx.md) | Plain SQL migrations and pgx, no ORM | Accepted |
| [0008](0008-synthetic-data-with-ground-truth.md) | Synthetic data with ground truth | Accepted |
| [0009](0009-layout-driven-ingestion.md) | Layout-driven ingestion that rejects whole files | Accepted |
| [0010](0010-idempotent-bulk-loading.md) | Idempotent bulk loading with file hashes and COPY | Accepted |
| [0011](0011-matching-in-phases.md) | Matching in phases, most certain first | Accepted |
| [0012](0012-identifier-aware-reference-similarity.md) | Identifier-aware reference similarity | Accepted |
| [0013](0013-greedy-assignment-with-ambiguity-check.md) | Greedy assignment with an ambiguity check | Accepted |
| [0014](0014-bounded-subset-search-for-n-to-m.md) | Bounded subset search for N-to-M, with provable uniqueness | Accepted |
| [0015](0015-quality-measured-in-ci.md) | Matching quality measured against ground truth in CI | Accepted |
| [0016](0016-rules-before-ai.md) | Rule-based break classification; AI advisory only | Accepted |
| [0017](0017-runs-as-reproducible-snapshots.md) | Runs stored as reproducible snapshots | Accepted |
| [0018](0018-standard-library-http-and-sse.md) | Standard-library HTTP API with SSE progress | Accepted |
