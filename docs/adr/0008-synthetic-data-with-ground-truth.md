# 0008. Synthetic data with ground truth

Status: Accepted

## Context

To claim the engine is accurate, there must be data where the right answer is known. Real
custodian files are confidential, and even when available, nobody has labelled which entries
should match. Without labels, "the engine matched 94%" says nothing about whether those matches
were right.

## Decision

`tools/apexgen` generates paired ledger and custodian files plus a truth file that records, for
every entry, the scenario it came from and the correct outcome (auto-match, propose, or break with
a category). It injects 11 cash and 8 position scenarios: timing, rounding, splits, netting, fee
batches, mangled references, duplicates, amount mismatches, missing entries, price breaks,
unbooked splits and DRIP fractions.

- The custodian file deliberately differs in format: absolute amounts with a D/C flag, `YYYYMMDD`
  dates, free-text narratives, shuffled order.
- Python standard library only, and integer arithmetic only, so a seed produces byte-identical
  files on every platform (`math.log` and `exp` can differ in the last bit between C libraries).
- Each component draws from its own seeded stream, so changing one scenario's settings doesn't
  reshuffle the rest of the data.
- A manifest records the seed, parameters, tolerances, scenario counts and file hashes.

## Alternatives considered

| Option | Why not |
|---|---|
| Real or anonymised production data | Not available, and unlabelled anyway. |
| Faker-style random rows | Realistic-looking, but random pairs have no known correct match, so precision can't be measured. |
| A generator in Go, next to the engine | Would share code with the engine, and shared code can share bugs: a generator that "agrees" with the engine proves nothing. A separate implementation in another language is an independent check, and a shared rounding test vector keeps the two honest. |
| Hand-written fixture files | Fine for unit tests (and used there), but too small to expose the density effects that broke the first N-to-M version. |

## Consequences

- Every quality claim in this repository is a measured number.
- The scenarios encode assumptions about how custodians behave; real data will contain cases the
  generator doesn't. The evaluation proves the engine handles these cases, not every case.
- Holidays aren't modelled (weekends only).
