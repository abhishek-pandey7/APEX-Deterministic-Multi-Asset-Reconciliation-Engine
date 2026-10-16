# 0006. Audit rules enforced by the database

Status: Accepted

## Context

Breaks are resolved by people, and resolutions such as write-offs move money on paper. Operations
controls require a complete history of who did what and why, a fixed workflow, and a second person
to approve any proposed resolution (four-eyes). Rules enforced only in application code can be
bypassed by any other client, a script, or a bug.

## Decision

The database enforces the rules itself:

- `break_events` is append-only: triggers reject `UPDATE`, `DELETE` and `TRUNCATE`. A break's
  current status is its latest event. Engine output (`matches`, `match_legs`) is protected the same way.
- A `BEFORE INSERT` trigger enforces the state machine
  (OPEN → ASSIGNED → PROPOSED → APPROVED → CLOSED, with REJECTED sending a break back) and refuses
  an approval by the person who made the proposal. It locks the break row first, so two concurrent
  transitions can't both read the same prior state.
- An entry can be auto-matched at most once per run, guaranteed by a partial unique index.

The API simply relays the database's refusal as HTTP 409.

## Alternatives considered

| Option | Why not |
|---|---|
| Enforce in the Go service only | Any other path to the database (a script, a future service, a manual fix) skips the check. |
| Event-sourcing infrastructure (EventStoreDB, Kafka log) | Gives an immutable log but moves the source of truth out of the transactional store, so state changes and their audit records stop being atomic. |
| Mutable status column plus a history table | Two places to keep consistent; the status can be edited without a matching history row. |

## Consequences

- No client, including this project's own API, can skip four-eyes or rewrite history.
- SQL tests in CI exercise every rule. Writing them caught a real bug: comparisons with a NULL
  previous status made the whole check NULL, which `IF` treats as passing.
- The rules live in PL/pgSQL, a second language to maintain.
- TRUNCATE bypasses row triggers; statement-level triggers were added once the gap was noticed
  (migration 5).
