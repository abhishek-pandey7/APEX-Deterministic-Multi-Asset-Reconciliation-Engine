// Package eval scores matching results against apexgen's ground truth, so the engine's quality
// is a measured number rather than a claim.
package eval

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// Group is one row of truth_cash.csv: entries that belong together and what a correct engine
// should do with them (AUTO_MATCH, PROPOSE or BREAK).
type Group struct {
	ID, Scenario, Expected, Category string
	Ledger, Custodian                []string
}

func LoadTruth(path string) ([]Group, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadTruth(f)
}

func ReadTruth(r io.Reader) ([]Group, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != "group_id,scenario,expected,category,ledger_ids,custodian_ids" {
		return nil, fmt.Errorf("eval: unexpected truth header")
	}
	groups := make([]Group, 0, len(rows)-1)
	for _, r := range rows[1:] {
		groups = append(groups, Group{r[0], r[1], r[2], r[3], split(r[4]), split(r[5])})
	}
	return groups, nil
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ";")
}

// Predicted is one engine match, by external id.
type Predicted struct {
	Ledger, Custodian []string
	Auto              bool
}

type ScenarioStats struct {
	Groups int
	// Recovered groups were reproduced exactly by one predicted match (any disposition).
	Recovered int
	// AutoRecovered groups were reproduced by an auto match.
	AutoRecovered int
	// LeftOpen groups (expected BREAK) had none of their entries matched.
	LeftOpen int
}

type Report struct {
	Scenarios map[string]*ScenarioStats

	TruePairs, PredictedPairs, CorrectPairs int
	AutoPairs, CorrectAutoPairs             int
}

func (r Report) Precision() float64     { return ratio(r.CorrectPairs, r.PredictedPairs) }
func (r Report) Recall() float64        { return ratio(r.CorrectPairs, r.TruePairs) }
func (r Report) AutoPrecision() float64 { return ratio(r.CorrectAutoPairs, r.AutoPairs) }
func (r Report) FalseAutoPairs() int    { return r.AutoPairs - r.CorrectAutoPairs }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

type pair struct{ l, c string }

// Evaluate compares predictions with the truth groups.
//
// A duplicated custodian line produces two identical rows; the generator records one as the
// match and the other as the DUPLICATE break, but either copy is equally correct. Both copies
// are therefore mapped to one canonical id before comparing.
func Evaluate(groups []Group, predicted []Predicted) Report {
	canon := duplicateTwins(groups)
	cid := func(id string) string {
		if c, ok := canon[id]; ok {
			return c
		}
		return id
	}

	truePairs := map[pair]bool{}
	for _, g := range groups {
		if g.Expected == "BREAK" {
			continue
		}
		for _, l := range g.Ledger {
			for _, c := range g.Custodian {
				truePairs[pair{l, cid(c)}] = true
			}
		}
	}

	r := Report{Scenarios: map[string]*ScenarioStats{}, TruePairs: len(truePairs)}
	matchedIDs := map[string]int{}
	exact := map[string]bool{} // signature of each predicted match -> auto?
	for _, p := range predicted {
		for _, l := range p.Ledger {
			matchedIDs[l]++
			for _, c := range p.Custodian {
				r.PredictedPairs++
				ok := truePairs[pair{l, cid(c)}]
				if ok {
					r.CorrectPairs++
				}
				if p.Auto {
					r.AutoPairs++
					if ok {
						r.CorrectAutoPairs++
					}
				}
			}
		}
		for _, c := range p.Custodian {
			matchedIDs[cid(c)]++
		}
		sig := signature(p.Ledger, p.Custodian, cid)
		exact[sig] = exact[sig] || p.Auto
	}

	for _, g := range groups {
		s := r.Scenarios[g.Scenario]
		if s == nil {
			s = &ScenarioStats{}
			r.Scenarios[g.Scenario] = s
		}
		s.Groups++
		if g.Expected == "BREAK" {
			open := true
			for _, id := range g.Ledger {
				open = open && matchedIDs[id] == 0
			}
			for _, id := range g.Custodian {
				// A DUPLICATE copy shares its id with the matched twin, which is matched once.
				limit := 0
				if g.Scenario == "DUPLICATE" {
					limit = 1
				}
				open = open && matchedIDs[cid(id)] <= limit
			}
			if open {
				s.LeftOpen++
			}
			continue
		}
		if auto, ok := exact[signature(g.Ledger, g.Custodian, cid)]; ok {
			s.Recovered++
			if auto {
				s.AutoRecovered++
			}
		}
	}
	return r
}

func signature(ledger, custodian []string, cid func(string) string) string {
	l := slices.Clone(ledger)
	c := make([]string, len(custodian))
	for i, id := range custodian {
		c[i] = cid(id)
	}
	slices.Sort(l)
	slices.Sort(c)
	return strings.Join(l, ",") + "|" + strings.Join(c, ",")
}

// duplicateTwins maps the custodian id of every DUPLICATE break to the id of its matched twin.
// The generator writes the match group immediately before its break group.
func duplicateTwins(groups []Group) map[string]string {
	twins := map[string]string{}
	for i := 1; i < len(groups); i++ {
		prev, g := groups[i-1], groups[i]
		if g.Scenario == "DUPLICATE" && g.Expected == "BREAK" && prev.Scenario == "DUPLICATE" &&
			len(g.Custodian) == 1 && len(prev.Custodian) == 1 {
			a, b := g.Custodian[0], prev.Custodian[0]
			c := min(a, b)
			twins[a], twins[b] = c, c
		}
	}
	return twins
}

// PredictedBreak is one engine break, by external id.
type PredictedBreak struct {
	Ledger, Custodian []string
	Category          string
}

// CategoryStats counts, for one expected category, how many truth breaks got it right.
type CategoryStats struct {
	Expected, Correct int
	// Got counts the categories actually assigned, for a confusion summary.
	Got map[string]int
}

// EvaluateBreaks compares break categories with the truth file's categories for every group
// that was expected to be a break. A truth break is looked up by any of its entries.
func EvaluateBreaks(groups []Group, breaks []PredictedBreak) map[string]*CategoryStats {
	canon := duplicateTwins(groups)
	cid := func(id string) string {
		if c, ok := canon[id]; ok {
			return c
		}
		return id
	}
	byEntry := map[string]string{}
	for _, b := range breaks {
		for _, id := range b.Ledger {
			byEntry[id] = b.Category
		}
		for _, id := range b.Custodian {
			byEntry[cid(id)] = b.Category
		}
	}

	out := map[string]*CategoryStats{}
	for _, g := range groups {
		if g.Expected != "BREAK" {
			continue
		}
		s := out[g.Category]
		if s == nil {
			s = &CategoryStats{Got: map[string]int{}}
			out[g.Category] = s
		}
		s.Expected++
		got := "(matched)"
		for _, id := range g.Ledger {
			if c, ok := byEntry[id]; ok {
				got = c
			}
		}
		for _, id := range g.Custodian {
			if c, ok := byEntry[cid(id)]; ok {
				got = c
			}
		}
		s.Got[got]++
		if got == g.Category {
			s.Correct++
		}
	}
	return out
}
