# 0018. Standard-library HTTP API with SSE progress

Status: Accepted

## Context

The web UI needs to list runs and breaks, show a break's evidence and history, move breaks through
the review workflow, and start a run while showing its progress live. A run takes from under a
second to about 20 seconds.

## Decision

- `net/http` with Go 1.22+ method-and-path routing (`GET /api/runs/{id}/breaks`), no framework.
- `POST /api/runs` starts a run in the background and returns a job. `GET /api/jobs/{id}/events`
  streams the job's progress as **Server-Sent Events**: the whole history so far, then live events,
  with a keep-alive comment every 15 seconds.
- Only one run at a time; a second request gets 409.
- The current status of each break is found with a `LATERAL` join to its newest event through the
  `(break_id, id)` index.
- Database refusals (an invalid transition, a self-approval) are returned as 409 with the database's
  own message.
- The acting user comes from an `X-Actor` header.

## Alternatives considered

| Option | Why not |
|---|---|
| Gin, Echo or chi | The standard library now routes by method and path parameter, which covers everything here without a dependency. |
| WebSockets for progress | Two-way channel, connection state and a library, for a stream that only ever flows server to client. SSE is plain HTTP and the browser reconnects automatically. |
| Polling a status endpoint | Adds latency and load, and the UI would miss the short stages of a fast run. |
| Running the reconciliation inside the request | Ties the run to one HTTP connection; a closed tab would cancel it. |
| The `break_status` view (`DISTINCT ON` over all events) | Scans every event ever written; the `LATERAL` lookup stays fast as runs accumulate. |
| Real authentication now | Out of scope for the demo. **This is a known gap:** `X-Actor` is trusted, so four-eyes is only as strong as the caller's honesty about identity. Production needs SSO, with the database role or session carrying the user. |

## Consequences

- No web framework dependency; the server is about 300 lines.
- Listing a page of breaks from a 51k-break run takes 60–320 ms, mostly the per-status counts.
  Fine now; a counts cache would be the next step.
- The job registry lives in memory, so a server restart forgets running jobs (the run itself is
  marked FAILED or completes in the database).
