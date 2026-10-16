# Matching Algorithms: Research and Choices

This document surveys the algorithms available for each stage of reconciliation, then records which one
APEX uses and why. References are listed at the end.

## 1. Problem framing

Reconciliation is a bipartite matching problem. One side has ledger entries `L`, the other has custodian
entries `C`. A *match* is a pair of non-empty subsets `(l ⊆ L, c ⊆ C)` that represent the same economic
event, and no entry may appear in more than one match. Entries left over are breaks.

J.P. Morgan AI Research formalise this as the **Subset Matching Problem (SMP)** and its numeric special case,
the **Subset Sum Matching Problem (SSMP)**. In SSMP, two subsets match when their sums differ by at most `ε`,
and the objective prefers solutions that cover more entries with more, smaller matches [1]. Even the decision
version (does *any* match exist?) is NP-complete, by reduction from classical subset sum [1].

The practical consequence is that a reconciliation engine can't search for the global optimum. Real engines
(and this one) reduce the problem with cheap certain steps first and keep the combinatorial step small:

```
normalise → block → exact → tolerance → N-to-M (bounded) → fuzzy (proposed) → classify breaks
```

## 2. Normalisation

Most false breaks come from formatting, not from economics. Normalisation runs once at ingestion so every
later phase compares like with like.

| Field | Technique |
|---|---|
| Amounts | Decimal string → `int64` minor units using the currency's ISO 4217 exponent; never via float |
| Sign | Unify conventions (signed amount vs. absolute amount + DR/CR flag) to *signed from the account's view* |
| Dates | Parse each custodian's format into a date; keep value date and trade date separately |
| References | Upper-case, strip punctuation, then extract structured tokens with regexes: trade ids, ISIN (with check-digit validation), CUSIP, SEDOL, invoice numbers |
| Counterparties | Upper-case, drop legal suffixes (`LLC`, `INC`, `LTD`, `PLC`, `SA`), expand known abbreviations (`MGT` → `MANAGEMENT`) |

Validating the ISIN check digit (a Luhn variant) means a token that merely *looks* like an ISIN can't
create false evidence.

## 3. Blocking (candidate generation)

Comparing every ledger entry with every custodian entry is O(|L|·|C|). Blocking restricts comparisons to
records that could plausibly match. In record linkage, the blocking rules are the main factor in runtime,
and a rule that is too narrow silently loses true matches [3].

| Technique | Idea | Used for |
|---|---|---|
| Standard blocking | Only compare records with equal key | `(account_id, currency)`; also the unit of parallelism |
| Sorted neighbourhood | Sort by a key, compare within a sliding window | Value date window |
| Range search on sorted array | Sort by amount, binary-search `[a − tol, a + tol]` | Tolerance phase, O(log n) per probe |
| Bucket hashing | Hash `⌊a / tol⌋` and probe buckets `b−1, b, b+1` | Same as above in O(1); the same trick the SSMP search solver uses with `round(d/ε)` [1] |
| Inverted token index | Map each reference token to the entries containing it | Fuzzy phase: only compare entries sharing a rare token |

**APEX:** standard blocking on `(account, currency)`, then a sorted amount index within each partition.
Fuzzy candidates come from the inverted token index plus an amount range.

## 4. Phase 1: exact matching

A hash multimap keyed on `(account, currency, amount_minor, value_date)`. Building it is O(n) and each probe
is O(1). When a key has several entries on both sides (e.g. three identical $25.00 fees on one day), the
pairing is chosen by reference similarity, then by source id, so the result doesn't depend on input order.

## 5. Phase 2: tolerance matching and assignment

Candidates satisfy `|Δamount| ≤ max(abs_tol, rel_tol·|amount|)` and a value-date gap of at most `k`
**business** days. Calendar days would treat a Friday→Monday settlement as a 3-day gap.

Each candidate pair `(l, c)` gets a cost from the amount difference, the date gap and reference agreement.
Then each entry needs to be assigned to at most one partner. The options are:

| Method | Complexity | Optimal? | Notes |
|---|---|---|---|
| Greedy by score | O(E log E) | No; ½-approximation for max-weight matching | Simple; depends on a deterministic tie-break |
| Hungarian (Kuhn–Munkres) | O(n³), or O(mn + n² log n) [5] | Yes, min-cost one-to-one | Handles unequal side sizes |
| Min-cost max-flow | Polynomial | Yes | Generalises to capacities; used for ledger reconciliation in patent literature [6] |
| Auction algorithm | Pseudo-polynomial | ε-optimal | Parallelises well; more than we need |

**APEX:** build the candidate graph and split it into connected components with union-find. Most components
are a single pair and are matched directly. Components up to a size limit go to the Hungarian algorithm,
and larger ones use greedy assignment with a deterministic tie-break and are flagged in run statistics. A
match is auto-confirmed only if it is the unique best option for *both* entries. A tie produces a proposal,
never a guess.

## 6. Phase 3: N-to-M (subset-sum) matching

These are the split cases (a capital call paid in three wires, a trade settled in partial fills) and the
netted cases (a day's fees posted as one debit).

### Options

| Method | Complexity | Exact? | Source |
|---|---|---|---|
| Brute-force enumeration up to size k | O(nᵏ) | Yes, within k | Common in invoice matchers [8] |
| Meet-in-the-middle | O(2^(n/2)) | Yes | Basis of the SSMP search solver [1] |
| Dynamic programming over integer sums | O(n·X), where X is the sum range | Exact on positive integers [1] | Sparse hash-set DP table in `dpss` [7] |
| MILP (CPLEX / OR-Tools) | Exponential worst case | Optimal if it converges | Times out on most instances with M+N ≥ 25 [1] |
| Greedy wrapper | Repeatedly find one match and remove it | Heuristic | Combined with any of the above [1] |

Benchmark results from [1] that matter here:

- DP solved 100×100 real-valued instances (ε = 1) in seconds; MILP and search timed out or degraded.
- MILP gives the best objective only on tiny instances.
- Integer amounts are the regime where DP is exact, which is why minor units matter.
- Discretisation can admit false candidates, so every recovered match must be **re-validated against the
  original amounts**.

### What APEX does

After phases 1–2, the leftovers in a partition are usually few, but the raw sum range `X` in minor units
can be large, so a dense DP table isn't always practical. APEX uses:

1. **Bounded enumeration with pruning** for 1:N and N:1. For each unmatched entry, consider entries on the
   other side within the date window, sorted by amount. Search subsets up to size `k` (default 4) with
   branch-and-bound: stop extending once the partial sum exceeds the target plus tolerance (only valid when
   all amounts have the same sign, which holds within a direction).
2. **Meet-in-the-middle** for N:M with a small cap (default 3×3): enumerate subset sums on each side, hash
   them into tolerance buckets, and probe the neighbouring buckets.
3. **Ambiguity check.** Count the solutions up to 2. If exactly one exists, auto-match it. If more than one
   exists, emit a proposal. When several solutions tie, the choice is canonical (lexicographically smallest
   sorted id tuple) so reruns agree.
4. **Re-validation** of every match on the original integer amounts.

The greedy wrapper is used: smallest groups first, which matches [1]'s observation that the objective
favours finer-grained matches.

## 7. Phase 4: fuzzy / probabilistic matching

### String similarity metrics

| Metric | Measures | Good for | Weakness |
|---|---|---|---|
| Levenshtein | Insertions, deletions, substitutions | Short ids with typos | O(mn); ignores token order |
| Damerau–Levenshtein | Above plus adjacent transpositions | Typed references (`1243` vs `1234`) | Same |
| Jaro–Winkler | Matching characters and transpositions, rewards shared prefix | Short codes, names | Poor on long narratives |
| Jaccard on character q-grams | Overlap of 3-gram sets | Order-insensitive, robust to truncation | Ignores frequency |
| Token-set ratio | Similarity of sorted token sets | Narratives with reordered words | Can over-reward subsets |
| TF-IDF cosine on char n-grams | Weighted overlap, rare grams count more | Long free-text narratives; vectorisable | Needs a fitted vocabulary |
| Phonetic (Soundex, Metaphone) | How it sounds | People's names | Little use for references |

Raw Levenshtein on full narratives (`"SETTLEMENT REF TRD2026000123 /XS..."` vs `"TRD-2026-000123 BUY"`)
performs badly. Extracting tokens first (section 2), then comparing the extracted ids exactly or with
Jaro–Winkler, separates matches from non-matches far better.

### Finding: compare identifiers by their digits, not by string similarity

The first evaluation run auto-confirmed 99 wrong pairs. Nearly all were fees with standard
amounts paired to a *neighbouring* fee: references are sequential, and Jaro–Winkler scores
`FEE0000123` against `FEE0000124` around 0.9, as similar as a genuine typo.

APEX therefore scores identifier-like tokens (four or more digits) on their digits:

| Evidence | Score |
|---|---|
| Token equals the reference | 1000 |
| One contains the other (truncated, prefix dropped, wrapped) | 950 |
| Same digits apart from one adjacent swap, possibly after truncation | 900 |
| Any other digit difference | at most 500 |

A single *changed* digit is deliberately not forgiven, because that is exactly how neighbouring
ids differ. Two thresholds then apply: a pair needs 950 to be auto-confirmed and 600 to be
proposed at all. The remaining error after the digit rule was one pair whose ids, `18769` and
`18679`, really are one swap apart; no similarity measure can separate that from a keying error,
so swap-level evidence (900) now always goes to a human. Wrong auto matches: 99, then 1, then 0
of 320,456 on the medium dataset.

### Combining evidence: Fellegi–Sunter

The classical record-linkage model [2, 3] scores each field comparison by how much more likely the outcome
is among true matches than among non-matches:

- `m = P(outcome | match)` and `u = P(outcome | non-match)`
- The field weight is `log₂(m / u)`. The total match weight is the prior log-odds plus the sum of field
  weights, which converts to a probability.
- Each field has ordered comparison levels (exact → Jaro–Winkler ≥ 0.92 → ≥ 0.80 → else), and the first
  level that applies is used [4].
- **Term-frequency adjustment:** agreement on a rare value (one trade id) is stronger evidence than
  agreement on a common one (`DIVIDEND`) [4].
- `m` and `u` can be estimated without labels by expectation maximisation [3], or with labels by counting.
  Splink notes that fields used in a blocking rule can't have their `m` estimated from pairs generated by
  that rule [3].

Comparisons in APEX: reference tokens, counterparty name, amount difference band, date gap band, and
transaction type.

**APEX:** a Fellegi–Sunter scorer with `m` and `u` estimated from the synthetic ground truth (labels are
available) and checked against EM. The alternative is logistic regression or gradient boosting on the same
features with calibrated probabilities. Fellegi–Sunter was chosen because each match explains itself: every
field contributes a readable weight that the UI can show.

### Choosing the threshold

0.85 isn't a principled number. The threshold is chosen on a held-out generated dataset as the lowest score
at which precision is at least 99.5%. Scores above it are proposed with high priority, and scores in a lower
band are proposed with low priority. Fuzzy matches are never auto-confirmed.

## 8. Position reconciliation

Positions are compared, not matched. A full outer join on `(account, security_id, as_of_date)` produces
a quantity difference, a price difference and a market-value difference, each with its own tolerance:

- **Quantity:** zero tolerance; any difference is a break [9].
- **Price:** relative tolerance in basis points (different pricing sources legitimately differ).
- **Market value:** absolute plus relative tolerance in the currency's minor units.

Deterministic signatures classify most position breaks before any AI is involved:

| Signature | Likely cause |
|---|---|
| `qty_custodian / qty_ledger` ∈ {2, 3, 1.5, ½, ⅓, …} | Unbooked stock split or reverse split |
| Difference equals the quantity of a trade not yet settled | Trade date vs. settlement date timing [9] |
| Small fractional difference | Dividend reinvestment (DRIP) fractional shares [9] |
| Quantity matches, price beyond tolerance | Price source difference |
| Present on one side only | Missing booking, or a position transferred in or out |

## 9. Break classification and the AI layer

Rules handle breaks with an unambiguous signature: only the date differs → `TIMING`; a small amount
difference on a non-base currency → `FX`; custodian-only with dividend keywords → `CORPORATE_ACTION`. The
AI worker handles the rest:

1. **Classification** into the break taxonomy, as structured output with a confidence value.
2. **Explanation** in plain language, plus a suggested action (a booking or a custodian query).
3. **Similar past breaks**: an embedding of the break's features and narrative, searched with pgvector
   (HNSW index, cosine distance), showing how analysts resolved the closest past cases.

The AI layer reads breaks and writes suggestions. It never changes match state (see ARCHITECTURE.md).

## 10. Evaluation

The generator writes the correct answer for every injected scenario, so the engine is measured, not
described.

| Metric | Definition |
|---|---|
| Pairwise precision / recall / F1 | Over `(ledger_id, custodian_id)` links produced vs. true links |
| Group exact-match rate | Share of true groups reproduced exactly (matters for N-to-M) |
| Auto-match rate | Share of entries auto-matched without human review |
| False auto-match rate | Auto-matches that are wrong; the number that matters most to operations |
| Break category accuracy | Predicted vs. injected category |
| Throughput | Rows per second per phase, at 10⁵ to 10⁷ rows |
| Determinism | Output hash identical across two runs and across `GOMAXPROCS` values |

## References

1. J.P. Morgan AI Research, *The Subset Sum Matching Problem*, arXiv:2508.19218 (2025).
   https://arxiv.org/abs/2508.19218
2. I. P. Fellegi and A. B. Sunter, "A Theory for Record Linkage," *JASA* 64(328), 1969.
3. Splink demos: estimating model parameters (Fellegi–Sunter, EM, blocking).
   https://nbviewer.org/github/moj-analytical-services/splink_demos/blob/max_pair/04_Estimating_model_parameters.ipynb
4. Splink comparison levels and term-frequency adjustment (DeepWiki).
   https://deepwiki.com/moj-analytical-services/splink/3.1-comparison-levels
5. L. Ramshaw and R. E. Tarjan, *On Minimum-Cost Assignments in Unbalanced Bipartite Graphs*.
   https://web.eecs.umich.edu/~pettie/matching/Ramshaw-Tarjan-imperfect-bipartite-matching.pdf
6. USPTO, *Ledger reconciliation* (min-cost max-flow formulation).
   https://image-ppubs.uspto.gov/dirsearch-public/print/downloadPdf/12332919
7. `dpss`: many-to-many subset-sum reconciliation with a sparse DP table.
   https://github.com/europeanplaice/subset_sum
8. `invoice-payment-matcher`: brute force for small sets, DP for larger ones.
   https://github.com/realtonkaa/invoice-payment-matcher
9. Practitioner notes on position and transaction reconciliation (zero-tolerance quantities, timing,
   DRIP fractions). https://skills.sh/joellewis/finance_skills/reconciliation
