package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/eval"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/ingest"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
)

// runEval matches a generated dataset straight from its files and scores the result against
// the truth file. It also reruns on shuffled input with a different worker count and checks
// that the result hash is unchanged.
func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	dir := fs.String("data", "data/small", "directory written by apexgen")
	fs.Parse(args) //nolint:errcheck // ExitOnError

	cfg, err := configFromManifest(filepath.Join(*dir, "manifest.json"))
	if err != nil {
		return err
	}
	ledger, err := readCash(filepath.Join(*dir, "ledger_cash.csv"), "apex-ledger-cash")
	if err != nil {
		return err
	}
	custodian, err := readCash(filepath.Join(*dir, "custodian_cash.csv"), "custodian-cash-v1")
	if err != nil {
		return err
	}
	truth, err := eval.LoadTruth(filepath.Join(*dir, "truth_cash.csv"))
	if err != nil {
		return err
	}

	start := time.Now()
	res := match.Run(ledger, custodian, cfg)
	elapsed := time.Since(start)
	hash := res.Hash(ledger, custodian)

	// Determinism check: shuffled input, one worker.
	ls, cs := slices.Clone(ledger), slices.Clone(custodian)
	rng := rand.New(rand.NewPCG(7, 7))
	rng.Shuffle(len(ls), func(i, j int) { ls[i], ls[j] = ls[j], ls[i] })
	rng.Shuffle(len(cs), func(i, j int) { cs[i], cs[j] = cs[j], cs[i] })
	single := cfg
	single.Workers = 1
	if rerun := match.Run(ls, cs, single).Hash(ls, cs); rerun != hash {
		return errors.New("determinism check failed: shuffled single-worker run produced a different result")
	}

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
	report := eval.Evaluate(truth, predicted)

	start = time.Now()
	breaks := match.Classify(ledger, custodian, res, cfg)
	classifyTime := time.Since(start)
	var predictedBreaks []eval.PredictedBreak
	for _, b := range breaks {
		pb := eval.PredictedBreak{Category: string(b.Category)}
		for _, i := range b.Ledger {
			pb.Ledger = append(pb.Ledger, ledger[i].ExternalID)
		}
		for _, i := range b.Custodian {
			pb.Custodian = append(pb.Custodian, custodian[i].ExternalID)
		}
		predictedBreaks = append(predictedBreaks, pb)
	}

	rows := len(ledger) + len(custodian)
	fmt.Printf("matched %d ledger + %d custodian entries in %s (%.0f rows/s, %d workers)\n",
		len(ledger), len(custodian), elapsed.Round(time.Millisecond), float64(rows)/elapsed.Seconds(), cfg.Workers)
	fmt.Printf("result hash %s, identical on shuffled single-worker rerun\n\n", hex.EncodeToString(hash[:8]))
	printReport(report)
	fmt.Println()
	fmt.Printf("%d breaks classified in %s\n", len(breaks), classifyTime.Round(time.Millisecond))
	printBreakReport(eval.EvaluateBreaks(truth, predictedBreaks))
	return nil
}

func printBreakReport(stats map[string]*eval.CategoryStats) {
	names := make([]string, 0, len(stats))
	for n := range stats {
		names = append(names, n)
	}
	slices.Sort(names)
	fmt.Printf("%-22s %8s %10s   %s\n", "expected category", "breaks", "correct", "assigned instead")
	total, correct := 0, 0
	for _, n := range names {
		s := stats[n]
		total += s.Expected
		correct += s.Correct
		var other []string
		for got, k := range s.Got {
			if got != n {
				other = append(other, fmt.Sprintf("%s %d", got, k))
			}
		}
		slices.Sort(other)
		fmt.Printf("%-22s %8d %10s   %s\n", n, s.Expected, pct(s.Correct, s.Expected), strings.Join(other, ", "))
	}
	fmt.Printf("category accuracy %.4f (%d of %d)\n", float64(correct)/float64(max(total, 1)), correct, total)
}

func printReport(r eval.Report) {
	names := make([]string, 0, len(r.Scenarios))
	for n := range r.Scenarios {
		names = append(names, n)
	}
	slices.Sort(names)

	fmt.Printf("%-22s %8s %10s %10s %10s\n", "scenario", "groups", "recovered", "auto", "left open")
	for _, n := range names {
		s := r.Scenarios[n]
		fmt.Printf("%-22s %8d %10s %10s %10s\n", n, s.Groups,
			pct(s.Recovered, s.Groups), pct(s.AutoRecovered, s.Groups), pct(s.LeftOpen, s.Groups))
	}
	fmt.Printf("\npairs: precision %.4f  recall %.4f  (%d predicted, %d true)\n",
		r.Precision(), r.Recall(), r.PredictedPairs, r.TruePairs)
	fmt.Printf("auto:  precision %.4f  (%d auto pairs, %d wrong)\n", r.AutoPrecision(), r.AutoPairs, r.FalseAutoPairs())
}

func pct(n, total int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(total))
}

func readCash(path, layout string) ([]domain.CashEntry, error) {
	l, _, err := ingest.Lookup(layout)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	f, err := ingest.ParseCash(fh, *l)
	if err != nil {
		return nil, err
	}
	if !f.OK() {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), rowErrors(f.ErrorCount, f.Errors))
	}
	return f.Records, nil
}

// configFromManifest uses the tolerances the dataset was generated for.
func configFromManifest(path string) (match.Config, error) {
	cfg := match.DefaultConfig()
	cfg.Workers = runtime.NumCPU()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	var m struct {
		Hints struct {
			DateWindow int32 `json:"date_window_business_days"`
			AmountTol  int64 `json:"amount_tolerance_minor"`
		} `json:"engine_hints"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	cfg.DateWindow, cfg.AmountTolerance = m.Hints.DateWindow, m.Hints.AmountTol
	return cfg, nil
}
