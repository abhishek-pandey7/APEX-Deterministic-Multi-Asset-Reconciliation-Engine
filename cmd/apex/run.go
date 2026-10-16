package main

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/recon"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

// runRecon reconciles stored cash entries and saves the result as an audited run.
func runRecon(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dbURL := fs.String("db", envOr("APEX_DB_URL", defaultDB), "Postgres URL")
	filesFlag := fs.String("files", "", "comma-separated source file ids (default: every cash file)")
	fs.Parse(args) //nolint:errcheck // ExitOnError

	var files []int64
	for _, f := range strings.Split(*filesFlag, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		id, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return fmt.Errorf("bad file id %q", f)
		}
		files = append(files, id)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, *dbURL)
	if err != nil {
		return err
	}
	defer st.Close()

	start := time.Now()
	sum, err := recon.Run(ctx, st, match.DefaultConfig(), files, func(e recon.Event) {
		fmt.Printf("%7s  %-8s %s\n", time.Since(start).Round(time.Millisecond), e.Stage, e.Message)
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nrun %d: %d ledger + %d custodian entries, output %s\n", sum.RunID, sum.Ledger, sum.Custodian, sum.OutputHash[:16])
	printCounts("matches", sum.Matches)
	printCounts("breaks", sum.Breaks)
	return nil
}

func printCounts(title string, m map[string]int) {
	keys := make([]string, 0, len(m))
	total := 0
	for k, v := range m {
		keys = append(keys, k)
		total += v
	}
	slices.Sort(keys)
	fmt.Printf("%s (%d)\n", title, total)
	for _, k := range keys {
		fmt.Printf("  %-24s %8d\n", k, m[k])
	}
}
