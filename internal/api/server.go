// Package api serves the reconciliation engine over HTTP: runs, live run progress over
// Server-Sent Events, and the break review workflow.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/match"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/recon"
	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/store"
)

type Server struct {
	store *store.Store
	cfg   match.Config
	jobs  *jobs
	log   *slog.Logger
	// ctx outlives requests: a run keeps going after the client that started it disconnects.
	ctx context.Context
}

func New(ctx context.Context, st *store.Store, cfg match.Config, log *slog.Logger) *Server {
	return &Server{store: st, cfg: cfg, jobs: newJobs(), log: log, ctx: ctx}
}

// Handler routes the API. ui, if not nil, serves the web app for every other path.
func (s *Server) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "engine": match.Version})
	})
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs", s.startRun)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/runs/{id}/breaks", s.listBreaks)
	mux.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)
	mux.HandleFunc("GET /api/breaks/{id}", s.getBreak)
	mux.HandleFunc("POST /api/breaks/{id}/events", s.addBreakEvent)
	mux.HandleFunc("POST /api/breaks/{id}/suggestions", s.requestSuggestion)
	if ui != nil {
		mux.Handle("/", ui)
	}
	return s.logRequests(mux)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(r.Context(), 50)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	run, err := s.store.GetRun(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// startRun begins a reconciliation in the background and returns the job whose progress can be
// followed at /api/jobs/{id}/events.
func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "body must be JSON like {\"file_ids\": [1, 2]}")
			return
		}
	}
	j := s.jobs.start()
	if j == nil {
		writeError(w, http.StatusConflict, "a run is already in progress")
		return
	}
	go func() {
		defer s.jobs.finish(j)
		sum, err := recon.Run(s.ctx, s.store, s.cfg, body.FileIDs, func(e recon.Event) { j.publish(e, false) })
		if err != nil {
			s.log.Error("run failed", "job", j.ID, "err", err)
			j.publish(recon.Event{Stage: "failed", Message: err.Error(), RunID: sum.RunID}, true)
			return
		}
		j.publish(recon.Event{Stage: "summary", Message: summaryJSON(sum), RunID: sum.RunID}, true)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID, "events": "/api/jobs/" + j.ID + "/events"})
}

// jobEvents streams a job's progress as Server-Sent Events: everything so far, then live
// events until the job finishes. A comment line every 15 s keeps proxies from closing it.
func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	j := s.jobs.get(r.PathValue("id"))
	if j == nil {
		writeError(w, http.StatusNotFound, "no such job")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	sent := 0
	for {
		events, done, wait := j.since(sent)
		for _, e := range events {
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Stage, data)
			sent++
		}
		flusher.Flush()
		if done {
			return
		}
		select {
		case <-wait:
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) listBreaks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.BreakFilter{
		RunID: id, Category: q.Get("category"), Status: q.Get("status"), Account: q.Get("account"),
		Limit: clamp(atoi(q.Get("limit"), 100), 1, 500), Offset: max(atoi(q.Get("offset"), 0), 0),
	}
	page, err := s.store.ListBreaks(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getBreak(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	b, err := s.store.GetBreak(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// addBreakEvent records an analyst action on a break. The actor comes from the X-Actor header;
// this demo has no login, so the header stands in for the authenticated user.
func (s *Server) addBreakEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	actor := strings.TrimSpace(r.Header.Get("X-Actor"))
	if actor == "" || len(actor) > 64 || actor == "engine" {
		writeError(w, http.StatusBadRequest, "set the X-Actor header to who is acting (not \"engine\")")
		return
	}
	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "body must be JSON like {\"status\": \"ASSIGNED\", \"reason\": \"...\"}")
		return
	}
	e, err := s.store.AddBreakEvent(r.Context(), id, strings.ToUpper(body.Status), actor, body.Reason)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// requestSuggestion queues an advisory AI suggestion for a break; the Python worker in
// tools/apexai writes it, and it appears in the break's detail when ready.
func (s *Server) requestSuggestion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	job, existing, err := s.store.RequestSuggestion(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job, "already_queued": existing})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	var rejected *store.RejectedError
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.As(err, &rejected):
		writeError(w, http.StatusConflict, rejected.Msg)
	default:
		s.log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.log.Info("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func atoi(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return fallback
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

func summaryJSON(sum recon.Summary) string {
	b, _ := json.Marshal(sum)
	return string(b)
}
