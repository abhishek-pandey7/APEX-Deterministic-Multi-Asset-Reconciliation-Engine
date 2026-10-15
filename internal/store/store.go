// Package store persists normalised inputs and engine output in Postgres.
package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// ErrAlreadyLoaded means a file with the same SHA-256 was loaded before. Reloading is a
// no-op rather than an error the caller has to clean up after.
var ErrAlreadyLoaded = errors.New("store: file already loaded")

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// SourceFile describes one input file.
type SourceFile struct {
	Side     domain.Side
	Kind     domain.Kind
	Filename string
	SHA256   [32]byte
}

// LoadCash records the file and bulk-loads its entries in one transaction, so a file is
// either fully loaded or not at all.
func (s *Store) LoadCash(ctx context.Context, f SourceFile, entries []domain.CashEntry) (int64, error) {
	// Copy in index order: custodian feeds arrive shuffled, and random-order inserts into the
	// (account, currency, value_date) index are several times slower than sequential ones.
	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		x, y := &entries[a], &entries[b]
		return cmp.Or(
			strings.Compare(x.AccountID, y.AccountID),
			strings.Compare(x.Currency, y.Currency),
			cmp.Compare(x.ValueDate, y.ValueDate),
			strings.Compare(x.ExternalID, y.ExternalID),
		)
	})

	columns := []string{"file_id", "side", "external_id", "account_id", "currency", "amount_minor",
		"value_date", "trade_date", "txn_type", "reference", "counterparty"}
	return s.load(ctx, f, len(entries), "cash_entries", columns, func(fileID int64, i int) []any {
		e := entries[order[i]]
		var trade any
		if e.HasTradeDate {
			trade = e.TradeDate.Time()
		}
		return []any{fileID, string(e.Side), e.ExternalID, e.AccountID, e.Currency, e.AmountMinor,
			e.ValueDate.Time(), trade, e.TxnType, e.Reference, e.Counterparty}
	})
}

// LoadPositions is LoadCash for position snapshots.
func (s *Store) LoadPositions(ctx context.Context, f SourceFile, positions []domain.Position) (int64, error) {
	columns := []string{"file_id", "side", "account_id", "as_of_date", "security_id", "asset_class",
		"currency", "quantity_e6", "price_e8", "market_value_minor"}
	return s.load(ctx, f, len(positions), "positions", columns, func(fileID int64, i int) []any {
		p := positions[i]
		return []any{fileID, string(p.Side), p.AccountID, p.AsOf.Time(), p.SecurityID, p.AssetClass,
			p.Currency, p.QuantityE6, p.PriceE8, p.MarketValueMinor}
	})
}

func (s *Store) load(ctx context.Context, f SourceFile, n int, table string, columns []string,
	row func(fileID int64, i int) []any) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	var fileID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO source_files (side, kind, filename, sha256, row_count)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sha256) DO NOTHING
		RETURNING id`,
		string(f.Side), string(f.Kind), f.Filename, f.SHA256[:], n,
	).Scan(&fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrAlreadyLoaded
	}
	if err != nil {
		return 0, fmt.Errorf("store: recording %s: %w", f.Filename, err)
	}

	copied, err := tx.CopyFrom(ctx, pgx.Identifier{table}, columns,
		pgx.CopyFromSlice(n, func(i int) ([]any, error) { return row(fileID, i), nil }))
	if err != nil {
		return 0, fmt.Errorf("store: loading %s: %w", f.Filename, err)
	}
	if copied != int64(n) {
		return 0, fmt.Errorf("store: loaded %d of %d rows from %s", copied, n, f.Filename)
	}
	return fileID, tx.Commit(ctx)
}
