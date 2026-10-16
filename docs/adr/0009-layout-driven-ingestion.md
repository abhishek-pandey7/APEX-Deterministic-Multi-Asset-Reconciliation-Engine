# 0009. Layout-driven ingestion that rejects whole files

Status: Accepted

## Context

Every custodian sends a different format: column names, column order, date formats, and sign
conventions (signed amounts, or absolute amounts plus a debit/credit flag). Files can contain bad
rows. The engine must only ever see one normalised shape.

## Decision

- Each source is described by a `Layout`: which header holds which field, the date format, and the
  sign convention. Adding a custodian means adding a layout, not new parsing code.
- Columns are found by header name, never by position, and a byte order mark on the first header
  is ignored.
- Every row is validated (currency, precision, dates, direction flag, empty or duplicate ids), and
  every error is reported with its line number.
- A file with any invalid row is rejected as a whole.

## Alternatives considered

| Option | Why not |
|---|---|
| A parser per custodian in code | Each new custodian means new code and new bugs; most differences are just mapping. |
| Columns by position | Breaks silently when a custodian reorders or adds a column. |
| Load the good rows, skip the bad ones | A partially loaded file makes the reconciliation silently incomplete: missing rows show up as breaks against the other side, and nobody learns the file was bad. |
| Stop at the first bad row | Forces a fix-and-retry loop per error; reporting all of them at once is kinder to operations. |

## Consequences

- Bad files fail loudly with a complete error list.
- Formats that aren't CSV, or need logic beyond mapping (for example deriving a transaction type
  from narrative text), will need an extension point when they appear.
