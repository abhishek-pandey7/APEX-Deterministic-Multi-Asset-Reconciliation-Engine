package eval

import (
	"strings"
	"testing"
)

const truth = `group_id,scenario,expected,category,ledger_ids,custodian_ids
G1,EXACT,AUTO_MATCH,,L1,C1
G2,SPLIT,AUTO_MATCH,,L2,C2;C3
G3,FUZZY_REF,PROPOSE,,L3,C4
G4,DUPLICATE,AUTO_MATCH,,L5,C6
G5,DUPLICATE,BREAK,DUPLICATE,,C7
G6,MISSING_IN_LEDGER,BREAK,MISSING_IN_LEDGER,,C8
`

func TestEvaluate(t *testing.T) {
	groups, err := ReadTruth(strings.NewReader(truth))
	if err != nil {
		t.Fatal(err)
	}
	r := Evaluate(groups, []Predicted{
		{Ledger: []string{"L1"}, Custodian: []string{"C1"}, Auto: true},
		{Ledger: []string{"L2"}, Custodian: []string{"C2"}, Auto: true}, // half of a split
		{Ledger: []string{"L3"}, Custodian: []string{"C4"}},             // proposed, correctly
		{Ledger: []string{"L5"}, Custodian: []string{"C7"}, Auto: true}, // the other duplicate copy
		{Ledger: []string{"L9"}, Custodian: []string{"C8"}, Auto: true}, // wrong
	})

	if r.TruePairs != 5 || r.PredictedPairs != 5 || r.CorrectPairs != 4 {
		t.Errorf("pairs true=%d predicted=%d correct=%d", r.TruePairs, r.PredictedPairs, r.CorrectPairs)
	}
	if r.AutoPairs != 4 || r.FalseAutoPairs() != 1 {
		t.Errorf("auto=%d false auto=%d", r.AutoPairs, r.FalseAutoPairs())
	}
	check := func(name string, want ScenarioStats) {
		t.Helper()
		if got := *r.Scenarios[name]; got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
	check("EXACT", ScenarioStats{Groups: 1, Recovered: 1, AutoRecovered: 1})
	check("SPLIT", ScenarioStats{Groups: 1})
	check("FUZZY_REF", ScenarioStats{Groups: 1, Recovered: 1})
	check("DUPLICATE", ScenarioStats{Groups: 2, Recovered: 1, AutoRecovered: 1, LeftOpen: 1})
	check("MISSING_IN_LEDGER", ScenarioStats{Groups: 1})
}
