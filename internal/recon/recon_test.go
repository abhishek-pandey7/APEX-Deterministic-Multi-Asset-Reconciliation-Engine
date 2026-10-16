package recon

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

// Needs a migrated database: set APEX_TEST_DB_URL. Runs leave rows behind, because matches and
// break events are append-only by design; ids are unique per test run so reruns don't collide.
func TestRunStoresAuditedResults(t *testing.T) {
	url := os.Getenv("APEX_TEST_DB_URL")
	if url == "" {
		t.Skip("APEX_TEST_DB_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A unique account keeps this test's entries apart from anything else in the database.
	acct := fmt.Sprintf("T%d", time.Now().UnixNano())
	day := domain.DateOf(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	entry := func(side domain.Side, id string, amount int64, ref string) domain.CashEntry {
		return domain.CashEntry{Side: side, ExternalID: acct + id, AccountID: acct, Currency: "USD",
			AmountMinor: amount, ValueDate: day, TxnType: "TEST", Reference: ref}
	}
	ledger := []domain.CashEntry{
		entry(domain.Ledger, "L1", 100_000, "TRD-2026-0000001"),
		entry(domain.Ledger, "L2", -2_500, "FEE-0000002"), // never settles
	}
	custodian := []domain.CashEntry{
		entry(domain.Custodian, "C1", 100_000, "SETTLEMENT REF TRD20260000001"),
		entry(domain.Custodian, "C2", 9_999, "DIVIDEND REF DIVUS0378331005"), // unbooked income
	}
	var files []int64
	for _, f := range []struct {
		side    domain.Side
		entries []domain.CashEntry
	}{{domain.Ledger, ledger}, {domain.Custodian, custodian}} {
		src := store.SourceFile{Side: f.side, Kind: domain.KindCash, Filename: acct + ".csv"}
		if _, err := rand.Read(src.SHA256[:]); err != nil {
			t.Fatal(err)
		}
		id, err := st.LoadCash(ctx, src, f.entries)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, id)
	}

	var stages []string
	sum, err := Run(ctx, st, match.DefaultConfig(), files, func(e Event) { stages = append(stages, e.Stage) })
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(stages) != "[load match classify save done]" {
		t.Errorf("stages = %v", stages)
	}
	if sum.Matches["EXACT/AUTO"] != 1 || sum.Breaks["MISSING_IN_CUSTODIAN"] != 1 || sum.Breaks["CORPORATE_ACTION"] != 1 {
		t.Errorf("summary = %+v", sum)
	}

	var status string
	var legs, breaks, openEvents int
	err = db.QueryRow(ctx, `
		SELECT r.status,
		       (SELECT count(*) FROM match_legs WHERE run_id = r.id),
		       (SELECT count(*) FROM breaks WHERE run_id = r.id),
		       (SELECT count(*) FROM break_status s JOIN breaks b ON b.id = s.break_id
		         WHERE b.run_id = r.id AND s.status = 'OPEN' AND s.actor = 'engine')
		FROM match_runs r WHERE r.id = $1`, sum.RunID).Scan(&status, &legs, &breaks, &openEvents)
	if err != nil {
		t.Fatal(err)
	}
	if status != "SUCCEEDED" || legs != 2 || breaks != 2 || openEvents != 2 {
		t.Errorf("status=%s legs=%d breaks=%d open events=%d", status, legs, breaks, openEvents)
	}

	// A second run over the same files must reproduce the same output exactly.
	again, err := Run(ctx, st, match.DefaultConfig(), files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.OutputHash != sum.OutputHash || again.RunID == sum.RunID {
		t.Errorf("rerun hash %s (run %d), first %s (run %d)", again.OutputHash, again.RunID, sum.OutputHash, sum.RunID)
	}
}
