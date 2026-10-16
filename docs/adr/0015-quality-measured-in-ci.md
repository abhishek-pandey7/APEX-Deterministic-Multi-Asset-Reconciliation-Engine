# 0015. Matching quality measured against ground truth in CI

Status: Accepted

## Context

Unit tests show that a rule does what its author intended; they don't show that the rule is right
on realistic data. Nearly every matching rule in this project was changed because evaluation on
generated data showed wrong auto-matches the unit tests had not anticipated.

## Decision

- `apex eval` matches a generated dataset straight from its files and scores the result against the
  truth file: per-scenario recovery, auto-match precision, breaks correctly left open, and break
  category accuracy. Duplicate custodian lines are treated as interchangeable.
- Every evaluation also proves determinism by re-running on shuffled input with one worker.
- CI generates a dataset and runs a quality gate that fails the build if auto precision drops below
  0.9995, if any recovered scenario falls below its bar, or if category accuracy falls below 0.99.
- Two dataset sizes: small (44k rows) for CI speed, medium (770k rows, 4× denser) because density
  exposed problems the small set didn't.

## Alternatives considered

| Option | Why not |
|---|---|
| Unit tests only | Missed every density-dependent failure found by evaluation. |
| Manual spot checks | Unrepeatable and can't catch a 0.1% regression. |
| A single accuracy number | Hides where it fails: a 96% overall rate can be 100% exact matches and 45% batches. Per-scenario numbers point at the phase responsible. |

## Consequences

- Precision regressions block merges.
- Bars are set at current quality and raised as phases improve, never lowered quietly.
- The gate measures synthetic scenarios only (see ADR 0008).
