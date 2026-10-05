package main

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// windowCounter counts hits per key in a tumbling window. Only its own lane's worker uses it, so it needs no lock.
type windowCounter struct {
	window    time.Duration
	threshold int
	start     time.Time
	counts    map[string]int
}

// newWindowCounter returns a counter that reports a key once per window, when it reaches threshold hits.
func newWindowCounter(window time.Duration, threshold int) *windowCounter {
	return &windowCounter{window: window, threshold: threshold, counts: map[string]int{}}
}

// hit records one event for key at event time at and reports whether the key just reached the threshold.
func (c *windowCounter) hit(key string, at time.Time) bool {
	if at.Sub(c.start) >= c.window {
		c.start = at
		clear(c.counts)
	}
	c.counts[key]++
	return c.counts[key] == c.threshold
}

// parseKeywords splits a comma-separated list into lowercase keywords, dropping blanks.
func parseKeywords(s string) []string {
	var keywords []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			keywords = append(keywords, k)
		}
	}
	return keywords
}

// matchKeyword returns the first keyword that appears in text, ignoring case, or "" if none does.
func matchKeyword(text string, keywords []string) string {
	text = strings.ToLower(text)
	for _, k := range keywords {
		if strings.Contains(text, k) {
			return k
		}
	}
	return ""
}

// postHandler returns a handler that raises a notification when a new post mentions a keyword, without logging the text.
func postHandler(keywords []string) func(Event) {
	return func(e Event) {
		var post struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Commit.Record, &post) != nil {
			return
		}
		if k := matchKeyword(post.Text, keywords); k != "" {
			slog.Info("keyword notification", "lane", lanePost, "keyword", k, "did", e.Did, "rkey", e.Commit.Rkey)
		}
	}
}

// engagementHandler returns a handler that alerts when one post's likes and reposts reach the threshold within a window.
func engagementHandler(c *windowCounter) func(Event) {
	return func(e Event) {
		var rec struct {
			Subject struct {
				URI string `json:"uri"`
			} `json:"subject"`
		}
		if json.Unmarshal(e.Commit.Record, &rec) != nil || rec.Subject.URI == "" {
			return
		}
		if c.hit(rec.Subject.URI, time.UnixMicro(e.TimeUS)) {
			slog.Info("engagement alert", "lane", laneEngagement, "subject", rec.Subject.URI, "count", c.threshold, "window_seconds", c.window.Seconds())
		}
	}
}

// followHandler returns a handler that alerts when one account gains the threshold of follows within a window.
func followHandler(c *windowCounter) func(Event) {
	return func(e Event) {
		var rec struct {
			Subject string `json:"subject"`
		}
		if json.Unmarshal(e.Commit.Record, &rec) != nil || rec.Subject == "" {
			return
		}
		if c.hit(rec.Subject, time.UnixMicro(e.TimeUS)) {
			slog.Info("follow burst", "lane", laneFollow, "account", rec.Subject, "count", c.threshold, "window_seconds", c.window.Seconds())
		}
	}
}

// deleteHandler logs each retraction at debug level, standing in for a cleanup pipeline.
func deleteHandler(e Event) {
	slog.Debug("retraction", "lane", laneDelete, "collection", e.Commit.Collection, "did", e.Did, "rkey", e.Commit.Rkey)
}
