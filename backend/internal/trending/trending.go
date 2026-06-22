// Package trending tracks recency per searched query and serves the top-K by
// effective recency. It is the recency-aware candidate source: a query that is
// recently hot but has a low all-time count would never show up among the Trie's
// count-ranked candidates, so the engine also pulls prefix-matching entries from
// here before re-ranking.
//
// Bounded to `cap` entries (evicting the lowest time-invariant key) to keep
// memory small no matter how many distinct queries are searched.
package trending

import (
	"math"
	"sort"
	"strings"
	"sync"

	"searchtypeahead/internal/model"
	"searchtypeahead/internal/ranking"
)

type state struct {
	value float64
	ts    int64
}

// Index is the bounded recency tracker.
type Index struct {
	mu    sync.Mutex
	p     ranking.Params
	cap   int
	items map[string]state
}

// New creates an Index keeping up to cap queries.
func New(p ranking.Params, cap int) *Index {
	if cap < 1 {
		cap = 200
	}
	return &Index{p: p, cap: cap, items: make(map[string]state)}
}

// Seed loads a query's persisted recency at startup (value assumed > 0).
func (x *Index) Seed(query string, value float64, ts int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.items[query] = state{value, ts}
	x.evict()
}

// Update applies a flushed batch and returns the new recency rows to persist.
func (x *Index) Update(deltas map[string]int64, now int64) map[string]model.Candidate {
	x.mu.Lock()
	defer x.mu.Unlock()
	out := make(map[string]model.Candidate, len(deltas))
	for q, d := range deltas {
		s := x.items[q]
		v, ts := x.p.Bump(s.value, s.ts, d, now)
		x.items[q] = state{v, ts}
		out[q] = model.Candidate{Query: q, RecentValue: v, RecentTS: ts}
	}
	x.evict()
	return out
}

// Effective returns a query's decayed recency now (0 if not tracked).
func (x *Index) Effective(query string, now int64) float64 {
	x.mu.Lock()
	defer x.mu.Unlock()
	s, ok := x.items[query]
	if !ok {
		return 0
	}
	return x.p.Effective(s.value, s.ts, now)
}

// MatchingPrefix returns the tracked queries that start with prefix.
func (x *Index) MatchingPrefix(prefix string) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	var out []string
	for q := range x.items {
		if strings.HasPrefix(q, prefix) {
			out = append(out, q)
		}
	}
	return out
}

// Top returns the k queries with the highest effective recency now.
func (x *Index) Top(k int, now int64) []model.Suggestion {
	x.mu.Lock()
	defer x.mu.Unlock()
	type qe struct {
		q   string
		eff float64
	}
	arr := make([]qe, 0, len(x.items))
	for q, s := range x.items {
		arr = append(arr, qe{q, x.p.Effective(s.value, s.ts, now)})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].eff != arr[j].eff {
			return arr[i].eff > arr[j].eff
		}
		return arr[i].q < arr[j].q
	})
	if k > len(arr) {
		k = len(arr)
	}
	out := make([]model.Suggestion, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, model.Suggestion{Query: arr[i].q, Score: arr[i].eff})
	}
	return out
}

// evict drops the lowest-key entries while over capacity (caller holds the lock).
func (x *Index) evict() {
	for len(x.items) > x.cap {
		minQ, minK := "", math.Inf(1)
		for q, s := range x.items {
			if k := x.p.Key(s.value, s.ts); k < minK {
				minK, minQ = k, q
			}
		}
		delete(x.items, minQ)
	}
}
