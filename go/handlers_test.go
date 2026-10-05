package main

import (
	"slices"
	"testing"
	"time"
)

// TestWindowCounter checks that a key alerts once at the threshold, keys count separately, and the window resets.
func TestWindowCounter(t *testing.T) {
	c := newWindowCounter(time.Minute, 3)
	t0 := time.Unix(1_700_000_000, 0)
	steps := []struct {
		key  string
		at   time.Duration
		want bool
	}{
		{"a", 0, false},
		{"a", time.Second, false},
		{"b", 2 * time.Second, false},
		{"a", 3 * time.Second, true},  // third hit on a
		{"a", 4 * time.Second, false}, // fires once per window
		{"a", 61 * time.Second, false},
		{"a", 62 * time.Second, false},
		{"a", 63 * time.Second, true}, // new window, counting from zero
	}
	for i, s := range steps {
		if got := c.hit(s.key, t0.Add(s.at)); got != s.want {
			t.Errorf("step %d: hit(%q, +%v) = %v, want %v", i, s.key, s.at, got, s.want)
		}
	}
}

// TestMatchKeyword checks case-insensitive matching and misses.
func TestMatchKeyword(t *testing.T) {
	keywords := []string{"golang", "kubernetes"}
	tests := map[string]string{
		"I love GoLang":        "golang",
		"KUBERNETES at 3am":    "kubernetes",
		"hello world":          "",
		"":                     "",
		"go lang is two words": "",
	}
	for text, want := range tests {
		if got := matchKeyword(text, keywords); got != want {
			t.Errorf("matchKeyword(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestParseKeywords checks trimming, lowercasing and dropping blanks.
func TestParseKeywords(t *testing.T) {
	got := parseKeywords(" Go , ,KUBERNETES,,")
	if want := []string{"go", "kubernetes"}; !slices.Equal(got, want) {
		t.Errorf("parseKeywords() = %q, want %q", got, want)
	}
	if got := parseKeywords(" , "); len(got) != 0 {
		t.Errorf("parseKeywords(blank) = %q, want none", got)
	}
}
