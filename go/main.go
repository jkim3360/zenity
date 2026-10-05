// Command jetstream-router reads the Bluesky Jetstream and routes each event to an independent worker lane by type.
package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// main reads config, starts the four lanes and the stream reader, and on SIGINT or SIGTERM drains the lanes and exits.
func main() {
	level := new(slog.LevelVar)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	if err := level.UnmarshalText([]byte(env("LOG_LEVEL", "info"))); err != nil {
		fatal("LOG_LEVEL must be debug, info, warn or error", "value", os.Getenv("LOG_LEVEL"))
	}
	keywords := parseKeywords(env("KEYWORDS", "golang,kubernetes"))
	if len(keywords) == 0 {
		fatal("KEYWORDS must list at least one keyword", "value", os.Getenv("KEYWORDS"))
	}
	engagementThreshold := envInt("ENGAGEMENT_THRESHOLD", 25)
	followThreshold := envInt("FOLLOW_THRESHOLD", 10)
	window := envDuration("WINDOW", time.Minute)
	queueSize := envInt("QUEUE_SIZE", 1000)
	jetstreamURL := env("JETSTREAM_URL", "wss://jetstream2.us-east.bsky.network/subscribe")
	if u, err := url.Parse(jetstreamURL); err != nil || (u.Scheme != "ws" && u.Scheme != "wss") {
		fatal("JETSTREAM_URL must be a ws:// or wss:// URL", "value", jetstreamURL)
	}
	slog.Info("starting", "keywords", keywords, "engagement_threshold", engagementThreshold, "follow_threshold", followThreshold,
		"window_seconds", window.Seconds(), "queue_size", queueSize)

	d := NewDispatcher(map[string]func(Event){
		lanePost:       postHandler(keywords),
		laneEngagement: engagementHandler(newWindowCounter(window, engagementThreshold)),
		laneFollow:     followHandler(newWindowCounter(window, followThreshold)),
		laneDelete:     deleteHandler,
	}, queueSize)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	go logStatsEvery(ctx, d, 10*time.Second)
	consume(ctx, jetstreamURL, d.Dispatch)
	stop() // a second signal now kills the process instead of waiting for the drain
	slog.Info("shutting down")
	d.Close()
	d.LogStats()
}

// logStatsEvery logs lane stats on every tick until ctx is cancelled.
func logStatsEvery(ctx context.Context, d *Dispatcher, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.LogStats()
		}
	}
}

// env returns the named environment variable, or def if it is unset or empty.
func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// envInt reads a positive integer from the environment and exits if the value is not one.
func envInt(name string, def int) int {
	v := env(name, strconv.Itoa(def))
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		fatal(name+" must be a positive integer", "value", v)
	}
	return n
}

// envDuration reads a positive Go duration such as 1m from the environment and exits if the value is not one.
func envDuration(name string, def time.Duration) time.Duration {
	v := env(name, def.String())
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		fatal(name+" must be a positive duration such as 1m", "value", v)
	}
	return d
}

// fatal logs an error and exits with status 1.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}
