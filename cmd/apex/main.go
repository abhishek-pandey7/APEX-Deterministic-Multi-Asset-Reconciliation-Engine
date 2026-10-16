// Command apex is the reconciliation engine's command-line entry point.
//
//	apex ingest -layout custodian-cash-v1 data/small/custodian_cash.csv
//	apex run
//	apex eval -data data/small
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/ingest"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

const defaultDB = "postgres://apex:apex@localhost:55432/apex?sslmode=disable"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "ingest":
		err = runIngest(os.Args[2:])
	case "eval":
		err = runEval(os.Args[2:])
	case "run":
		err = runRecon(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "weights":
		err = runWeights(os.Args[2:])
	case "layouts":
		fmt.Println(strings.Join(ingest.LayoutNames(), "\n"))
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "apex:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: apex ingest -layout NAME FILE...
       apex run [-files IDS]
       apex serve [-addr HOST:PORT]
       apex eval -data DIR
       apex layouts`)
	os.Exit(2)
}

func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	layoutName := fs.String("layout", "", "input layout (see `apex layouts`)")
	dbURL := fs.String("db", envOr("APEX_DB_URL", defaultDB), "Postgres URL")
	fs.Parse(args) //nolint:errcheck // ExitOnError
	if *layoutName == "" || fs.NArg() == 0 {
		fs.Usage()
		return errors.New("need -layout and at least one file")
	}
	cashLayout, posLayout, err := ingest.Lookup(*layoutName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	st, err := store.Open(ctx, *dbURL)
	if err != nil {
		return err
	}
	defer st.Close()

	failed := false
	for _, path := range fs.Args() {
		start := time.Now()
		rows, fileID, err := ingestFile(ctx, st, path, cashLayout, posLayout)
		name := filepath.Base(path)
		switch {
		case errors.Is(err, store.ErrAlreadyLoaded):
			fmt.Printf("%s: already loaded, skipped\n", name)
		case err != nil:
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			failed = true
		default:
			fmt.Printf("%s: %d rows loaded as file %d in %s\n", name, rows, fileID, time.Since(start).Round(time.Millisecond))
		}
	}
	if failed {
		return errors.New("some files were not loaded")
	}
	return nil
}

func ingestFile(ctx context.Context, st *store.Store, path string,
	cashLayout *ingest.CashLayout, posLayout *ingest.PositionLayout) (int, int64, error) {
	fh, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer fh.Close()
	src := store.SourceFile{Filename: filepath.Base(path)}

	if cashLayout != nil {
		f, err := ingest.ParseCash(fh, *cashLayout)
		if err != nil {
			return 0, 0, err
		}
		if !f.OK() {
			return 0, 0, rowErrors(f.ErrorCount, f.Errors)
		}
		src.Side, src.Kind, src.SHA256 = cashLayout.Side, domain.KindCash, f.SHA256
		id, err := st.LoadCash(ctx, src, f.Records)
		return len(f.Records), id, err
	}

	f, err := ingest.ParsePositions(fh, *posLayout)
	if err != nil {
		return 0, 0, err
	}
	if !f.OK() {
		return 0, 0, rowErrors(f.ErrorCount, f.Errors)
	}
	src.Side, src.Kind, src.SHA256 = posLayout.Side, domain.KindPosition, f.SHA256
	id, err := st.LoadPositions(ctx, src, f.Records)
	return len(f.Records), id, err
}

// rowErrors reports the first few problems; the file is rejected as a whole.
func rowErrors(total int, errs []ingest.RowError) error {
	var b strings.Builder
	fmt.Fprintf(&b, "rejected, %d invalid rows", total)
	for i, e := range errs {
		if i == 10 {
			fmt.Fprintf(&b, "\n  ... %d more", total-i)
			break
		}
		fmt.Fprintf(&b, "\n  %v", e)
	}
	return errors.New(b.String())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
