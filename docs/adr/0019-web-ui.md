# 0019. A React single-page app served by the Go binary

Status: Accepted

## Context

Analysts need to work a queue of tens of thousands of breaks: filter it, open one, see the evidence
on both sides, and move it through the review workflow with a second person approving. The UI talks
only to the API (ADR 0018) and should ship with the engine, not as a separate deployment.

## Decision

- **React + TypeScript, built with Vite.** TanStack Query handles fetching and cache invalidation;
  TanStack Table (v8) renders the queue, with paging done by the server.
- **Tailwind CSS v4** with a small set of colour tokens that carry meaning: steel blue is always the
  ledger, bronze always the custodian, red always a difference. Light and dark themes come from the
  same tokens.
- **The break detail is a T-account**: ledger on the left, custodian on the right, the difference in
  red in the gutter between them, the way an accountant would draw it.
- **The analyst's vocabulary, not the database's**: "Settled late", "Awaiting approval", not `TIMING`
  or `PROPOSED`. One mapping file keeps each action's name the same on every screen.
- **`apex serve` serves the built app** from `web/dist`, falling back to `index.html` so links like
  `?run=3&break=95` open straight onto a break. Without a build, it serves the API only.
- **The font is bundled** (`@fontsource`), so the app works on a network without internet access.
- **Amounts are displayed with thousands separators** (`money.FormatGrouped`), while machine-readable
  output keeps the plain format that the strict parser reads back (ADR 0004).
- **"Acting as" stands in for login**, so the four-eyes rule can be demonstrated by switching user.

## Alternatives considered

| Option | Why not |
|---|---|
| Next.js | Server rendering and routing aren't needed for an internal tool behind a login, and they would add a Node server next to the Go one. |
| Server-rendered Go templates with htmx | Less JavaScript, but the detail panel, live progress and optimistic filtering are easier to build as client state. A reasonable choice for a smaller UI. |
| Embedding the build in the binary with `go:embed` | Makes the Go build depend on a frontend build being present, which complicates CI and local development. Serving from a directory keeps them independent; embedding is easy to add for release builds. |
| TanStack Table v9 | Installed by default, but it was released recently and its API differs from v8, which is well documented and known. Pinned to v8 rather than guessed at. |
| A component kit (MUI, shadcn/ui) | Fast to start, but the result looks like every other admin panel. A dozen hand-made components cover what this tool needs. |
| Disabling "Approve" for the proposer | Better day-to-day UX, but it would hide that the database enforces the rule. The button stays, a note explains, and the database's refusal is shown verbatim. |

## Consequences

- One `apex serve` process serves both the API and the UI.
- Frontend changes need `npm run build` before `apex serve` picks them up; in development,
  `npm run dev` proxies `/api` to a running `apex serve`.
- No authentication yet (see ADR 0018).
