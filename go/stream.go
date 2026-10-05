package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"
)

// collections is the wantedCollections filter: Jetstream sends only these record types (plus identity and account events).
var collections = []string{"app.bsky.feed.post", "app.bsky.feed.like", "app.bsky.feed.repost", "app.bsky.graph.follow"}

const (
	reconnectDelay = 2 * time.Second
	resumeRewind   = 5 * time.Second // replay a little on resume, so a reconnect loses nothing (at-least-once)
)

// consume reads the stream until ctx is cancelled, reconnecting from the last seen time_us after any error.
func consume(ctx context.Context, base string, dispatch func(Event)) {
	var last int64
	for {
		err := readStream(ctx, streamURL(base, last), func(e Event) {
			last = e.TimeUS
			dispatch(e)
		})
		if ctx.Err() != nil {
			return
		}
		slog.Warn("disconnected", "error", err.Error(), "retry_in", reconnectDelay.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// streamURL builds the subscribe URL with the collection filter and, once an event has been seen, a rewound cursor.
func streamURL(base string, last int64) string {
	q := url.Values{"wantedCollections": collections}
	if last > 0 {
		q.Set("cursor", strconv.FormatInt(last-resumeRewind.Microseconds(), 10))
	}
	return base + "?" + q.Encode()
}

// readStream dials u and passes each decoded event to handle until the connection fails or ctx is cancelled.
func readStream(ctx context.Context, u string, handle func(Event)) error {
	conn, _, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	slog.Info("connected", "url", u)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var e Event
		if json.Unmarshal(data, &e) != nil {
			slog.Warn("undecodable event", "bytes", len(data))
			continue
		}
		handle(e)
	}
}
