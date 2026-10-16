# 0012. Identifier-aware reference similarity

Status: Accepted

## Context

The ledger has a clean reference (`TRD-2026-0001234`). The custodian has free text
(`SETTLEMENT REF TRD20260001234 NORTHBRIDGE SEC`), sometimes mangled: truncated, prefix dropped,
wrapped as `/RFB/...`, or with two digits swapped. Matching needs a score for "does this narrative
refer to that reference".

The first implementation used Jaro–Winkler on tokens and produced 99 wrong auto-matches on 44k rows.
References are sequential, and Jaro–Winkler scores `FEE0000123` against `FEE0000124` at about 0.9:
as similar as a genuine typo.

## Decision

Compare identifier-like tokens (four or more digits) on their digits:

| Evidence | Score |
|---|---|
| A token equals the reference | 1000 |
| One contains the other with every digit kept (prefix dropped, wrapped) | 950 |
| Digits lost to truncation, or one adjacent swap | 900 |
| Any other digit difference | at most 500 |

Tokens without digits fall back to Jaro–Winkler. Jaro–Winkler itself is computed with exact integer
fractions, so scores are identical on every CPU. Auto-confirming needs 950; proposing needs 600.

## Alternatives considered

| Option | Why not |
|---|---|
| Jaro–Winkler or Levenshtein on the whole token | Can't tell a typo from a neighbouring sequential id; measured 99 wrong auto-matches. |
| Forgive one substituted digit | That is exactly how neighbouring ids differ. Only adjacent swaps are forgiven. |
| Treat truncation as strong evidence (950) | `TRD202600079` is a prefix of ten different trades. When it was 950, the N-to-M phase used it as an anchor and merged two unrelated groups. Moved to 900. |
| TF-IDF or embeddings over narratives | Good at "these texts are about the same topic", not at "these two ids are the same id". |
| Fellegi–Sunter weights learned from data | Planned for the fuzzy phase, where several weak signals have to be combined. For one identifier comparison, explicit rules are clearer. |

## Consequences

- Wrong auto-matches went 99, then 1, then 0 on the small dataset.
- The remaining hard case is real ambiguity: ids `18769` and `18679` are one swap apart, so swap-level
  evidence (900) always goes to a person rather than being auto-confirmed.
- Exact or contained references need no tuning; the thresholds are the only knobs.
