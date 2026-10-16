# 0021. Fuzzy matching with Fellegi–Sunter weights estimated from labelled data

Status: Accepted

## Context

After phases 1–3, the main scenario left unrecovered was a custodian line that settles 3–6 business
days late with a mangled reference (truncated, prefix dropped, digits swapped). It is outside the
tolerance window, and no single piece of evidence is conclusive. Several weak signals have to be
combined: reference similarity, amount, delay, and whether the narrative names the counterparty.

## Decision

- **Phase 4 uses the Fellegi–Sunter model.** Each candidate pair (same sign, amount within
  tolerance, within 10 business days) gets a comparison vector of four fields with discrete levels.
  Each level carries a weight `log2(m/u)`: how much more often that level occurs among true matches
  than among false candidates. The pair's score is the sum.
- **Weights are estimated, not hand-picked.** `apex weights` runs phases 1–3 on a labelled dataset,
  collects the candidates phase 4 would see, labels them from the truth file and counts each level
  (with add-one smoothing). Estimated on the small dataset: 565 true and 208 false pairs.
- **Weights are capped at ±6 bits**, so extreme ratios from sparse or artificial levels can't
  dominate.
- **The threshold is chosen on held-out data.** `apex weights` sweeps thresholds on a dataset with a
  different seed and on the denser medium dataset:

  | Threshold (millibits) | Held-out precision | Held-out recall | Medium precision |
  |---|---|---|---|
  | 0 | 1.000 | 0.967 | 0.9997 (3 wrong) |
  | 2,000–6,000 | 1.000 | 0.967 | 1.000 |
  | 8,000–10,000 | 1.000 | 0.944 | 1.000 |

  4,000 is the middle of the plateau.
- **Weights and threshold are integers** and part of the rule version (ADR 0017), so scores are
  exact and any change to them is visible on every stored run.
- **Fuzzy matches are always proposed**, never auto-confirmed.

## Alternatives considered

| Option | Why not |
|---|---|
| Widen the tolerance window to 10 days | Late settlements would then compete on reference evidence alone, and the delay itself is evidence that should be weighed, not ignored. |
| Hand-set weights | Arbitrary and unexplainable. Estimated weights come with the counts that justify them. |
| Logistic regression or gradient boosting | Calibrated probabilities and interactions between fields, but less explainable to an auditor than a per-field weight table, and more machinery for four discrete fields. A natural next step if fields multiply. |
| Expectation–maximisation without labels (Splink's approach) | Needed when there is no truth. Here labels exist, and counting is simpler and exact. EM is the route for real data before analysts have confirmed enough proposals. |
| No cap on weights | The generator never delays a true settlement by 7–10 days or omits the counterparty from a fuzzy narrative, so those levels drew weights near −9 bits: artefacts of the generator, not facts about custodians. |

## Consequences

- FUZZY_REF recovery went from 0% to 97.7% (small) and 96.7% (medium), all as proposals. Pair
  precision stayed at 1.000 and 0.9999, and breaks that should stay open still do.
- **These weights describe the generator, not real custodians.** Even capped, "counterparty not
  named" (−6,000) effectively vetoes a proposal, because the generator always names the counterparty
  in these narratives. On real data the weights must be re-estimated from labelled pairs, which is
  what `apex weights` is for. Analyst-confirmed proposals become those labels over time.
- Proposals now outnumber breaks in some categories, so the UI needs a proposal review queue (not
  built yet). Until then, the run summary shows how many proposals need review.
