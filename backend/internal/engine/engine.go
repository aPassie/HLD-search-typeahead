// Package engine orchestrates the read/write flows across the cache, trie,
// trending index, batch writer, and store. Keeping this logic out of the HTTP
// handlers makes it testable without a server.
package engine

import (
	"log"
	"sort"
	"time"

	"searchtypeahead/internal/batch"
	"searchtypeahead/internal/cache"
	"searchtypeahead/internal/metrics"
	"searchtypeahead/internal/model"
	"searchtypeahead/internal/ranking"
	"searchtypeahead/internal/store"
	"searchtypeahead/internal/trending"
	"searchtypeahead/internal/trie"
)

// Ranking modes for /suggest.
const (
	ModeCount   = "count"   // basic: all-time popularity
	ModeRecency = "recency" // enhanced: popularity + recency
)

// Engine wires the components behind the API.
type Engine struct {
	trie     *trie.Trie
	cache    *cache.Distributed
	store    *store.Store
	trending *trending.Index
	writer   *batch.Writer
	params   ranking.Params

	latCached *metrics.Histogram
	latCold   *metrics.Histogram

	topK        int
	candidateN  int
	defaultMode string

	retry map[string]model.Candidate // rows that failed to persist; retried next flush (apply-worker only)
}

// New builds an Engine. Call SetWriter before serving writes.
func New(t *trie.Trie, c *cache.Distributed, s *store.Store, tr *trending.Index, p ranking.Params, topK, candidateN int, defaultMode string) *Engine {
	if defaultMode != ModeCount {
		defaultMode = ModeRecency
	}
	return &Engine{
		trie: t, cache: c, store: s, trending: tr, params: p,
		latCached: metrics.NewHistogram(), latCold: metrics.NewHistogram(),
		topK: topK, candidateN: candidateN, defaultMode: defaultMode,
		retry: map[string]model.Candidate{},
	}
}

// SetWriter attaches the batch writer used by Record.
func (e *Engine) SetWriter(w *batch.Writer) { e.writer = w }

// Suggest returns up to topK suggestions for the raw prefix. mode is "" (use the
// configured default), "count", or "recency". Read path: cache, else build the
// candidate set from Trie+trending, then re-rank with the request's `now`.
// Latency is recorded separately for cache hits and cold reads.
func (e *Engine) Suggest(rawPrefix, mode string) []model.Suggestion {
	now := time.Now().Unix()
	prefix := model.Normalize(rawPrefix)
	if prefix == "" {
		return e.trending.Top(e.topK, now) // empty input → trending
	}
	if mode == "" {
		mode = e.defaultMode
	}
	start := time.Now()
	cands, hit := e.cache.Get(prefix)
	if !hit {
		cands = e.candidates(prefix)
		e.cache.Set(prefix, cands)
	}
	out := e.rank(cands, mode, now)
	if hit {
		e.latCached.Observe(time.Since(start))
	} else {
		e.latCold.Observe(time.Since(start))
	}
	return out
}

// candidates is the recency-aware candidate set for a prefix: the Trie's
// count-ranked candidates unioned with prefix-matching trending queries (which
// may have low all-time count but high recency).
func (e *Engine) candidates(prefix string) []model.Candidate {
	seen := make(map[string]model.Candidate)
	for _, c := range e.trie.TopK(prefix, e.candidateN) {
		seen[c.Query] = c
	}
	for _, q := range e.trending.MatchingPrefix(prefix) {
		if _, ok := seen[q]; !ok {
			seen[q] = model.Candidate{Query: q, Count: e.trie.Count(q)}
		}
	}
	out := make([]model.Candidate, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	return out
}

// rank scores the candidate set by mode and returns the top-K. In recency mode
// every candidate is decayed to the same `now` before scoring.
func (e *Engine) rank(cands []model.Candidate, mode string, now int64) []model.Suggestion {
	type scored struct {
		q string
		s float64
	}
	arr := make([]scored, len(cands))
	for i, c := range cands {
		s := float64(c.Count)
		if mode == ModeRecency {
			s = e.params.Score(c.Count, e.trending.Effective(c.Query, now))
		}
		arr[i] = scored{c.Query, s}
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].s != arr[j].s {
			return arr[i].s > arr[j].s
		}
		return arr[i].q < arr[j].q
	})
	k := e.topK
	if k > len(arr) {
		k = len(arr)
	}
	out := make([]model.Suggestion, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, model.Suggestion{Query: arr[i].q, Score: arr[i].s})
	}
	return out
}

// Trending returns the current top-K trending queries.
func (e *Engine) Trending() []model.Suggestion {
	return e.trending.Top(e.topK, time.Now().Unix())
}

// Record submits a search to the batch writer. Returns false for empty input.
func (e *Engine) Record(rawQuery string) bool {
	q := model.Normalize(rawQuery)
	if q == "" {
		return false
	}
	e.writer.Submit(q)
	return true
}

// ApplyBatch is the batch writer's flush callback: update recency (trending),
// update counts (Trie), persist both, then invalidate affected prefixes.
func (e *Engine) ApplyBatch(deltas map[string]int64) {
	now := time.Now().Unix()
	recency := e.trending.Update(deltas, now)
	e.trie.ApplyBatch(deltas) // in-memory source of truth for counts

	// We persist absolute count (from the Trie) and recency, not deltas. Absolute
	// values are idempotent, so rows that failed on a previous flush are safe to
	// retry here.
	rows := make(map[string]model.Candidate, len(deltas)+len(e.retry))
	for q := range deltas {
		r := recency[q]
		r.Count = e.trie.Count(q)
		rows[q] = r
	}
	for q, r := range e.retry { // fold in prior failures; this batch's rows win
		if _, ok := rows[q]; !ok {
			rows[q] = r
		}
	}
	if _, err := e.store.PersistBatch(rows); err != nil {
		e.retry = rows // retry on the next flush
		log.Printf("persist batch failed (%d queries); queued for retry: %v", len(rows), err)
	} else if len(e.retry) > 0 {
		e.retry = map[string]model.Candidate{}
	}
	e.invalidatePrefixes(deltas)
}

// invalidatePrefixes drops the cache entry for every prefix of each changed
// query, since those are the prefixes whose ranking may have shifted.
func (e *Engine) invalidatePrefixes(deltas map[string]int64) {
	seen := make(map[string]struct{})
	for q := range deltas {
		runes := []rune(q)
		for i := 1; i <= len(runes); i++ {
			p := string(runes[:i])
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			e.cache.Invalidate(p)
		}
	}
}

// CacheDebug reports how a prefix routes on the ring and whether it is cached.
func (e *Engine) CacheDebug(rawPrefix string) cache.DebugInfo {
	info := e.cache.Debug(model.Normalize(rawPrefix))
	info.Prefix = rawPrefix
	return info
}

// CacheStats snapshots cache hit/miss counters and per-node sizes.
func (e *Engine) CacheStats() cache.Stats { return e.cache.Stats() }

// WriteStats snapshots batch-writer counters (write-reduction evidence).
func (e *Engine) WriteStats() batch.Stats { return e.writer.Stats() }

// LatencyStats snapshots /suggest latency, split by cache hit vs cold.
func (e *Engine) LatencyStats() map[string]metrics.Stats {
	return map[string]metrics.Stats{
		"cached": e.latCached.Snapshot(),
		"cold":   e.latCold.Snapshot(),
	}
}

// DBStats returns durable-store read/write counters.
func (e *Engine) DBStats() map[string]int64 {
	r, w := e.store.DBStats()
	return map[string]int64{"reads": r, "writes": w}
}
