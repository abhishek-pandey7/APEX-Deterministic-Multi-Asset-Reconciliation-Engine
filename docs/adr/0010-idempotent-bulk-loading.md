# 0010. Idempotent bulk loading with file hashes and COPY

Status: Accepted

## Context

Files arrive by SFTP and get re-sent, re-dropped and re-run. Loading the same file twice would
double every entry and fabricate breaks. Files are large (hundreds of thousands of rows), so
loading has to be fast, and it has to be all-or-nothing (ADR 0009).

## Decision

- The file is hashed (SHA-256) while it is parsed. `source_files.sha256` is unique, so a second load
  of the same bytes is a no-op reported as "already loaded".
- The file record and all its rows are written in one transaction with Postgres `COPY`. A failure
  rolls back both, so a failed file can simply be retried.
- Rows are sorted into index order before `COPY`. Custodian files arrive shuffled, and random-order
  inserts into the `(account, currency, value_date)` index were the bottleneck: sorting cut a
  392k-row load from 43.8 s to 22.3 s.

## Alternatives considered

| Option | Why not |
|---|---|
| Deduplicate by file name | Custodians reuse names; a corrected file with the same name is new data. |
| Deduplicate by timestamp or a header sequence number | Not every format has one, and they can't be trusted to be unique. |
| Batched `INSERT` statements | Several times slower than `COPY` for bulk data. |
| Drop indexes during load, rebuild after | Fast for one-off bulk imports, but rebuilding the full index after each daily file costs more than it saves. |

## Consequences

- Re-delivering a file is always safe.
- Byte-identical content is treated as the same file, even if it was intentionally sent twice.
  That's the right default; an operator can override it by deleting the file record.
- The remaining gap between ledger (10.6 s) and custodian (22.3 s) loads comes from the unique
  `(side, external_id)` index, which sorting by account doesn't help. Accepted.
