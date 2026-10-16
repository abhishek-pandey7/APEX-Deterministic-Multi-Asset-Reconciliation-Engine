# 0011. Matching in phases, most certain first

Status: Accepted

## Context

Most ledger entries have an obvious custodian counterpart; a minority need tolerance, grouping, or
judgement. Running the expensive, uncertain logic over everything is slow, and it lets weak
evidence compete with strong evidence on equal terms.

## Decision

Match in phases. Each phase only sees what earlier phases left:

1. **Exact**: same amount and value date, and the references agree (score ≥ 950).
2. **Tolerance**: amount within 0.99 and value date within 2 business days, ranked by reference
   evidence, then amount difference, then date gap.
3. **N-to-M**: splits, netting and fee batches (ADR 0014).
4. **Fuzzy**: mangled references outside the window, proposed only (planned).

Every match is either auto-confirmed or proposed for review. A wrong auto-match is treated as the
most expensive error, because nobody looks at it again; an unmatched entry becomes a visible break
that someone will.

## Alternatives considered

| Option | Why not |
|---|---|
| One scoring model over all candidate pairs | Lets a strong exact match and a weak fuzzy one compete on a single number, and makes every decision depend on weights nobody can explain to an auditor. |
| Exact phase on amount and date alone | The first version did this. Same-amount pairs with conflicting references (two different $25 fees on one day) claimed entries that a better candidate in the next phase needed. Requiring reference agreement fixed it. |
| Machine-learned matcher | Needs labelled production data, is hard to explain to auditors, and isn't deterministic across retrains. Kept as a possible scorer for the fuzzy phase, where it would only propose. |

## Consequences

- Throughput is dominated by the cheap exact phase: about 2M rows/s for phases 1–2.
- Each phase has its own rules and its own evaluation numbers, so a regression points at one phase.
- Thresholds (950 to auto-confirm, 600 to propose) are tuned on synthetic data; real data may need
  different values, which is why they are configuration rather than constants.
