# 0007. Plain SQL migrations and pgx, no ORM

Status: Accepted

## Context

Most of this system's guarantees live in the schema: check constraints, partial unique indexes,
triggers, append-only rules (ADR 0006). The hot paths are bulk loads of hundreds of thousands of
rows. Schema changes must be reviewable and reversible.

## Decision

- Schema changes are numbered, hand-written SQL files with matching down migrations, applied by
  golang-migrate. CI applies them up, all the way down, and up again on every push.
- Go talks to Postgres through pgx v5 directly: explicit SQL, `COPY` for bulk writes,
  `pgx.CollectRows` for reads.

## Alternatives considered

| Option | Why not |
|---|---|
| GORM or another ORM with auto-migrations | Hides the triggers, partial indexes and constraints the design depends on, and generates schema changes nobody reviewed. Bulk `COPY` and `LATERAL` joins end up as raw SQL anyway. |
| sqlc (SQL compiled to typed Go) | A good fit and a likely later step; skipped for now to avoid a code-generation step while queries were changing quickly. |
| goose or Atlas for migrations | Both fine. golang-migrate has a small, ubiquitous CLI and Docker image, which CI uses without installing anything. |
| `database/sql` with lib/pq | pgx is faster, supports `COPY` natively and maps Postgres types (arrays, dates) more precisely. |

## Consequences

- Every query is visible and reviewable as SQL.
- Down migrations are tested, so a bad release can be rolled back.
- Query results are mapped by hand, which is more code than an ORM, but each mapping is small.
