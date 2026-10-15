package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// Integration tests need a migrated database: set APEX_TEST_DB_URL.
func openTest(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("APEX_TEST_DB_URL")
	if url == "" {
		t.Skip("APEX_TEST_DB_URL not set")
	}
	s, err := Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// uniqueFile returns a SourceFile with a random hash, plus a cleanup of everything it loads.
func uniqueFile(t *testing.T, s *Store, kind domain.Kind) SourceFile {
	t.Helper()
	f := SourceFile{Side: domain.Ledger, Kind: kind, Filename: t.Name() + ".csv"}
	if _, err := rand.Read(f.SHA256[:]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM cash_entries WHERE file_id IN (SELECT id FROM source_files WHERE sha256 = $1)`,
			`DELETE FROM positions WHERE file_id IN (SELECT id FROM source_files WHERE sha256 = $1)`,
			`DELETE FROM source_files WHERE sha256 = $1`,
		} {
			if _, err := s.pool.Exec(ctx, q, f.SHA256[:]); err != nil {
				t.Error(err)
			}
		}
	})
	return f
}

func TestLoadCashIsIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	f := uniqueFile(t, s, domain.KindCash)

	day := domain.DateOf(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	prefix := fmt.Sprintf("T%d-", time.Now().UnixNano())
	entries := []domain.CashEntry{
		{Side: domain.Ledger, ExternalID: prefix + "1", AccountID: "ACC1", Currency: "USD",
			AmountMinor: -123450, ValueDate: day, TradeDate: day - 1, HasTradeDate: true, TxnType: "TRADE_SETTLEMENT"},
		{Side: domain.Ledger, ExternalID: prefix + "2", AccountID: "ACC1", Currency: "JPY",
			AmountMinor: 3750, ValueDate: day, TxnType: "FEE"},
	}

	id, err := s.LoadCash(ctx, f, entries)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadCash(ctx, f, entries); !errors.Is(err, ErrAlreadyLoaded) {
		t.Fatalf("second load err = %v, want ErrAlreadyLoaded", err)
	}

	var n int
	var sum int64
	var trade *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT count(*), sum(amount_minor), min(trade_date) FROM cash_entries WHERE file_id = $1`, id,
	).Scan(&n, &sum, &trade)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || sum != -119700 || trade == nil || domain.DateOf(*trade) != day-1 {
		t.Errorf("loaded n=%d sum=%d trade=%v", n, sum, trade)
	}
}

func TestLoadCashRollsBackOnFailure(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	f := uniqueFile(t, s, domain.KindCash)

	dup := domain.CashEntry{Side: domain.Ledger, ExternalID: fmt.Sprintf("D%d", time.Now().UnixNano()),
		AccountID: "ACC1", Currency: "USD", AmountMinor: 1, TxnType: "FEE"}
	if _, err := s.LoadCash(ctx, f, []domain.CashEntry{dup, dup}); err == nil {
		t.Fatal("loading duplicate ids succeeded")
	}

	var files int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM source_files WHERE sha256 = $1`, f.SHA256[:]).Scan(&files); err != nil {
		t.Fatal(err)
	}
	if files != 0 {
		t.Error("source_files row survived a failed load; the file could never be retried")
	}
}

func TestLoadPositions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	f := uniqueFile(t, s, domain.KindPosition)
	day := domain.DateOf(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))

	id, err := s.LoadPositions(ctx, f, []domain.Position{{
		Side: domain.Ledger, AccountID: fmt.Sprintf("P%d", time.Now().UnixNano()), AsOf: day,
		SecurityID: "US0378331005", AssetClass: "EQUITY", Currency: "USD",
		QuantityE6: 100_000000, PriceE8: 10125000000, MarketValueMinor: 1012500,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var mv int64
	if err := s.pool.QueryRow(ctx, `SELECT market_value_minor FROM positions WHERE file_id = $1`, id).Scan(&mv); err != nil {
		t.Fatal(err)
	}
	if mv != 1012500 {
		t.Errorf("market value = %d", mv)
	}
}
