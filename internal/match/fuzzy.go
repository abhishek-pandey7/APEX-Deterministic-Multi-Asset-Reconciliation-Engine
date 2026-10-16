package match

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// Phase 4 scores what is still unmatched with the Fellegi–Sunter model: each candidate pair is
// described by a few comparisons, each comparison outcome (a "level") carries a weight
// log2(m/u) — how much more often that outcome occurs among true matches than among false
// ones — and the weights are summed. Weights are integers in millibits, estimated from labelled
// data by EstimateWeights (see `apex weights`), so scores are exact and reproducible.
//
// Fuzzy matches are only ever proposed. The phase exists to put the likely counterpart of a
// late, badly referenced settlement in front of an analyst, not to decide for them.

// Comparison fields and their levels.
const (
	FieldRef = iota
	FieldAmount
	FieldGap
	FieldCounterparty
	numFields
)

var fieldNames = [numFields]string{"reference", "amount", "date gap", "counterparty"}

// levelNames documents what each level of each field means, in order.
var levelNames = [numFields][]string{
	{"same id", "id kept, letters dropped or wrapped", "keying error or truncation", "weak", "conflicting"},
	{"equal", "within tolerance"},
	{"0-2 business days", "3-4", "5-6", "7-10"},
	{"named in narrative", "not named", "no counterparty on the ledger"},
}

// Levels is one candidate pair's comparison vector.
type Levels [numFields]uint8

// Weights holds log2(m/u) × 1000 for every field and level.
type Weights [numFields][5]int32

// DefaultWeights were estimated with `apex weights -data data/small` (apexgen seed 42, 44k rows:
// 565 true and 208 false candidate pairs), capped at ±MaxWeight, and checked on a separate dataset
// (seed 7) and the denser medium one; see docs/adr/0021-fuzzy-matching.md.
var DefaultWeights = Weights{
	{5613, 4902, 5942, 1750, -5529}, // reference: same id, id kept, keying error, weak, conflicting
	{173, -6000},                    // amount: equal, within tolerance
	{-6000, 1344, 1352, -6000},      // date gap: 0-2, 3-4, 5-6, 7-10 business days
	{3909, -6000, -1429},            // counterparty: named, not named, none on the ledger
}

// DefaultFuzzyThreshold sits in the middle of the range (2000-6000) where held-out precision was
// 1.000 at 96.7% recall; at 0 the dense dataset let 3 wrong pairs through, above 8000 recall drops.
const DefaultFuzzyThreshold = 4000

func refLevel(sim int32) uint8 {
	switch {
	case sim >= 1000:
		return 0
	case sim >= 950:
		return 1
	case sim >= 900:
		return 2
	case sim >= 600:
		return 3
	default:
		return 4
	}
}

func gapLevel(gap int32) uint8 {
	switch {
	case gap <= 2:
		return 0
	case gap <= 4:
		return 1
	case gap <= 6:
		return 2
	default:
		return 3
	}
}

var legalSuffixes = map[string]bool{"LLC": true, "INC": true, "LTD": true, "LP": true, "PLC": true, "CORP": true, "CO": true, "SA": true}

// counterpartyLevel checks whether the ledger counterparty's first significant word appears in
// the custodian narrative ("Northbridge Securities LLC" against "... NORTHBRIDGE SEC").
func counterpartyLevel(counterparty string, narrative []string) uint8 {
	for _, w := range strings.Fields(strings.ToUpper(counterparty)) {
		w = compact(w)
		if len(w) < 3 || legalSuffixes[w] {
			continue
		}
		if slices.Contains(narrative, w) {
			return 0
		}
		return 1
	}
	return 2
}

// Score sums the weights of a comparison vector.
func (w *Weights) Score(lv Levels) int32 {
	var s int32
	for f, level := range lv {
		s += w[f][level]
	}
	return s
}

// FuzzyPair is one candidate the fuzzy phase considers, exposed so weights can be estimated.
type FuzzyPair struct {
	Ledger, Custodian int32 // indices into the slices passed to Run
	Levels            Levels
	RefSim            int32
}

// fuzzyCandidates lists every pair of unused entries that could plausibly be the same movement:
// same sign, amounts within tolerance, value dates within FuzzyWindow business days. No
// reference evidence is required here; that is what the weights are for.
func fuzzyCandidates(l, c *side, cfg Config) (pairs []FuzzyPair, li, ci []int) {
	wide := cfg
	wide.DateWindow = cfg.FuzzyWindow
	for i, e := range l.entries {
		if l.used[i] {
			continue
		}
		for _, j := range window(c, e.ValueDate, e.AmountMinor, -1, wide) {
			ce := c.entries[j]
			diff := ce.AmountMinor - e.AmountMinor
			if max(diff, -diff) > cfg.AmountTolerance {
				continue
			}
			sim := RefSim(l.ref[i], c.tokens[j])
			lv := Levels{refLevel(sim), 0, gapLevel(BizGap(e.ValueDate, ce.ValueDate)), counterpartyLevel(e.Counterparty, c.tokens[j])}
			if diff != 0 {
				lv[FieldAmount] = 1
			}
			pairs = append(pairs, FuzzyPair{Ledger: l.index[i], Custodian: c.index[j], Levels: lv, RefSim: sim})
			li, ci = append(li, i), append(ci, j)
		}
	}
	return pairs, li, ci
}

// fuzzyPhase proposes the best-scoring pairs above FuzzyThreshold, greedily and one-to-one.
func fuzzyPhase(l, c *side, cfg Config, r *Result) {
	pairs, li, ci := fuzzyCandidates(l, c, cfg)
	type scored struct {
		k     int
		score int32
	}
	var keep []scored
	for k, p := range pairs {
		if s := cfg.FuzzyWeights.Score(p.Levels); s >= cfg.FuzzyThreshold {
			keep = append(keep, scored{k, s})
		}
	}
	slices.SortFunc(keep, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score),
			strings.Compare(l.entries[li[a.k]].ExternalID, l.entries[li[b.k]].ExternalID),
			strings.Compare(c.entries[ci[a.k]].ExternalID, c.entries[ci[b.k]].ExternalID))
	})
	for _, s := range keep {
		i, j := li[s.k], ci[s.k]
		if l.used[i] || c.used[j] {
			continue
		}
		l.used[i], c.used[j] = true, true
		r.Matches = append(r.Matches, Match{
			Phase: PhaseFuzzy, Disposition: Proposed,
			Ledger: []int32{l.index[i]}, Custodian: []int32{c.index[j]},
			RefSim: pairs[s.k].RefSim, Score: s.score,
		})
	}
}

// FuzzyPairs runs phases 1–3 and returns every candidate phase 4 would score, for estimating
// weights against labelled data.
func FuzzyPairs(ledger, custodian []domain.CashEntry, cfg Config) []FuzzyPair {
	var out []FuzzyPair
	for _, p := range partition(ledger, custodian) {
		l := newSide(ledger, p.ledger, true)
		c := newSide(custodian, p.custodian, false)
		var r Result
		exactPhase(l, c, cfg, &r)
		tolerancePhase(l, c, cfg, &r)
		nToMPhase(l, c, cfg, &r)
		pairs, _, _ := fuzzyCandidates(l, c, cfg)
		out = append(out, pairs...)
	}
	return out
}

// LevelCounts tallies how often each level occurs among true and false candidate pairs.
type LevelCounts struct {
	True, False [numFields][5]int
	NTrue       int
	NFalse      int
}

// MaxWeight caps every weight at ±6 bits, so no single comparison can force or veto a proposal on
// its own: it always takes at least two pieces of evidence. Without the cap, levels the generator
// never produces for true pairs (gaps over 6 days, narratives without the counterparty) got
// weights near -9 bits, which would make the matcher brittle on real data.
const MaxWeight = 6000

// EstimateWeights computes log2(m/u) for every level from labelled pairs, with add-one smoothing
// so that a level never seen among true (or false) pairs gets a finite weight, then caps each
// weight at ±MaxWeight.
func EstimateWeights(pairs []FuzzyPair, isTrue func(FuzzyPair) bool) (Weights, LevelCounts) {
	var c LevelCounts
	for _, p := range pairs {
		t := isTrue(p)
		if t {
			c.NTrue++
		} else {
			c.NFalse++
		}
		for f, lv := range p.Levels {
			if t {
				c.True[f][lv]++
			} else {
				c.False[f][lv]++
			}
		}
	}
	var w Weights
	for f := range numFields {
		n := len(levelNames[f])
		for lv := range n {
			m := float64(c.True[f][lv]+1) / float64(c.NTrue+n)
			u := float64(c.False[f][lv]+1) / float64(c.NFalse+n)
			w[f][lv] = max(-MaxWeight, min(MaxWeight, int32(math.Round(1000*math.Log2(m/u)))))
		}
	}
	return w, c
}

// FieldName and LevelName describe the comparison vector for reports.
func FieldName(f int) string     { return fieldNames[f] }
func LevelName(f, lv int) string { return levelNames[f][lv] }
func NumLevels(f int) int        { return len(levelNames[f]) }
func NumFields() int             { return numFields }
