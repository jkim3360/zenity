package main

import (
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// Lane names. Each lane is a bounded queue drained by its own worker.
const (
	lanePost       = "post"
	laneEngagement = "engagement"
	laneFollow     = "follow"
	laneDelete     = "delete"
)

// Event is the part of a Jetstream event the router reads.
type Event struct {
	Did    string  `json:"did"`
	TimeUS int64   `json:"time_us"`
	Kind   string  `json:"kind"`
	Commit *Commit `json:"commit"`
}

// Commit is the repository change carried by a commit event. Record stays raw so each handler decodes only what it needs.
type Commit struct {
	Operation  string          `json:"operation"`
	Collection string          `json:"collection"`
	Rkey       string          `json:"rkey"`
	Record     json.RawMessage `json:"record"`
}

// routeOf returns the lane for an event from its kind, operation and collection, or "" if no lane handles it.
func routeOf(e Event) string {
	if e.Kind != "commit" || e.Commit == nil {
		return ""
	}
	if e.Commit.Operation == "delete" {
		return laneDelete
	}
	if e.Commit.Operation != "create" {
		return ""
	}
	switch e.Commit.Collection {
	case "app.bsky.feed.post":
		return lanePost
	case "app.bsky.feed.like", "app.bsky.feed.repost":
		return laneEngagement
	case "app.bsky.graph.follow":
		return laneFollow
	}
	return ""
}

// lane is one bounded queue, the handler its worker runs, and its counters.
type lane struct {
	ch        chan Event
	handle    func(Event)
	processed atomic.Int64
	dropped   atomic.Int64
}

// Dispatcher fans events out to one bounded lane per event type.
type Dispatcher struct {
	lanes map[string]*lane
	wg    sync.WaitGroup
}

// NewDispatcher starts one worker per handler, each fed by its own queue of queueSize events.
func NewDispatcher(handlers map[string]func(Event), queueSize int) *Dispatcher {
	d := &Dispatcher{lanes: make(map[string]*lane, len(handlers))}
	for name, handle := range handlers {
		l := &lane{ch: make(chan Event, queueSize), handle: handle}
		d.lanes[name] = l
		d.wg.Go(func() { l.work(name) })
	}
	return d
}

// work runs the lane's handler on each queued event until the queue is closed and empty.
func (l *lane) work(name string) {
	for e := range l.ch {
		l.run(name, e)
	}
}

// run handles one event and counts it, recovering from a panic so one bad event cannot stop the lane.
func (l *lane) run(name string, e Event) {
	defer l.processed.Add(1)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("handler failed", "lane", name, "error", r)
		}
	}()
	l.handle(e)
}

// Dispatch routes an event to its lane without blocking, dropping and counting it if that lane is full.
func (d *Dispatcher) Dispatch(e Event) {
	l, ok := d.lanes[routeOf(e)]
	if !ok {
		return
	}
	select {
	case l.ch <- e:
	default:
		l.dropped.Add(1)
	}
}

// LogStats logs each lane's queue depth and its processed and dropped counts.
func (d *Dispatcher) LogStats() {
	for _, name := range slices.Sorted(maps.Keys(d.lanes)) {
		l := d.lanes[name]
		slog.Info("lane stats", "lane", name, "queued", len(l.ch), "processed", l.processed.Load(), "dropped", l.dropped.Load())
	}
}

// Close stops the lanes from accepting events and waits for each to drain its backlog.
func (d *Dispatcher) Close() {
	for _, l := range d.lanes {
		close(l.ch)
	}
	d.wg.Wait()
}
