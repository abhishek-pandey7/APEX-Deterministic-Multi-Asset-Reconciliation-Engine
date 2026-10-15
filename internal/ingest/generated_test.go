package ingest

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// Scenarios whose ledger and custodian sides sum to exactly the same amount.
var balancedScenarios = map[string]bool{
	"EXACT": true, "TIMING": true, "SPLIT": true, "AGGREGATE": true,
	"MANY_TO_MANY": true, "FUZZY_REF": true, "DUPLICATE": true,
}

// TestGeneratedDataset parses apexgen output end to end. Set APEX_TEST_DATA to a directory
// produced by `python -m apexgen`. It checks that every file parses cleanly and, using the
// truth file, that sign conventions were normalised: balanced groups must net to zero.
func TestGeneratedDataset(t *testing.T) {
	dir := os.Getenv("APEX_TEST_DATA")
	if dir == "" {
		t.Skip("APEX_TEST_DATA not set")
	}

	byID := map[string]domain.CashEntry{}
	for file, layout := range map[string]string{
		"ledger_cash.csv":    "apex-ledger-cash",
		"custodian_cash.csv": "custodian-cash-v1",
	} {
		l, _ := mustLayout(t, layout)
		f := parseFile(t, filepath.Join(dir, file), func(fh *os.File) (int, []RowError, error) {
			pf, err := ParseCash(fh, *l)
			if err != nil {
				return 0, nil, err
			}
			for _, e := range pf.Records {
				byID[e.ExternalID] = e
			}
			return len(pf.Records), pf.Errors, nil
		})
		t.Logf("%s: %d rows", file, f)
	}
	for file, layout := range map[string]string{
		"ledger_positions.csv":    "apex-ledger-positions",
		"custodian_positions.csv": "custodian-positions-v1",
	} {
		_, l := mustLayout(t, layout)
		n := parseFile(t, filepath.Join(dir, file), func(fh *os.File) (int, []RowError, error) {
			pf, err := ParsePositions(fh, *l)
			if err != nil {
				return 0, nil, err
			}
			return len(pf.Records), pf.Errors, nil
		})
		t.Logf("%s: %d rows", file, n)
	}

	truth, err := os.Open(filepath.Join(dir, "truth_cash.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer truth.Close()
	rows, err := csv.NewReader(truth).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, r := range rows[1:] { // group_id, scenario, expected, category, ledger_ids, custodian_ids
		if r[2] == "BREAK" || !balancedScenarios[r[1]] {
			continue
		}
		var net int64
		for _, id := range split(r[4]) {
			net += byID[id].AmountMinor
		}
		for _, id := range split(r[5]) {
			net -= byID[id].AmountMinor
		}
		if net != 0 {
			t.Errorf("group %s (%s) nets to %d, want 0", r[0], r[1], net)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no balanced groups found in truth file")
	}
	t.Logf("%d balanced groups net to zero", checked)
}

func parseFile(t *testing.T, path string, parse func(*os.File) (int, []RowError, error)) int {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	n, rowErrs, err := parse(fh)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for _, e := range rowErrs {
		t.Errorf("%s: %v", filepath.Base(path), e)
	}
	return n
}

func split(ids string) []string {
	if ids == "" {
		return nil
	}
	return strings.Split(ids, ";")
}
