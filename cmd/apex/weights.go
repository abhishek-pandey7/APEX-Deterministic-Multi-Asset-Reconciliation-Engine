package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/eval"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
)

// runWeights estimates Fellegi–Sunter weights for the fuzzy phase from one labelled dataset and
// reports, on a second one, the precision and recall of fuzzy proposals at a range of
// thresholds, so the threshold is chosen from held-out evidence rather than guessed.
func runWeights(args []string) error {
	fs := flag.NewFlagSet("weights", flag.ExitOnError)
	trainDir := fs.String("data", "data/small", "labelled dataset to estimate weights from")
	holdoutDir := fs.String("holdout", "data/holdout", "separate labelled dataset to choose the threshold on")
	fs.Parse(args) //nolint:errcheck // ExitOnError

	ledger, custodian, truth, cfg, err := loadDataset(*trainDir)
	if err != nil {
		return err
	}
	links := truePairs(truth)
	pairs := match.FuzzyPairs(ledger, custodian, cfg)
	isTrue := func(p match.FuzzyPair) bool {
		return links[[2]string{ledger[p.Ledger].ExternalID, custodian[p.Custodian].ExternalID}]
	}
	w, counts := match.EstimateWeights(pairs, isTrue)

	fmt.Printf("%d candidate pairs left after phases 1-3 in %s: %d true, %d false\n\n",
		len(pairs), *trainDir, counts.NTrue, counts.NFalse)
	fmt.Printf("%-13s %-38s %7s %7s %9s\n", "field", "level", "true", "false", "weight")
	for f := range match.NumFields() {
		for lv := range match.NumLevels(f) {
			fmt.Printf("%-13s %-38s %7d %7d %9d\n", match.FieldName(f), match.LevelName(f, lv),
				counts.True[f][lv], counts.False[f][lv], w[f][lv])
		}
	}
	fmt.Printf("\nvar DefaultWeights = Weights{\n")
	for f := range match.NumFields() {
		var row []string
		for lv := range 5 {
			row = append(row, fmt.Sprint(w[f][lv]))
		}
		fmt.Printf("\t{%s}, // %s\n", strings.Join(row, ", "), match.FieldName(f))
	}
	fmt.Printf("}\n\n")

	hl, hc, htruth, hcfg, err := loadDataset(*holdoutDir)
	if err != nil {
		return err
	}
	hlinks := truePairs(htruth)
	fuzzyGroups := 0
	for _, g := range htruth {
		if g.Scenario == "FUZZY_REF" {
			fuzzyGroups++
		}
	}
	fmt.Printf("thresholds on %s (%d FUZZY_REF groups):\n", *holdoutDir, fuzzyGroups)
	fmt.Printf("%10s %10s %10s %10s\n", "threshold", "proposed", "precision", "recall")
	hcfg.FuzzyWeights = w
	for _, t := range []int32{0, 2000, 4000, 6000, 8000, 10000, 12000, 14000, 16000, 18000, 20000} {
		hcfg.FuzzyThreshold = t
		res := match.Run(hl, hc, hcfg)
		proposed, correct := 0, 0
		for _, m := range res.Matches {
			if m.Phase != match.PhaseFuzzy {
				continue
			}
			proposed++
			if hlinks[[2]string{hl[m.Ledger[0]].ExternalID, hc[m.Custodian[0]].ExternalID}] {
				correct++
			}
		}
		prec := 0.0
		if proposed > 0 {
			prec = float64(correct) / float64(proposed)
		}
		fmt.Printf("%10d %10d %10.4f %10.4f\n", t, proposed, prec, float64(correct)/float64(max(fuzzyGroups, 1)))
	}
	return nil
}

func loadDataset(dir string) ([]domain.CashEntry, []domain.CashEntry, []eval.Group, match.Config, error) {
	cfg, err := configFromManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	ledger, err := readCash(filepath.Join(dir, "ledger_cash.csv"), "apex-ledger-cash")
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	custodian, err := readCash(filepath.Join(dir, "custodian_cash.csv"), "custodian-cash-v1")
	if err != nil {
		return nil, nil, nil, cfg, err
	}
	truth, err := eval.LoadTruth(filepath.Join(dir, "truth_cash.csv"))
	return ledger, custodian, truth, cfg, err
}

// truePairs lists every (ledger id, custodian id) pair the truth file says belongs together.
func truePairs(groups []eval.Group) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, g := range groups {
		if g.Expected == "BREAK" {
			continue
		}
		for _, l := range g.Ledger {
			for _, c := range g.Custodian {
				out[[2]string{l, c}] = true
			}
		}
	}
	return out
}
