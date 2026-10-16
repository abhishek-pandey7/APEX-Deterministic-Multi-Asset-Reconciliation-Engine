package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// EnqueueJob adds work for the Python AI worker (tools/apexai), which claims jobs with
// FOR UPDATE SKIP LOCKED.
func (s *Store) EnqueueJob(ctx context.Context, kind string, payload any) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2) RETURNING id`, kind, raw).Scan(&id)
	return id, err
}

// RequestSuggestion queues an AI suggestion for a break. If one is already queued or running for
// that break, its job id is returned instead of adding a duplicate.
func (s *Store) RequestSuggestion(ctx context.Context, breakID int64) (jobID int64, existing bool, err error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM breaks WHERE id = $1)`, breakID).Scan(&exists); err != nil {
		return 0, false, err
	}
	if !exists {
		return 0, false, ErrNotFound
	}
	err = s.pool.QueryRow(ctx, `
		SELECT id FROM jobs
		WHERE kind = 'suggest_break' AND payload ->> 'break_id' = $1 AND status IN ('QUEUED', 'RUNNING')
		ORDER BY id LIMIT 1`, strconv.FormatInt(breakID, 10)).Scan(&jobID)
	if err == nil {
		return jobID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, err
	}
	jobID, err = s.EnqueueJob(ctx, "suggest_break", map[string]int64{"break_id": breakID})
	return jobID, false, err
}

type SimilarBreak struct {
	ID         int64   `json:"id"`
	RunID      int64   `json:"run_id"`
	Category   string  `json:"category"`
	Amount     string  `json:"amount"`
	Status     string  `json:"status"`
	Resolution *string `json:"resolution"`
}

type Suggestion struct {
	ID           int64          `json:"id"`
	Model        string         `json:"model"`
	Category     *string        `json:"category"`
	Confidence   *int           `json:"confidence"`
	Explanation  string         `json:"explanation"`
	NextAction   string         `json:"next_action"`
	ActionDetail string         `json:"action_detail"`
	LatencyMS    *int           `json:"latency_ms"`
	CreatedAt    time.Time      `json:"created_at"`
	Similar      []SimilarBreak `json:"similar"`
}

// suggestions returns a break's suggestions, newest first, each with its similar breaks and
// how those ended; and whether another suggestion is still being prepared.
func (s *Store) suggestions(ctx context.Context, breakID int64) ([]Suggestion, bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, model, category, confidence, explanation, next_action, action_detail, latency_ms,
		       created_at, similar_break_ids
		FROM break_suggestions WHERE break_id = $1 ORDER BY id DESC LIMIT 5`, breakID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Suggestion{}
	var similarIDs [][]int64
	for rows.Next() {
		var sg Suggestion
		var ids []int64
		var conf *int16
		if err := rows.Scan(&sg.ID, &sg.Model, &sg.Category, &conf, &sg.Explanation, &sg.NextAction,
			&sg.ActionDetail, &sg.LatencyMS, &sg.CreatedAt, &ids); err != nil {
			return nil, false, err
		}
		if conf != nil {
			c := int(*conf)
			sg.Confidence = &c
		}
		out = append(out, sg)
		similarIDs = append(similarIDs, ids)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	for i, ids := range similarIDs {
		out[i].Similar = []SimilarBreak{}
		if len(ids) == 0 {
			continue
		}
		srows, err := s.pool.Query(ctx, `
			SELECT b.id, b.run_id, b.category, b.amount_minor, b.currency, s.status,
			       (SELECT h.reason FROM break_events h
			         WHERE h.break_id = b.id AND h.actor <> 'engine' AND h.reason <> ''
			         ORDER BY h.id DESC LIMIT 1)
			FROM unnest($1::bigint[]) WITH ORDINALITY AS u(id, ord)
			JOIN breaks b ON b.id = u.id `+latestEvent+`
			ORDER BY u.ord`, ids)
		if err != nil {
			return nil, false, err
		}
		for srows.Next() {
			var sb SimilarBreak
			var amount int64
			var ccy string
			if err := srows.Scan(&sb.ID, &sb.RunID, &sb.Category, &amount, &ccy, &sb.Status, &sb.Resolution); err != nil {
				srows.Close()
				return nil, false, err
			}
			sb.Amount = formatAmount(amount, ccy)
			out[i].Similar = append(out[i].Similar, sb)
		}
		srows.Close()
		if err := srows.Err(); err != nil {
			return nil, false, err
		}
	}

	var pending bool
	err = s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM jobs WHERE kind = 'suggest_break' AND payload ->> 'break_id' = $1
		               AND status IN ('QUEUED', 'RUNNING'))`, strconv.FormatInt(breakID, 10)).Scan(&pending)
	return out, pending, err
}
