package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/domain"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

// TestReviewWorkflow drives the API end to end against a migrated database (APEX_TEST_DB_URL):
// start a run, follow its progress stream, then take a break through the review workflow,
// including an approval the database must refuse.
func TestReviewWorkflow(t *testing.T) {
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
	files := loadFixture(t, st)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(ctx, st, match.DefaultConfig(), log).Handler(nil))
	defer srv.Close()

	// Start a run and follow it to the end.
	var started struct{ Job, Events string }
	call(t, srv, "POST", "/api/runs", "", map[string]any{"file_ids": files}, http.StatusAccepted, &started)
	stages, summary := followEvents(t, srv.URL+started.Events)
	if strings.Join(stages, ",") != "load,match,classify,save,done,summary" {
		t.Fatalf("stages = %v", stages)
	}
	var sum struct {
		RunID int64 `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(summary), &sum); err != nil || sum.RunID == 0 {
		t.Fatalf("summary %q: %v", summary, err)
	}

	var page store.BreakPage
	call(t, srv, "GET", fmt.Sprintf("/api/runs/%d/breaks", sum.RunID), "", nil, http.StatusOK, &page)
	if page.Total != 2 || page.ByStatus["OPEN"] != 2 || page.ByCategory["MISSING_IN_CUSTODIAN"] != 1 {
		t.Fatalf("breaks page = %+v", page)
	}
	b := page.Breaks[0]
	path := fmt.Sprintf("/api/breaks/%d/events", b.ID)

	// The workflow: the database enforces every rule, the API only reports.
	call(t, srv, "POST", path, "", map[string]string{"status": "ASSIGNED"}, http.StatusBadRequest, nil) // no actor
	call(t, srv, "POST", path, "ana", map[string]string{"status": "APPROVED"}, http.StatusConflict, nil)
	call(t, srv, "POST", path, "ana", map[string]string{"status": "ASSIGNED"}, http.StatusCreated, nil)
	call(t, srv, "POST", path, "ana", map[string]string{"status": "PROPOSED", "reason": "write off"}, http.StatusCreated, nil)
	var refused map[string]string
	call(t, srv, "POST", path, "ana", map[string]string{"status": "APPROVED"}, http.StatusConflict, &refused)
	if !strings.Contains(refused["error"], "approver must differ from proposer") {
		t.Errorf("self-approval refused with %q", refused["error"])
	}
	call(t, srv, "POST", path, "ben", map[string]string{"status": "APPROVED"}, http.StatusCreated, nil)
	call(t, srv, "POST", path, "ben", map[string]string{"status": "CLOSED"}, http.StatusCreated, nil)

	var detail store.BreakDetail
	call(t, srv, "GET", fmt.Sprintf("/api/breaks/%d", b.ID), "", nil, http.StatusOK, &detail)
	var trail []string
	for _, e := range detail.History {
		trail = append(trail, e.Status+":"+e.Actor)
	}
	if strings.Join(trail, " ") != "OPEN:engine ASSIGNED:ana PROPOSED:ana APPROVED:ben CLOSED:ben" {
		t.Errorf("history = %v", trail)
	}
	if detail.Status != "CLOSED" || len(detail.Entries) != 1 {
		t.Errorf("detail = %+v", detail)
	}

	// Asking twice for a suggestion queues one job, and the break shows it as pending.
	var first, second struct {
		JobID         int64 `json:"job_id"`
		AlreadyQueued bool  `json:"already_queued"`
	}
	sugPath := fmt.Sprintf("/api/breaks/%d/suggestions", b.ID)
	call(t, srv, "POST", sugPath, "", nil, http.StatusAccepted, &first)
	call(t, srv, "POST", sugPath, "", nil, http.StatusAccepted, &second)
	if first.JobID == 0 || first.AlreadyQueued || second.JobID != first.JobID || !second.AlreadyQueued {
		t.Errorf("suggestion requests: first %+v, second %+v", first, second)
	}
	call(t, srv, "GET", fmt.Sprintf("/api/breaks/%d", b.ID), "", nil, http.StatusOK, &detail)
	if !detail.SuggestionPending || len(detail.Suggestions) != 0 {
		t.Errorf("pending=%v suggestions=%d", detail.SuggestionPending, len(detail.Suggestions))
	}
	call(t, srv, "POST", "/api/breaks/999999999/suggestions", "", nil, http.StatusNotFound, nil)

	call(t, srv, "GET", "/api/breaks/999999999", "", nil, http.StatusNotFound, nil)
	call(t, srv, "GET", "/api/jobs/nope/events", "", nil, http.StatusNotFound, nil)
}

func loadFixture(t *testing.T, st *store.Store) []int64 {
	t.Helper()
	acct := fmt.Sprintf("API%d", time.Now().UnixNano())
	day := domain.DateOf(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	e := func(side domain.Side, id string, amount int64, ref string) domain.CashEntry {
		return domain.CashEntry{Side: side, ExternalID: acct + id, AccountID: acct, Currency: "USD",
			AmountMinor: amount, ValueDate: day, TxnType: "TEST", Reference: ref}
	}
	var ids []int64
	for side, entries := range map[domain.Side][]domain.CashEntry{
		domain.Ledger: {e(domain.Ledger, "L1", 100_000, "TRD-2026-0000001"), e(domain.Ledger, "L2", -2_500, "FEE-0000002")},
		domain.Custodian: {e(domain.Custodian, "C1", 100_000, "SETTLEMENT REF TRD20260000001"),
			e(domain.Custodian, "C2", -999, "CHARGES REF ACCOUNT MAINTENANCE")},
	} {
		src := store.SourceFile{Side: side, Kind: domain.KindCash, Filename: acct + ".csv"}
		if _, err := rand.Read(src.SHA256[:]); err != nil {
			t.Fatal(err)
		}
		id, err := st.LoadCash(context.Background(), src, entries)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func call(t *testing.T, srv *httptest.Server, method, path, actor string, body any, want int, out any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rd)
	if actor != "" {
		req.Header.Set("X-Actor", actor)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: %v in %s", method, path, err, raw)
		}
	}
}

// followEvents reads an SSE stream to the end and returns the event names and the last data line.
func followEvents(t *testing.T, url string) (stages []string, last string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			stages = append(stages, name)
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var e struct{ Message string }
			_ = json.Unmarshal([]byte(data), &e)
			last = e.Message
		}
	}
	return stages, last
}
