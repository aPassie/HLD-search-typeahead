// Package trie is the in-memory prefix index. Each node stores a precomputed
// top-k of its subtree by count, so a prefix lookup is O(len(prefix)) to walk
// plus O(1) to return the list.
//
// Bulk path: Insert + Finalize, used to build from the store at startup.
// Runtime path: ApplyBatch, which applies count increments from the batch
// writer. Counts only increase, so each affected node's top-k is updated by
// bubbling the changed query up its root path; no sibling merges needed.
package trie

import (
	"sort"
	"sync"

	"searchtypeahead/internal/model"
)

type node struct {
	children map[rune]*node
	count    int64  // >0 if this node terminates a query
	query    string // the full query string if terminal
	top      []model.Candidate
}

// Trie is a prefix tree with precomputed per-node top-k. Safe for concurrent
// reads (TopK) alongside a single writer (ApplyBatch).
type Trie struct {
	mu   sync.RWMutex
	root *node
	k    int // size of the precomputed list stored per node
}

// New returns an empty Trie that keeps up to k candidates per node.
func New(k int) *Trie {
	if k < 1 {
		k = 10
	}
	return &Trie{root: &node{children: map[rune]*node{}}, k: k}
}

// Insert sets a query's count. Bulk build only (call Finalize afterward); not
// safe to interleave with serving.
func (t *Trie) Insert(query string, count int64) {
	n := t.root
	for _, r := range query {
		child := n.children[r]
		if child == nil {
			child = &node{children: map[rune]*node{}}
			n.children[r] = child
		}
		n = child
	}
	n.count = count
	n.query = query
}

// Finalize computes the top-k for every node in one post-order DFS. A node's
// top-k is the k highest-count entries among its own terminal (if any) and its
// children's top-k. This is correct: any query in a node's top-k is also in the
// top-k of the child whose subtree contains it.
func (t *Trie) Finalize() {
	var dfs func(n *node) []model.Candidate
	dfs = func(n *node) []model.Candidate {
		var cands []model.Candidate
		if n.count > 0 {
			cands = append(cands, model.Candidate{Query: n.query, Count: n.count})
		}
		for _, c := range n.children {
			cands = append(cands, dfs(c)...)
		}
		sortCandidates(cands)
		if len(cands) > t.k {
			cands = cands[:t.k]
		}
		n.top = cands
		return cands
	}
	dfs(t.root)
}

// ApplyBatch applies count increments keyed by query. New queries create their
// path. Each changed query is re-inserted into the top-k of every node along
// its root path. Takes the write lock once for the whole batch.
func (t *Trie) ApplyBatch(deltas map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for q, d := range deltas {
		if q == "" || d == 0 {
			continue
		}
		n := t.root
		prefixNodes := make([]*node, 0, len(q))
		for _, r := range q {
			child := n.children[r]
			if child == nil {
				child = &node{children: map[rune]*node{}}
				n.children[r] = child
			}
			n = child
			prefixNodes = append(prefixNodes, n)
		}
		if n.count == 0 {
			n.query = q
		}
		n.count += d
		for _, anc := range prefixNodes {
			anc.upsertTop(q, n.count, t.k)
		}
	}
}

// TopK returns up to k suggestions for the normalized prefix, ordered by count
// descending, or nil if the prefix matches nothing.
func (t *Trie) TopK(prefix string, k int) []model.Candidate {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.root
	for _, r := range prefix {
		n = n.children[r]
		if n == nil {
			return nil
		}
	}
	if k > len(n.top) {
		k = len(n.top)
	}
	out := make([]model.Candidate, k)
	copy(out, n.top[:k])
	return out
}

// Count returns the stored count for an exact normalized query, or 0 if absent.
// Used to attach a base count to recency-sourced candidates.
func (t *Trie) Count(query string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := t.root
	for _, r := range query {
		n = n.children[r]
		if n == nil {
			return 0
		}
	}
	return n.count
}

// upsertTop inserts or updates query in the node's count-ordered top-k.
func (n *node) upsertTop(query string, count int64, k int) {
	for i := range n.top {
		if n.top[i].Query == query {
			n.top[i].Count = count
			sortCandidates(n.top)
			return
		}
	}
	if len(n.top) < k {
		n.top = append(n.top, model.Candidate{Query: query, Count: count})
		sortCandidates(n.top)
		return
	}
	if count > n.top[len(n.top)-1].Count {
		n.top[len(n.top)-1] = model.Candidate{Query: query, Count: count}
		sortCandidates(n.top)
	}
}

// sortCandidates orders by count descending, breaking ties by query ascending
// so the result is deterministic.
func sortCandidates(c []model.Candidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Count != c[j].Count {
			return c[i].Count > c[j].Count
		}
		return c[i].Query < c[j].Query
	})
}
