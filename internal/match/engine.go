// Package match is the deterministic matching engine. It pairs ledger and custodian cash
// entries in phases, cheapest and most certain first, and reports everything it could not
// pair. Given the same entries and Config it always returns the same Result, regardless of
// input order or how many workers run.
package match

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

type Phase uint8

const (
	PhaseExact Phase = iota + 1
	PhaseTolerance
	PhaseNToM
)

func (p Phase) String() string {
	switch p {
	case PhaseExact:
		return "EXACT"
	case PhaseTolerance:
		return "TOLERANCE"
	case PhaseNToM:
		return "N_TO_M"
	}
	return fmt.Sprintf("Phase(%d)", p)
}

type Disposition uint8

const (
	// Auto matches need no human review.
	Auto Disposition = iota + 1
	// Proposed matches are plausible but need an analyst to confirm them.
	Proposed
)

func (d Disposition) String() string {
	if d == Auto {
		return "AUTO"
	}
	return "PROPOSED"
}

// Match links ledger and custodian entries, identified by their index in the slices passed to Run.
type Match struct {
	Phase       Phase
	Disposition Disposition
	Ledger      []int32
	Custodian   []int32
	// RefSim is the reference similarity (0..1000) that supported the match.
	RefSim int32
}

type Config struct {
	// DateWindow is the largest value-date gap, in business days, the tolerance phase accepts.
	DateWindow int32
	// AmountTolerance is the largest amount difference, in minor units, the tolerance phase accepts.
	AmountTolerance int64
	// MinAutoRefSim is the reference similarity a match needs to be confirmed without review.
	// The default (950) admits exact and truncated references but not ones that only match by
	// assuming a keying error: two different ids can be one digit swap apart, so those go to a human.
	MinAutoRefSim int32
	// MinProposeRefSim is the reference similarity a tolerance pair needs to be proposed at all.
	// Below it the references conflict (different ids), and a proposal would only be noise.
	MinProposeRefSim int32
	// MaxGroupSize caps how many entries one side of an N-to-M group may have.
	MaxGroupSize int
	// MaxCandidates caps how many entries an N-to-M search considers, keeping it bounded.
	MaxCandidates int
	// Workers is how many partitions are matched in parallel. It never changes the result.
	Workers int
}

func DefaultConfig() Config {
	return Config{DateWindow: 2, AmountTolerance: 99, MinAutoRefSim: 950, MinProposeRefSim: 600,
		MaxGroupSize: 4, MaxCandidates: 24, Workers: runtime.NumCPU()}
}

type Result struct {
	Matches            []Match
	UnmatchedLedger    []int32
	UnmatchedCustodian []int32
}

// Run matches ledger against custodian entries. Work is split by (account, currency): an
// entry can only ever match within its own partition, and each partition is processed by
// exactly one goroutine with no shared mutable state.
func Run(ledger, custodian []domain.CashEntry, cfg Config) Result {
	parts := partition(ledger, custodian)
	results := make([]Result, len(parts))

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < max(1, cfg.Workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = matchPartition(ledger, custodian, parts[i], cfg)
			}
		}()
	}
	for i := range parts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	// Partitions are merged in key order, so scheduling never affects the output order.
	var out Result
	for _, r := range results {
		out.Matches = append(out.Matches, r.Matches...)
		out.UnmatchedLedger = append(out.UnmatchedLedger, r.UnmatchedLedger...)
		out.UnmatchedCustodian = append(out.UnmatchedCustodian, r.UnmatchedCustodian...)
	}
	return out
}

type partKey struct{ account, currency string }

type part struct {
	key               partKey
	ledger, custodian []int32
}

func partition(ledger, custodian []domain.CashEntry) []part {
	byKey := map[partKey]*part{}
	get := func(e *domain.CashEntry) *part {
		k := partKey{e.AccountID, e.Currency}
		p := byKey[k]
		if p == nil {
			p = &part{key: k}
			byKey[k] = p
		}
		return p
	}
	for i := range ledger {
		p := get(&ledger[i])
		p.ledger = append(p.ledger, int32(i))
	}
	for i := range custodian {
		p := get(&custodian[i])
		p.custodian = append(p.custodian, int32(i))
	}

	// Map iteration order is random in Go; sort before anything depends on order.
	parts := make([]part, 0, len(byKey))
	for _, p := range byKey {
		parts = append(parts, *p)
	}
	slices.SortFunc(parts, func(a, b part) int {
		return cmp.Or(strings.Compare(a.key.account, b.key.account), strings.Compare(a.key.currency, b.key.currency))
	})
	return parts
}

// side holds one partition's entries from one source, in canonical order, plus what
// matching needs to know about each.
type side struct {
	entries []*domain.CashEntry
	index   []int32 // position in the caller's slice
	used    []bool
	ref     []string   // ledger: compacted reference
	tokens  [][]string // custodian: compacted narrative tokens
}

func newSide(all []domain.CashEntry, idx []int32, isLedger bool) *side {
	s := &side{index: slices.Clone(idx)}
	// Canonical order makes every later loop independent of the caller's input order.
	slices.SortFunc(s.index, func(a, b int32) int {
		x, y := &all[a], &all[b]
		return cmp.Or(cmp.Compare(x.ValueDate, y.ValueDate), cmp.Compare(x.AmountMinor, y.AmountMinor),
			strings.Compare(x.ExternalID, y.ExternalID))
	})
	s.entries = make([]*domain.CashEntry, len(s.index))
	s.used = make([]bool, len(s.index))
	for i, j := range s.index {
		s.entries[i] = &all[j]
	}
	if isLedger {
		s.ref = make([]string, len(s.entries))
		for i, e := range s.entries {
			s.ref[i] = compact(e.Reference)
		}
	} else {
		s.tokens = make([][]string, len(s.entries))
		for i, e := range s.entries {
			s.tokens[i] = tokens(e.Reference)
		}
	}
	return s
}

func matchPartition(ledger, custodian []domain.CashEntry, p part, cfg Config) Result {
	l := newSide(ledger, p.ledger, true)
	c := newSide(custodian, p.custodian, false)
	var r Result
	exactPhase(l, c, cfg, &r)
	tolerancePhase(l, c, cfg, &r)
	nToMPhase(l, c, cfg, &r)
	for i, u := range l.used {
		if !u {
			r.UnmatchedLedger = append(r.UnmatchedLedger, l.index[i])
		}
	}
	for i, u := range c.used {
		if !u {
			r.UnmatchedCustodian = append(r.UnmatchedCustodian, c.index[i])
		}
	}
	return r
}

// edge is a candidate pairing of ledger position li with custodian position ci.
type edge struct {
	li, ci int
	sim    int32
	diff   int64 // absolute amount difference
	gap    int32 // business days between value dates
}

// better orders candidate edges: stronger reference evidence first, then closer amounts,
// then closer dates, then by external id so that ties are always broken the same way.
func better(l, c *side) func(a, b edge) int {
	return func(a, b edge) int {
		return cmp.Or(
			cmp.Compare(b.sim, a.sim),
			cmp.Compare(a.diff, b.diff),
			cmp.Compare(a.gap, b.gap),
			strings.Compare(l.entries[a.li].ExternalID, l.entries[b.li].ExternalID),
			strings.Compare(c.entries[a.ci].ExternalID, c.entries[b.ci].ExternalID),
		)
	}
}

// exactPhase pairs entries with the same amount and value date. When several entries share a
// key (three $25 fees on one day), the reference decides who pairs with whom; identical
// candidates (a duplicated custodian line) are split by external id.
func exactPhase(l, c *side, cfg Config, r *Result) {
	type key struct {
		amount int64
		date   domain.Date
	}
	custodianByKey := map[key][]int{}
	for i, e := range c.entries {
		k := key{e.AmountMinor, e.ValueDate}
		custodianByKey[k] = append(custodianByKey[k], i)
	}

	// Only pairs whose references agree are taken here. A same-amount pair with conflicting
	// references falls through to the tolerance phase, where it competes with every other
	// candidate instead of claiming an entry a better match may need.
	var edges []edge
	for li, e := range l.entries {
		for _, ci := range custodianByKey[key{e.AmountMinor, e.ValueDate}] {
			if sim := RefSim(l.ref[li], c.tokens[ci]); sim >= cfg.MinAutoRefSim {
				edges = append(edges, edge{li: li, ci: ci, sim: sim})
			}
		}
	}
	assign(l, c, edges, PhaseExact, cfg, r)
}

// tolerancePhase pairs what's left when amounts differ by at most AmountTolerance and value
// dates by at most DateWindow business days.
func tolerancePhase(l, c *side, cfg Config, r *Result) {
	var open []int
	for ci, used := range c.used {
		if !used {
			open = append(open, ci)
		}
	}
	slices.SortFunc(open, func(a, b int) int {
		return cmp.Or(cmp.Compare(c.entries[a].AmountMinor, c.entries[b].AmountMinor), cmp.Compare(a, b))
	})

	var edges []edge
	for li, e := range l.entries {
		if l.used[li] {
			continue
		}
		lo := e.AmountMinor - cfg.AmountTolerance
		start, _ := slices.BinarySearchFunc(open, lo, func(ci int, v int64) int {
			return cmp.Compare(c.entries[ci].AmountMinor, v)
		})
		for _, ci := range open[start:] {
			ce := c.entries[ci]
			if ce.AmountMinor > e.AmountMinor+cfg.AmountTolerance {
				break
			}
			if (ce.AmountMinor > 0) != (e.AmountMinor > 0) {
				continue
			}
			gap := BizGap(e.ValueDate, ce.ValueDate)
			if gap > cfg.DateWindow {
				continue
			}
			sim := RefSim(l.ref[li], c.tokens[ci])
			if sim < cfg.MinProposeRefSim {
				continue
			}
			diff := ce.AmountMinor - e.AmountMinor
			edges = append(edges, edge{li, ci, sim, max(diff, -diff), gap})
		}
	}
	assign(l, c, edges, PhaseTolerance, cfg, r)
}

// assign picks one-to-one pairs greedily, best edge first. A pair is auto-confirmed only when
// its reference evidence is strong and it was the unambiguous best option for both entries;
// otherwise it is proposed for review.
func assign(l, c *side, edges []edge, phase Phase, cfg Config, r *Result) {
	cmpEdge := better(l, c)
	slices.SortFunc(edges, cmpEdge)

	// How many edges tie for the best similarity at each entry.
	bestL := map[int]int32{}
	bestC := map[int]int32{}
	tiesL := map[int]int{}
	tiesC := map[int]int{}
	for _, e := range edges {
		if s, ok := bestL[e.li]; !ok || e.sim > s {
			bestL[e.li], tiesL[e.li] = e.sim, 1
		} else if e.sim == s {
			tiesL[e.li]++
		}
		if s, ok := bestC[e.ci]; !ok || e.sim > s {
			bestC[e.ci], tiesC[e.ci] = e.sim, 1
		} else if e.sim == s {
			tiesC[e.ci]++
		}
	}

	for _, e := range edges {
		if l.used[e.li] || c.used[e.ci] {
			continue
		}
		l.used[e.li], c.used[e.ci] = true, true
		disp := Proposed
		unambiguous := e.sim == bestL[e.li] && e.sim == bestC[e.ci] && tiesL[e.li] == 1 && tiesC[e.ci] == 1
		if e.sim >= cfg.MinAutoRefSim && (unambiguous || phase == PhaseExact) {
			disp = Auto
		}
		r.Matches = append(r.Matches, Match{
			Phase:       phase,
			Disposition: disp,
			Ledger:      []int32{l.index[e.li]},
			Custodian:   []int32{c.index[e.ci]},
			RefSim:      e.sim,
		})
	}
}

// Hash fingerprints a result by external ids rather than slice positions, so two runs over
// the same data in a different input order hash the same.
func (r Result) Hash(ledger, custodian []domain.CashEntry) [32]byte {
	h := sha256.New()
	ids := func(prefix string, all []domain.CashEntry, idx []int32) {
		names := make([]string, len(idx))
		for i, j := range idx {
			names[i] = all[j].ExternalID
		}
		slices.Sort(names)
		fmt.Fprintf(h, "%s%s\n", prefix, strings.Join(names, ","))
	}
	lines := make([]string, len(r.Matches))
	for i, m := range r.Matches {
		names := func(all []domain.CashEntry, idx []int32) string {
			out := make([]string, len(idx))
			for k, j := range idx {
				out[k] = all[j].ExternalID
			}
			slices.Sort(out)
			return strings.Join(out, " ")
		}
		lines[i] = fmt.Sprintf("%s|%s|%d|%s|%s", m.Phase, m.Disposition, m.RefSim,
			names(ledger, m.Ledger), names(custodian, m.Custodian))
	}
	slices.Sort(lines)
	for _, line := range lines {
		fmt.Fprintln(h, line)
	}
	ids("unmatched-ledger:", ledger, r.UnmatchedLedger)
	ids("unmatched-custodian:", custodian, r.UnmatchedCustodian)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// Version identifies the engine build recorded on every stored run.
const Version = "0.4.0"

// RuleVersion fingerprints everything that can change a result: the engine version and every
// setting except Workers, which never does. Two runs with the same RuleVersion over the same
// input files must produce the same output hash.
func (c Config) RuleVersion() string {
	c.Workers = 0
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%+v", Version, c)))
	return fmt.Sprintf("%x", sum[:6])
}
