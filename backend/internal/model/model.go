// Package model holds the shared types and the normalization used wherever a
// query string is stored or looked up.
package model

import "strings"

// Suggestion is one item returned by GET /suggest.
type Suggestion struct {
	Query string  `json:"query"`
	Score float64 `json:"score"`
}

// Candidate carries the fields needed to rank a query. In count mode only Count
// matters; RecentValue/RecentTS are used in recency mode.
type Candidate struct {
	Query       string
	Count       int64
	RecentValue float64
	RecentTS    int64
}

// Normalize trims surrounding whitespace and lowercases, so that case- and
// space-variants of a query hit the same Trie path and cache key. Must be
// applied identically on load and on lookup, or stored data becomes unreachable.
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
