package match

import (
	"cmp"
	"slices"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// nToMPhase finds groups whose totals agree: one ledger entry settled as several custodian
// lines (a split), several ledger entries posted as one line (a fee batch), or several on both
// sides (netted trades). It runs on what the one-to-one phases left over, in two passes:
//
//  1. Reference-anchored. Custodian lines that carry a ledger entry's reference are gathered;
//     their total is matched by that entry alone (a split) or by it plus a few more ledger
//     entries from the same day (netting).
//  2. Batches. A custodian line with no specific reference is matched to several same-day
//     ledger entries by amount alone.
//
// Without a reference, an amount total is weak evidence: round amounts recur, and one fee can
// stand in for another. So batches demand more: an exact total, the same value date on every
// leg, no specific reference on the line, a unique combination found by an exhaustive search,
// and no competition (see batchPass). Splits without any reference are left open for review rather than guessed.
func nToMPhase(l, c *side, cfg Config, r *Result) {
	for li := range l.entries {
		if !l.used[li] {
			anchoredGroup(l, c, li, cfg, r)
		}
	}
	batchPass(l, c, cfg, r)
}

// window returns unused entries on s with the same sign as amount and a value date within
// DateWindow business days of d. Entries are sorted by value date, so the calendar range that
// can contain such dates is found by binary search.
func window(s *side, d domain.Date, amount int64, skip int, cfg Config) []int {
	// Every 5 business days span at most 7 calendar days, plus a weekend at either end.
	span := domain.Date(cfg.DateWindow + 2*(cfg.DateWindow/5+1))
	start, _ := slices.BinarySearchFunc(s.entries, d-span, func(e *domain.CashEntry, t domain.Date) int {
		return cmp.Compare(e.ValueDate, t)
	})
	var out []int
	for i := start; i < len(s.entries) && s.entries[i].ValueDate <= d+span; i++ {
		e := s.entries[i]
		if s.used[i] || i == skip || (e.AmountMinor > 0) != (amount > 0) {
			continue
		}
		if BizGap(e.ValueDate, d) <= cfg.DateWindow {
			out = append(out, i)
		}
	}
	return out
}

func anchoredGroup(l, c *side, li int, cfg Config, r *Result) {
	e := l.entries[li]
	var linked []int
	for _, ci := range window(c, e.ValueDate, e.AmountMinor, -1, cfg) {
		if RefSim(l.ref[li], c.tokens[ci]) >= cfg.MinAutoRefSim {
			linked = append(linked, ci)
		}
	}
	if len(linked) == 0 || len(linked) > cfg.MaxCandidates {
		return
	}

	// A split: some of the linked lines add up to this entry.
	if len(linked) >= 2 {
		sols := subsets(amounts(c, linked), e.AmountMinor, cfg.AmountTolerance, 2, cfg.MaxGroupSize, 2)
		if len(sols) > 0 {
			emit(l, c, []int{li}, pick(linked, sols[0]), len(sols) == 1, r)
			return
		}
	}

	// Many-to-many: all linked lines together are matched by this entry plus a few others. One
	// linked line with a different amount is an amount mismatch, not a netted group, so it needs
	// at least two; and the extra ledger entries are found by amount alone, so the total must be exact.
	total := sumOf(c, linked)
	rest := total - e.AmountMinor
	if len(linked) < 2 || len(linked) > cfg.MaxGroupSize || rest == 0 || (rest > 0) != (e.AmountMinor > 0) {
		return
	}
	// Netted trades settle together, so the extra legs share the anchor's value date.
	sameDay := cfg
	sameDay.DateWindow = 0
	others := window(l, e.ValueDate, e.AmountMinor, li, sameDay)
	if len(others) > cfg.MaxCandidates {
		return // too many to search exhaustively, so a unique answer couldn't be certified
	}
	sols := subsets(amounts(l, others), rest, 0, 1, cfg.MaxGroupSize-1, 2)
	if len(sols) > 0 {
		emit(l, c, append([]int{li}, pick(others, sols[0])...), linked, len(sols) == 1, r)
	}
}

// batchPass matches custodian batch lines to the same-day ledger entries they sum to. Every
// line's candidate group is found first, without claiming anything; a group is accepted only if
// none of its ledger entries is wanted by another line's group too. Resolving lines one at a
// time instead would let an early wrong pick consume a real member, after which a later line
// "uniquely" matches a stand-in with the same amount.
func batchPass(l, c *side, cfg Config, r *Result) {
	sameDay := cfg
	sameDay.DateWindow = 0
	type proposal struct {
		ci   int
		legs []int
	}
	var props []proposal
	wanted := map[int]int{}
	for ci, e := range c.entries {
		// A line that names a specific item ("CHARGES REF FEE0002254", "SETTLEMENT REF TRD...")
		// refers to that one item, so it is never a batch of several, even if one of them is the
		// item it names. A batch line names nothing more specific than a date.
		if c.used[ci] || hasSpecificID(c.tokens[ci]) {
			continue
		}
		cands := window(l, e.ValueDate, e.AmountMinor, -1, sameDay)
		// Uniqueness is the only evidence here, and only an exhaustive search can certify it.
		if len(cands) < 2 || len(cands) > cfg.MaxCandidates {
			continue
		}
		sols := subsets(amounts(l, cands), e.AmountMinor, 0, 2, cfg.MaxGroupSize, 2)
		if len(sols) != 1 {
			continue
		}
		legs := pick(cands, sols[0])
		props = append(props, proposal{ci, legs})
		for _, li := range legs {
			wanted[li]++
		}
	}
	for _, p := range props {
		contested := false
		for _, li := range p.legs {
			contested = contested || wanted[li] > 1
		}
		if !contested {
			emit(l, c, p.legs, []int{p.ci}, true, r)
		}
	}
}

// hasSpecificID reports whether any token looks like an identifier: at least four digits and
// not a YYYYMMDD date.
func hasSpecificID(tokens []string) bool {
	for _, t := range tokens {
		if len(digitsOf(t)) >= 4 && !isDate(t) {
			return true
		}
	}
	return false
}

func isDate(t string) bool {
	if len(t) != 8 || digitsOf(t) != t {
		return false
	}
	y, m, d := t[:4], t[4:6], t[6:]
	return (y[:2] == "19" || y[:2] == "20") && m >= "01" && m <= "12" && d >= "01" && d <= "31"
}

func emit(l, c *side, lis, cis []int, unique bool, r *Result) {
	m := Match{Phase: PhaseNToM, Disposition: Proposed}
	if unique {
		m.Disposition = Auto
	}
	// Legs are listed in each side's canonical order, never in the caller's input order.
	slices.Sort(lis)
	slices.Sort(cis)
	for _, i := range lis {
		l.used[i] = true
		m.Ledger = append(m.Ledger, l.index[i])
	}
	for _, i := range cis {
		c.used[i] = true
		m.Custodian = append(m.Custodian, c.index[i])
	}
	r.Matches = append(r.Matches, m)
}

func amounts(s *side, idx []int) []int64 {
	out := make([]int64, len(idx))
	for k, i := range idx {
		out[k] = s.entries[i].AmountMinor
	}
	return out
}

func sumOf(s *side, idx []int) int64 {
	var t int64
	for _, i := range idx {
		t += s.entries[i].AmountMinor
	}
	return t
}

func pick(idx []int, sel []int) []int {
	out := make([]int, len(sel))
	for k, j := range sel {
		out[k] = idx[j]
	}
	return out
}
