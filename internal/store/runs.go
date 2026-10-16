package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
)

// LoadCashEntries reads stored cash entries, split by side, in id order. With no file ids it
// reads every cash file; it also returns the ids of the files it read.
func (s *Store) LoadCashEntries(ctx context.Context, fileIDs []int64) (ledger, custodian []domain.CashEntry, files []int64, err error) {
	if len(fileIDs) == 0 {
		rows, err := s.pool.Query(ctx, `SELECT id FROM source_files WHERE kind = 'CASH' ORDER BY id`)
		if err != nil {
			return nil, nil, nil, err
		}
		fileIDs, err = pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return nil, nil, nil, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, side, external_id, account_id, currency, amount_minor, value_date, trade_date,
		       txn_type, reference, counterparty
		FROM cash_entries WHERE file_id = ANY($1) ORDER BY id`, fileIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e domain.CashEntry
		var side string
		var value time.Time
		var trade *time.Time
		if err := rows.Scan(&e.ID, &side, &e.ExternalID, &e.AccountID, &e.Currency, &e.AmountMinor,
			&value, &trade, &e.TxnType, &e.Reference, &e.Counterparty); err != nil {
			return nil, nil, nil, err
		}
		e.Side = domain.Side(side)
		e.ValueDate = domain.DateOf(value)
		if trade != nil {
			e.TradeDate, e.HasTradeDate = domain.DateOf(*trade), true
		}
		if e.Side == domain.Ledger {
			ledger = append(ledger, e)
		} else {
			custodian = append(custodian, e)
		}
	}
	return ledger, custodian, fileIDs, rows.Err()
}

// RunMeta is what a run records about itself so that it can be reproduced later.
type RunMeta struct {
	EngineVersion string
	RuleVersion   string
	Config        any
	InputFileIDs  []int64
}

// StartRun records a run as RUNNING and returns its id.
func (s *Store) StartRun(ctx context.Context, m RunMeta) (int64, error) {
	cfg, err := json.Marshal(m.Config)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO match_runs (engine_version, rule_version, config, input_file_ids)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		m.EngineVersion, m.RuleVersion, cfg, m.InputFileIDs).Scan(&id)
	return id, err
}

// FinishRun marks a run SUCCEEDED with its statistics and output hash.
func (s *Store) FinishRun(ctx context.Context, runID int64, stats any, outputHash [32]byte) error {
	raw, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE match_runs SET status = 'SUCCEEDED', stats = $2, output_sha256 = $3, finished_at = now()
		WHERE id = $1`, runID, raw, outputHash[:])
	return err
}

// FailRun marks a run FAILED, keeping the error in its stats.
func (s *Store) FailRun(ctx context.Context, runID int64, cause error) error {
	raw, _ := json.Marshal(map[string]string{"error": cause.Error()})
	_, err := s.pool.Exec(ctx, `
		UPDATE match_runs SET status = 'FAILED', stats = $2, finished_at = now() WHERE id = $1`, runID, raw)
	return err
}

// MatchRow is a match ready to store; legs are cash_entries ids.
type MatchRow struct {
	Phase, Disposition, Rule string
	RefSim                   int32
	Ledger, Custodian        []int64
}

// BreakRow is a break ready to store; legs are cash_entries ids.
type BreakRow struct {
	Category, AccountID, Currency, Reason string
	AmountMinor                           int64
	OpenedOn                              domain.Date
	Legs                                  []int64
}

// SaveResults writes a run's matches and breaks, and opens every break in the audit log as
// actor "engine", all in one transaction. Ids are reserved from the sequences up front so that
// every table can be bulk-loaded with COPY.
func (s *Store) SaveResults(ctx context.Context, runID int64, matches []MatchRow, breaks []BreakRow) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	matchIDs, err := reserveIDs(ctx, tx, "matches", len(matches))
	if err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"matches"},
		[]string{"id", "run_id", "phase", "disposition", "ref_sim", "rule"},
		pgx.CopyFromSlice(len(matches), func(i int) ([]any, error) {
			m := matches[i]
			return []any{matchIDs[i], runID, m.Phase, m.Disposition, m.RefSim, m.Rule}, nil
		})); err != nil {
		return fmt.Errorf("store: matches: %w", err)
	}
	var legs [][]any
	for i, m := range matches {
		for _, e := range append(append([]int64(nil), m.Ledger...), m.Custodian...) {
			legs = append(legs, []any{matchIDs[i], runID, m.Disposition, e})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"match_legs"},
		[]string{"match_id", "run_id", "disposition", "entry_id"}, pgx.CopyFromRows(legs)); err != nil {
		return fmt.Errorf("store: match legs: %w", err)
	}

	breakIDs, err := reserveIDs(ctx, tx, "breaks", len(breaks))
	if err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"breaks"},
		[]string{"id", "run_id", "kind", "category", "account_id", "currency", "amount_minor", "opened_on", "reason"},
		pgx.CopyFromSlice(len(breaks), func(i int) ([]any, error) {
			b := breaks[i]
			return []any{breakIDs[i], runID, "CASH", b.Category, b.AccountID, b.Currency, b.AmountMinor,
				b.OpenedOn.Time(), b.Reason}, nil
		})); err != nil {
		return fmt.Errorf("store: breaks: %w", err)
	}
	var blegs [][]any
	for i, b := range breaks {
		for _, e := range b.Legs {
			blegs = append(blegs, []any{breakIDs[i], e})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"break_legs"},
		[]string{"break_id", "cash_entry_id"}, pgx.CopyFromRows(blegs)); err != nil {
		return fmt.Errorf("store: break legs: %w", err)
	}
	// The transition trigger checks every OPEN event, so the audit trail starts out valid.
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"break_events"},
		[]string{"break_id", "status", "actor", "reason"},
		pgx.CopyFromSlice(len(breaks), func(i int) ([]any, error) {
			return []any{breakIDs[i], "OPEN", "engine", breaks[i].Reason}, nil
		})); err != nil {
		return fmt.Errorf("store: break events: %w", err)
	}
	return tx.Commit(ctx)
}

func reserveIDs(ctx context.Context, tx pgx.Tx, table string, n int) ([]int64, error) {
	rows, err := tx.Query(ctx, `SELECT nextval(pg_get_serial_sequence($1, 'id')) FROM generate_series(1, $2)`, table, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}
