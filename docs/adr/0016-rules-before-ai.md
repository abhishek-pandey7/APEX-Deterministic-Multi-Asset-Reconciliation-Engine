# 0016. Rule-based break classification; AI advisory only

Status: Accepted

## Context

Every unmatched entry becomes a break that an analyst must understand: settled late, wrong amount,
reported twice, income not booked, and so on. An LLM could classify and explain breaks, and that
is part of the plan. But matching and classification feed financial controls, which need
answers that are reproducible, explainable and cheap at 50,000 breaks per run.

## Decision

- Deterministic rules classify every break and write a one-line reason, applied in order:
  duplicate, settled late, amount differs, unbooked income, missing in ledger, missing at custodian.
- An `AMOUNT` pairing needs reference evidence of 950; a `TIMING` pairing may use 900, because the
  amounts agreeing is extra evidence.
- The planned AI worker reads breaks and writes *suggestions* (classification, explanation,
  similar past breaks) to its own table, tagged with model id and prompt hash. It never creates,
  removes or changes a match or a break's category.

## Alternatives considered

| Option | Why not |
|---|---|
| LLM classifies every break | Not reproducible run to run, costs money per break at 50k breaks, and its answers can't be re-derived for an auditor. Rules reach 99.8–100% on the categories they cover. |
| LLM matches entries | Non-deterministic matching breaks ADR 0005, and a hallucinated match is the most expensive kind of error. |
| AI that can update state directly | A model mistake would bypass the four-eyes workflow (ADR 0006). Suggestions go through the same human approval as everything else. |
| Pair breaks by keying-error evidence (900) for every category | Measured: on dense data it labelled 817 missing entries as amount mismatches, because a swapped digit often lands on an unrelated sequential fee. |

## Consequences

- Category accuracy is 100% on the small dataset and 99.8% on the medium one, at under a second
  for 51k breaks (after indexing custodian lines by digits, down from 66 s).
- Rules only know the patterns someone wrote down. Breaks outside them are where the AI layer
  earns its place, as a suggestion.
