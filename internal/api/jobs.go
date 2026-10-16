package api

import (
	"strconv"
	"sync"

	"github.com/abhishek-pandey7/APEX-Deterministic-Multi-Asset-Reconciliation-Engine/internal/recon"
)

// job is one background reconciliation. Its events are kept so that a client connecting late
// still sees the whole history before it starts following live.
type job struct {
	ID string

	mu      sync.Mutex
	events  []recon.Event
	done    bool
	waiters []chan struct{}
}

func (j *job) publish(e recon.Event, final bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.events = append(j.events, e)
	j.done = j.done || final
	for _, w := range j.waiters {
		close(w)
	}
	j.waiters = nil
}

// since returns events from index i on, whether the job has finished, and a channel that is
// closed when anything new arrives.
func (j *job) since(i int) ([]recon.Event, bool, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	w := make(chan struct{})
	if !j.done {
		j.waiters = append(j.waiters, w)
	}
	return append([]recon.Event(nil), j.events[i:]...), j.done, w
}

// jobs tracks background runs. Only one runs at a time: two concurrent runs over the same
// data would be legal but pointless, and would compete for the same database rows.
type jobs struct {
	mu      sync.Mutex
	next    int
	all     map[string]*job
	running *job
}

func newJobs() *jobs { return &jobs{all: map[string]*job{}} }

// start registers a new job, or returns nil if one is already running.
func (js *jobs) start() *job {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.running != nil {
		return nil
	}
	js.next++
	j := &job{ID: strconv.Itoa(js.next)}
	js.all[j.ID] = j
	js.running = j
	return j
}

func (js *jobs) finish(j *job) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.running == j {
		js.running = nil
	}
}

func (js *jobs) get(id string) *job {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.all[id]
}
