// Package hashring implements a consistent-hash ring with virtual nodes.
//
// Keys and vnode positions both hash with MD5 (first 8 bytes as a big-endian
// uint64). MD5 is deterministic across restarts and spreads the sequential
// vnode ids ("node-0#0", "node-0#1", ...) evenly. FNV-1a is deterministic too
// but clusters those ids (~1.76x skew), and runtime/maphash is well-distributed
// but seeded randomly per process, so neither fits.
package hashring

import (
	"crypto/md5"
	"encoding/binary"
	"sort"
	"strconv"
)

type vnode struct {
	pos  uint64
	node string
	idx  int
}

// Ring maps keys to nodes via a virtual-node hash ring. Once built it is safe
// for concurrent reads (Get/Lookup). Add/Remove are not concurrent-safe and run
// only at startup or in the rebalance tool.
type Ring struct {
	vnodesPerNode int
	nodes         []string
	ring          []vnode // sorted by (pos, node, idx)
}

// New builds a ring with the given virtual-node count and initial nodes.
func New(vnodesPerNode int, nodes ...string) *Ring {
	if vnodesPerNode < 1 {
		vnodesPerNode = 100
	}
	r := &Ring{vnodesPerNode: vnodesPerNode}
	for _, n := range nodes {
		r.Add(n)
	}
	return r
}

// HashU64 maps a string to a ring position (first 8 MD5 bytes, big-endian).
func HashU64(s string) uint64 {
	sum := md5.Sum([]byte(s))
	return binary.BigEndian.Uint64(sum[:8])
}

// Add inserts a node and its virtual nodes, then re-sorts the ring.
func (r *Ring) Add(node string) {
	for _, n := range r.nodes {
		if n == node {
			return
		}
	}
	r.nodes = append(r.nodes, node)
	for i := 0; i < r.vnodesPerNode; i++ {
		r.ring = append(r.ring, vnode{pos: HashU64(node + "#" + strconv.Itoa(i)), node: node, idx: i})
	}
	r.sortRing()
}

// Remove deletes a node and its virtual nodes (sorted order is preserved).
func (r *Ring) Remove(node string) {
	keptNodes := r.nodes[:0]
	for _, n := range r.nodes {
		if n != node {
			keptNodes = append(keptNodes, n)
		}
	}
	r.nodes = keptNodes
	kept := r.ring[:0]
	for _, v := range r.ring {
		if v.node != node {
			kept = append(kept, v)
		}
	}
	r.ring = kept
}

func (r *Ring) sortRing() {
	sort.Slice(r.ring, func(i, j int) bool {
		a, b := r.ring[i], r.ring[j]
		if a.pos != b.pos {
			return a.pos < b.pos
		}
		if a.node != b.node {
			return a.node < b.node // tie-break on equal positions, independent of insert order
		}
		return a.idx < b.idx
	})
}

// Get returns the node that owns key (empty string if the ring has no nodes).
func (r *Ring) Get(key string) string {
	v, ok := r.lookupHash(HashU64(key))
	if !ok {
		return ""
	}
	return v.node
}

// Lookup returns ownership detail for key, used by GET /cache/debug.
func (r *Ring) Lookup(key string) (node string, keyHash, vnodePos uint64, ok bool) {
	h := HashU64(key)
	v, found := r.lookupHash(h)
	if !found {
		return "", h, 0, false
	}
	return v.node, h, v.pos, true
}

func (r *Ring) lookupHash(h uint64) (vnode, bool) {
	if len(r.ring) == 0 {
		return vnode{}, false
	}
	// first vnode with pos >= h, wrapping past the end to index 0
	i := sort.Search(len(r.ring), func(i int) bool { return r.ring[i].pos >= h })
	if i == len(r.ring) {
		i = 0
	}
	return r.ring[i], true
}

// Nodes returns a copy of the physical node ids.
func (r *Ring) Nodes() []string {
	out := make([]string, len(r.nodes))
	copy(out, r.nodes)
	return out
}
