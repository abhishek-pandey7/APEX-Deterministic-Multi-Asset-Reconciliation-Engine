package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/money"
)

var ErrNotFound = errors.New("store: not found")

// RejectedError is a change the database refused: an invalid break transition, or an approval
// by the person who proposed it. The rules live in triggers, so the API cannot bypass them.
type RejectedError struct{ Msg string }

func (e *RejectedError) Error() string { return e.Msg }

type Run struct {
	ID            int64           `json:"id"`
	Status        string          `json:"status"`
	EngineVersion string          `json:"engine_version"`
	RuleVersion   string          `json:"rule_version"`
	InputFileIDs  []int64         `json:"input_file_ids"`
	Stats         json.RawMessage `json:"stats"`
	OutputSHA256  string          `json:"output_sha256"`
	StartedAt     time.Time       `json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at"`
}

const runColumns = `id, status, engine_version, rule_version, input_file_ids, coalesce(stats, 'null'),
	coalesce(encode(output_sha256, 'hex'), ''), started_at, finished_at`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.Status, &r.EngineVersion, &r.RuleVersion, &r.InputFileIDs, &r.Stats,
		&r.OutputSHA256, &r.StartedAt, &r.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *Store) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+` FROM match_runs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) GetRun(ctx context.Context, id int64) (Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM match_runs WHERE id = $1`, id))
}

type Break struct {
	ID          int64     `json:"id"`
	RunID       int64     `json:"run_id"`
	Category    string    `json:"category"`
	AccountID   string    `json:"account_id"`
	Currency    string    `json:"currency"`
	AmountMinor int64     `json:"amount_minor"`
	Amount      string    `json:"amount"`
	OpenedOn    string    `json:"opened_on"`
	Reason      string    `json:"reason"`
	Status      string    `json:"status"`
	StatusBy    string    `json:"status_by"`
	StatusSince time.Time `json:"status_since"`
	Legs        int       `json:"legs"`
}

type BreakFilter struct {
	RunID                     int64
	Category, Status, Account string
	Limit, Offset             int
}

type BreakPage struct {
	Breaks     []Break        `json:"breaks"`
	Total      int            `json:"total"`
	ByCategory map[string]int `json:"by_category"`
	ByStatus   map[string]int `json:"by_status"`
}

// latestEvent joins each break to its newest audit event through the (break_id, id) index.
const latestEvent = `JOIN LATERAL (
	SELECT e.status, e.actor, e.created_at FROM break_events e
	WHERE e.break_id = b.id ORDER BY e.id DESC LIMIT 1) s ON true`

// ListBreaks returns one page of a run's breaks plus counts by category and status for the
// whole run (before the category and status filters), which drive the UI's filter chips.
func (s *Store) ListBreaks(ctx context.Context, f BreakFilter) (BreakPage, error) {
	page := BreakPage{Breaks: []Break{}, ByCategory: map[string]int{}, ByStatus: map[string]int{}}
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.run_id, b.category, b.account_id, b.currency, b.amount_minor, b.opened_on,
		       b.reason, s.status, s.actor, s.created_at,
		       (SELECT count(*) FROM break_legs l WHERE l.break_id = b.id),
		       count(*) OVER ()
		FROM breaks b `+latestEvent+`
		WHERE b.run_id = $1
		  AND ($2 = '' OR b.category = $2)
		  AND ($3 = '' OR s.status = $3)
		  AND ($4 = '' OR b.account_id = $4)
		ORDER BY b.opened_on, b.id
		LIMIT $5 OFFSET $6`, f.RunID, f.Category, f.Status, f.Account, f.Limit, f.Offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var b Break
		var opened time.Time
		if err := rows.Scan(&b.ID, &b.RunID, &b.Category, &b.AccountID, &b.Currency, &b.AmountMinor, &opened,
			&b.Reason, &b.Status, &b.StatusBy, &b.StatusSince, &b.Legs, &page.Total); err != nil {
			return page, err
		}
		b.OpenedOn = opened.Format(time.DateOnly)
		b.Amount = formatAmount(b.AmountMinor, b.Currency)
		page.Breaks = append(page.Breaks, b)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}

	counts, err := s.pool.Query(ctx, `
		SELECT b.category, s.status, count(*)
		FROM breaks b `+latestEvent+`
		WHERE b.run_id = $1 AND ($2 = '' OR b.account_id = $2)
		GROUP BY 1, 2`, f.RunID, f.Account)
	if err != nil {
		return page, err
	}
	defer counts.Close()
	for counts.Next() {
		var cat, status string
		var n int
		if err := counts.Scan(&cat, &status, &n); err != nil {
			return page, err
		}
		page.ByCategory[cat] += n
		page.ByStatus[status] += n
	}
	return page, counts.Err()
}

type Leg struct {
	EntryID    int64  `json:"entry_id"`
	Side       string `json:"side"`
	ExternalID string `json:"external_id"`
	Amount     string `json:"amount"`
	ValueDate  string `json:"value_date"`
	TxnType    string `json:"txn_type"`
	Reference  string `json:"reference"`
}

type Event struct {
	ID        int64     `json:"id"`
	Status    string    `json:"status"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type BreakDetail struct {
	Break
	Entries     []Leg        `json:"entries"`
	History     []Event      `json:"history"`
	Suggestions []Suggestion `json:"suggestions"`
	// SuggestionPending is true while the AI worker is preparing a suggestion for this break.
	SuggestionPending bool `json:"suggestion_pending"`
}

func (s *Store) GetBreak(ctx context.Context, id int64) (BreakDetail, error) {
	var d BreakDetail
	var opened time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT b.id, b.run_id, b.category, b.account_id, b.currency, b.amount_minor, b.opened_on,
		       b.reason, s.status, s.actor, s.created_at,
		       (SELECT count(*) FROM break_legs l WHERE l.break_id = b.id)
		FROM breaks b `+latestEvent+` WHERE b.id = $1`, id).Scan(
		&d.ID, &d.RunID, &d.Category, &d.AccountID, &d.Currency, &d.AmountMinor, &opened,
		&d.Reason, &d.Status, &d.StatusBy, &d.StatusSince, &d.Legs)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.OpenedOn = opened.Format(time.DateOnly)
	d.Amount = formatAmount(d.AmountMinor, d.Currency)

	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.side, e.external_id, e.currency, e.amount_minor, e.value_date, e.txn_type, e.reference
		FROM break_legs l JOIN cash_entries e ON e.id = l.cash_entry_id
		WHERE l.break_id = $1 ORDER BY e.side DESC, e.value_date, e.id`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	d.Entries = []Leg{}
	for rows.Next() {
		var l Leg
		var ccy string
		var amount int64
		var value time.Time
		if err := rows.Scan(&l.EntryID, &l.Side, &l.ExternalID, &ccy, &amount, &value, &l.TxnType, &l.Reference); err != nil {
			return d, err
		}
		l.Amount = formatAmount(amount, ccy)
		l.ValueDate = value.Format(time.DateOnly)
		d.Entries = append(d.Entries, l)
	}
	if err := rows.Err(); err != nil {
		return d, err
	}

	events, err := s.pool.Query(ctx, `
		SELECT id, status, actor, reason, created_at FROM break_events WHERE break_id = $1 ORDER BY id`, id)
	if err != nil {
		return d, err
	}
	d.History, err = pgx.CollectRows(events, pgx.RowToStructByPos[Event])
	if err != nil {
		return d, err
	}
	d.Suggestions, d.SuggestionPending, err = s.suggestions(ctx, id)
	return d, err
}

// AddBreakEvent appends a status change to a break's audit log. Transitions and the two-person
// rule are enforced by the database; a refusal comes back as *RejectedError.
func (s *Store) AddBreakEvent(ctx context.Context, breakID int64, status, actor, reason string) (Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx, `
		INSERT INTO break_events (break_id, status, actor, reason) VALUES ($1, $2, $3, $4)
		RETURNING id, status, actor, reason, created_at`, breakID, status, actor, reason).Scan(
		&e.ID, &e.Status, &e.Actor, &e.Reason, &e.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "P0001", "23514": // raised by the transition trigger, or a check constraint
			return e, &RejectedError{Msg: pgErr.Message}
		case "23503":
			return e, ErrNotFound
		}
	}
	return e, err
}

func formatAmount(minor int64, currency string) string {
	ccy, err := money.LookupCurrency(currency)
	if err != nil {
		return ""
	}
	return money.FormatGrouped(minor, ccy.Exponent) + " " + ccy.Code
}
