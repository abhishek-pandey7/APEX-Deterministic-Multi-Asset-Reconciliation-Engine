package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/eval"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/ingest"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
)

// TestQualityGate runs the engine on generated data (APEX_TEST_DATA) and fails if quality drops
// below what the implemented phases already achieve. Raise the bars as phases are added.
func TestQualityGate(t *testing.T) {
	dir := os.Getenv("APEX_TEST_DATA")
	if dir == "" {
		t.Skip("APEX_TEST_DATA not set")
	}
	ledger := load(t, filepath.Join(dir, "ledger_cash.csv"), "apex-ledger-cash")
	custodian := load(t, filepath.Join(dir, "custodian_cash.csv"), "custodian-cash-v1")
	truth, err := eval.LoadTruth(filepath.Join(dir, "truth_cash.csv"))
	if err != nil {
		t.Fatal(err)
	}

	res := match.Run(ledger, custodian, match.DefaultConfig())
	var predicted []eval.Predicted
	for _, m := range res.Matches {
		p := eval.Predicted{Auto: m.Disposition == match.Auto}
		for _, i := range m.Ledger {
			p.Ledger = append(p.Ledger, ledger[i].ExternalID)
		}
		for _, i := range m.Custodian {
			p.Custodian = append(p.Custodian, custodian[i].ExternalID)
		}
		predicted = append(predicted, p)
	}
	r := eval.Evaluate(truth, predicted)

	// A wrong auto match is the costliest error: nobody reviews it.
	if got := r.AutoPrecision(); got < 0.9995 {
		t.Errorf("auto precision %.5f, want >= 0.9995 (%d wrong)", got, r.FalseAutoPairs())
	}
	for s, bar := range map[string]float64{
		"EXACT": 0.999, "TIMING": 0.999, "ROUNDING": 0.999, "SPLIT": 0.99, "MANY_TO_MANY": 0.95,
		// Batches are matched on amounts alone, so ambiguous ones are rightly left open.
		"AGGREGATE": 0.70,
	} {
		st := r.Scenarios[s]
		if got := float64(st.AutoRecovered) / float64(st.Groups); got < bar {
			t.Errorf("%s auto-recovered %.4f, want >= %.2f", s, got, bar)
		}
	}
	for _, s := range []string{"MISSING_IN_CUSTODIAN", "MISSING_IN_LEDGER", "AMOUNT_MISMATCH"} {
		st := r.Scenarios[s]
		if got := float64(st.LeftOpen) / float64(st.Groups); got < 0.99 {
			t.Errorf("%s left open %.4f, want >= 0.99", s, got)
		}
	}
	var predictedBreaks []eval.PredictedBreak
	for _, b := range match.Classify(ledger, custodian, res, match.DefaultConfig()) {
		pb := eval.PredictedBreak{Category: string(b.Category)}
		for _, i := range b.Ledger {
			pb.Ledger = append(pb.Ledger, ledger[i].ExternalID)
		}
		for _, i := range b.Custodian {
			pb.Custodian = append(pb.Custodian, custodian[i].ExternalID)
		}
		predictedBreaks = append(predictedBreaks, pb)
	}
	total, correct := 0, 0
	for _, s := range eval.EvaluateBreaks(truth, predictedBreaks) {
		total += s.Expected
		correct += s.Correct
	}
	if acc := float64(correct) / float64(total); acc < 0.99 {
		t.Errorf("break category accuracy %.4f, want >= 0.99", acc)
	}

	t.Logf("auto precision %.5f, pair recall %.4f, break categories %d/%d", r.AutoPrecision(), r.Recall(), correct, total)
}

func load(t *testing.T, path, layout string) []domain.CashEntry {
	t.Helper()
	l, _, err := ingest.Lookup(layout)
	if err != nil {
		t.Fatal(err)
	}
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	f, err := ingest.ParseCash(fh, *l)
	if err != nil || !f.OK() {
		t.Fatalf("%s: %v %v", path, err, f.Errors)
	}
	return f.Records
}
