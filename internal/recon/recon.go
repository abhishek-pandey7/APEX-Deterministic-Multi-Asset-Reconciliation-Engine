// Package recon runs a full reconciliation against the database: load the stored entries,
// match, classify what is left, and save everything as one audited run.
package recon

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

// Event reports progress. Stages arrive in order: load, match, classify, save, done.
type Event struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	RunID   int64  `json:"run_id,omitempty"`
}

type Summary struct {
	RunID      int64          `json:"run_id"`
	Ledger     int            `json:"ledger_entries"`
	Custodian  int            `json:"custodian_entries"`
	Matches    map[string]int `json:"matches"` // keyed "PHASE/DISPOSITION"
	Breaks     map[string]int `json:"breaks"`  // keyed by category
	OutputHash string         `json:"output_sha256"`
	DurationMS int64          `json:"duration_ms"`
}

// Run reconciles the given cash files (all of them when fileIDs is empty) and stores the result.
// If anything fails after the run is recorded, the run is marked FAILED rather than left RUNNING.
func Run(ctx context.Context, st *store.Store, cfg match.Config, fileIDs []int64, progress func(Event)) (sum Summary, err error) {
	if progress == nil {
		progress = func(Event) {}
	}
	start := time.Now()

	progress(Event{Stage: "load", Message: "loading cash entries"})
	ledger, custodian, files, err := st.LoadCashEntries(ctx, fileIDs)
	if err != nil {
		return sum, err
	}
	if len(ledger) == 0 && len(custodian) == 0 {
		return sum, fmt.Errorf("recon: no cash entries loaded")
	}

	runID, err := st.StartRun(ctx, store.RunMeta{
		EngineVersion: match.Version,
		RuleVersion:   cfg.RuleVersion(),
		Config:        cfg,
		InputFileIDs:  files,
	})
	if err != nil {
		return sum, err
	}
	defer func() {
		if err != nil {
			_ = st.FailRun(context.WithoutCancel(ctx), runID, err)
		}
	}()
	sum = Summary{RunID: runID, Ledger: len(ledger), Custodian: len(custodian),
		Matches: map[string]int{}, Breaks: map[string]int{}}

	progress(Event{Stage: "match", RunID: runID,
		Message: fmt.Sprintf("matching %d ledger and %d custodian entries", len(ledger), len(custodian))})
	res := match.Run(ledger, custodian, cfg)
	hash := res.Hash(ledger, custodian)

	progress(Event{Stage: "classify", RunID: runID,
		Message: fmt.Sprintf("%d matches; classifying what is left", len(res.Matches))})
	breaks := match.Classify(ledger, custodian, res, cfg)

	matchRows := make([]store.MatchRow, len(res.Matches))
	for i, m := range res.Matches {
		matchRows[i] = store.MatchRow{
			Phase: m.Phase.String(), Disposition: m.Disposition.String(), RefSim: m.RefSim,
			Rule: rule(m.Phase), Ledger: ids(ledger, m.Ledger), Custodian: ids(custodian, m.Custodian),
		}
		sum.Matches[m.Phase.String()+"/"+m.Disposition.String()]++
	}
	breakRows := make([]store.BreakRow, len(breaks))
	for i, b := range breaks {
		breakRows[i] = store.BreakRow{
			Category: string(b.Category), AccountID: b.AccountID, Currency: b.Currency, Reason: b.Reason,
			AmountMinor: b.AmountMinor, OpenedOn: b.OpenedOn,
			Legs: append(ids(ledger, b.Ledger), ids(custodian, b.Custodian)...),
		}
		sum.Breaks[string(b.Category)]++
	}

	progress(Event{Stage: "save", RunID: runID,
		Message: fmt.Sprintf("saving %d matches and %d breaks", len(matchRows), len(breakRows))})
	if err = st.SaveResults(ctx, runID, matchRows, breakRows); err != nil {
		return sum, err
	}
	sum.OutputHash = hex.EncodeToString(hash[:])
	sum.DurationMS = time.Since(start).Milliseconds()
	if err = st.FinishRun(ctx, runID, sum, hash); err != nil {
		return sum, err
	}
	// Similarity vectors are prepared in the background by the AI worker. A failure here doesn't
	// fail the run: the worker also vectorizes a break on demand when a suggestion is requested.
	done := fmt.Sprintf("run %d finished in %d ms", runID, sum.DurationMS)
	if _, qerr := st.EnqueueJob(ctx, "vectorize_run", map[string]int64{"run_id": runID}); qerr != nil {
		done += "; similarity vectors not queued: " + qerr.Error()
	}
	progress(Event{Stage: "done", RunID: runID, Message: done})
	return sum, nil
}

func ids(all []domain.CashEntry, idx []int32) []int64 {
	out := make([]int64, len(idx))
	for k, i := range idx {
		out[k] = all[i].ID
	}
	return out
}

func rule(p match.Phase) string {
	switch p {
	case match.PhaseExact:
		return "same amount and value date, references agree"
	case match.PhaseTolerance:
		return "amount and date within tolerance, best reference evidence"
	default:
		return "group totals agree: reference-anchored split or netting, or a unique same-day batch"
	}
}
