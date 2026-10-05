package main

import (
	"log/slog"
	"testing"
	"time"
)

// commit builds a commit event for the given operation and collection.
func commit(op, collection string) Event {
	return Event{Kind: "commit", Commit: &Commit{Operation: op, Collection: collection}}
}

// TestRouteOf checks that each event type reaches its lane and everything else is ignored.
func TestRouteOf(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{"post create", commit("create", "app.bsky.feed.post"), lanePost},
		{"like create", commit("create", "app.bsky.feed.like"), laneEngagement},
		{"repost create", commit("create", "app.bsky.feed.repost"), laneEngagement},
		{"follow create", commit("create", "app.bsky.graph.follow"), laneFollow},
		{"any delete", commit("delete", "app.bsky.feed.like"), laneDelete},
		{"post update", commit("update", "app.bsky.feed.post"), ""},
		{"block create", commit("create", "app.bsky.graph.block"), ""},
		{"identity event", Event{Kind: "identity"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeOf(tt.event); got != tt.want {
				t.Errorf("routeOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDispatchIsolatesSlowLane checks that a stuck lane drops its own overflow while other lanes keep working.
func TestDispatchIsolatesSlowLane(t *testing.T) {
	release := make(chan struct{})
	followed := make(chan struct{})
	d := NewDispatcher(map[string]func(Event){
		lanePost:   func(Event) { <-release },
		laneFollow: func(Event) { close(followed) },
	}, 1)

	for range 5 {
		d.Dispatch(commit("create", "app.bsky.feed.post"))
	}
	d.Dispatch(commit("create", "app.bsky.graph.follow"))

	select {
	case <-followed:
	case <-time.After(time.Second):
		t.Fatal("follow lane was blocked by the stuck post lane")
	}
	if got := d.lanes[lanePost].dropped.Load(); got < 3 {
		t.Errorf("post lane dropped %d, want at least 3", got)
	}
	close(release)
	d.Close()
}

// TestDispatchRecoversFromPanic checks that a lane keeps processing after its handler panics.
func TestDispatchRecoversFromPanic(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(prev) })

	calls := 0
	d := NewDispatcher(map[string]func(Event){
		laneDelete: func(Event) {
			if calls++; calls == 1 {
				panic("boom")
			}
		},
	}, 10)
	d.Dispatch(commit("delete", "app.bsky.feed.post"))
	d.Dispatch(commit("delete", "app.bsky.feed.post"))
	d.Close()

	if got := d.lanes[laneDelete].processed.Load(); got != 2 {
		t.Errorf("processed = %d, want 2", got)
	}
}
