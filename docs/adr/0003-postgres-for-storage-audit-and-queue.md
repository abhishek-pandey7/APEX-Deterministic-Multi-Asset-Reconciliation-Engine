# 0003. PostgreSQL for storage, audit and the job queue

Status: Accepted

## Context

The system stores normalised inputs, match results, breaks, an audit trail that must be
tamper-resistant, and a queue of work for the AI worker. Financial records have rigid schemas
(dates, amounts, currencies, account ids) and need transactional guarantees: a run's matches
and breaks must appear together or not at all.

## Decision

Use PostgreSQL 17 (with pgvector available for later) for all of it:

- relational tables with check constraints and foreign keys for inputs and results;
- triggers for the audit rules (ADR 0006);
- a `jobs` table consumed with `SELECT ... FOR UPDATE SKIP LOCKED` as the work queue.

## Alternatives considered

| Option | Why not |
|---|---|
| MongoDB or another document store | The data is relational (entries, matches, legs, breaks, events) and needs foreign keys and multi-table transactions. Schema flexibility is a liability here: a missing currency field should be rejected, not stored. |
| Redis for the queue and job state | One more system to run, secure and back up, for a queue Postgres already provides. `SKIP LOCKED` lets many workers poll without blocking each other (tested: a second worker skips a locked job immediately). Redis would be worth it at a much higher job rate. |
| Kafka or RabbitMQ | Built for high-volume event streams between many services. This system has one producer, one consumer type and a handful of jobs per run. A broker would add operations without solving a problem we have. |
| A separate ledger database (immudb, QLDB) for the audit trail | Stronger tamper evidence, but splits one transaction across two systems. Triggers in the same database keep the audit write atomic with the change it records. |
| SQLite | No concurrent writers, no `SKIP LOCKED`, no pgvector. |

## Consequences

- One stateful dependency to operate.
- Queue throughput is bounded by Postgres, which is far above what this workload needs.
- The audit trail is as strong as database access control: a superuser can still drop triggers.
  Hash-chaining the events would make tampering detectable; not done yet.
