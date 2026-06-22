// Package cache provides a distributed suggestion cache: N logical nodes behind
// a consistent-hash ring. Each node is an in-process LRU+TTL map by default, and
// the same routing works unchanged against Redis-backed nodes (optional mode).
//
// Entries hold the per-prefix candidate set, not a finished ordering, so the
// engine can re-rank per request. Recency mode needs this because its order
// shifts with elapsed time even when no writes happen.
package cache

import (
	"sync/atomic"
	"time"

	"searchtypeahead/internal/hashring"
	"searchtypeahead/internal/model"
)

// State is the outcome of a non-mutating debug probe.
type State string

const (
	StateHit     State = "hit"
	StateMiss    State = "miss"
	StateExpired State = "expired"
)

// Node is one logical cache node.
type Node interface {
	ID() string
	Get(key string) ([]model.Candidate, bool) // hot-path read, updates LRU
	Set(key string, val []model.Candidate, ttl time.Duration)
	Peek(key string) State // non-mutating, for GET /cache/debug
	Delete(key string)     // for cache invalidation
	Len() int
}

// Distributed routes keys to nodes via the ring and tracks hit/miss counts.
type Distributed struct {
	ring  *hashring.Ring
	nodes map[string]Node
	ttl   time.Duration
	hits  atomic.Int64
	miss  atomic.Int64
}

// NewDistributed builds the cache over the given ring and nodes.
func NewDistributed(ring *hashring.Ring, nodes map[string]Node, ttl time.Duration) *Distributed {
	return &Distributed{ring: ring, nodes: nodes, ttl: ttl}
}

// Get returns the cached candidate set for the (normalized) key, if present.
func (d *Distributed) Get(key string) ([]model.Candidate, bool) {
	n := d.nodes[d.ring.Get(key)]
	if n == nil {
		d.miss.Add(1)
		return nil, false
	}
	v, ok := n.Get(key)
	if ok {
		d.hits.Add(1)
	} else {
		d.miss.Add(1)
	}
	return v, ok
}

// Set stores the candidate set for the key on its owning node with the TTL.
func (d *Distributed) Set(key string, val []model.Candidate) {
	if n := d.nodes[d.ring.Get(key)]; n != nil {
		n.Set(key, val, d.ttl)
	}
}

// Invalidate removes the key from its owning node (no-op if absent).
func (d *Distributed) Invalidate(key string) {
	if n := d.nodes[d.ring.Get(key)]; n != nil {
		n.Delete(key)
	}
}

// DebugInfo describes how a prefix routes and whether it is cached.
type DebugInfo struct {
	Prefix   string `json:"prefix"`    // the raw caller input
	Key      string `json:"key"`       // normalized routing key
	Hash     uint64 `json:"hash"`      // ring position of the key
	Node     string `json:"node"`      // owning cache node
	VnodePos uint64 `json:"vnode_pos"` // the vnode it mapped to
	State    State  `json:"state"`     // hit | miss | expired
}

// Debug returns routing + presence info without mutating the cache or metrics.
func (d *Distributed) Debug(key string) DebugInfo {
	node, hash, vpos, ok := d.ring.Lookup(key)
	info := DebugInfo{Prefix: key, Key: key, Hash: hash, Node: node, VnodePos: vpos, State: StateMiss}
	if ok {
		if n := d.nodes[node]; n != nil {
			info.State = n.Peek(key)
		}
	}
	return info
}

// Stats reports hit/miss counters and per-node sizes.
type Stats struct {
	Hits    int64          `json:"hits"`
	Misses  int64          `json:"misses"`
	HitRate float64        `json:"hit_rate"`
	Nodes   map[string]int `json:"nodes"` // node id → entries held
}

// Stats snapshots the current cache statistics.
func (d *Distributed) Stats() Stats {
	h, m := d.hits.Load(), d.miss.Load()
	rate := 0.0
	if h+m > 0 {
		rate = float64(h) / float64(h+m)
	}
	ns := make(map[string]int, len(d.nodes))
	for id, n := range d.nodes {
		ns[id] = n.Len()
	}
	return Stats{Hits: h, Misses: m, HitRate: rate, Nodes: ns}
}
