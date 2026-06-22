// Command rebalance shows consistent-hashing behavior: how keys distribute
// across nodes, and how few of them move when a node is added, compared to
// the naive hash % N approach.
//
//	go run ./cmd/rebalance
package main

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sort"

	"searchtypeahead/internal/hashring"
)

const keys = 100000

func main() {
	fmt.Printf("=== Consistent hashing demo: %d keys ===\n\n", keys)

	ring := hashring.New(150, "node-0", "node-1", "node-2")
	before := make([]string, keys)
	dist := map[string]int{}
	for i := 0; i < keys; i++ {
		n := ring.Get(key(i))
		before[i] = n
		dist[n]++
	}
	fmt.Println("Distribution over 3 nodes (MD5, 150 vnodes):")
	printDist(dist)

	ring.Add("node-3")
	moved := 0
	dist2 := map[string]int{}
	for i := 0; i < keys; i++ {
		n := ring.Get(key(i))
		dist2[n]++
		if n != before[i] {
			moved++
		}
	}
	fmt.Printf("\nAfter adding node-3 — keys moved: %d (%.1f%%)\n", moved, pct(moved))
	printDist(dist2)

	movedMod := 0
	for i := 0; i < keys; i++ {
		if hashMod(key(i), 3) != hashMod(key(i), 4) {
			movedMod++
		}
	}
	fmt.Printf("\nBaseline `hash %% N` (3→4 nodes) — keys moved: %d (%.1f%%)\n", movedMod, pct(movedMod))
	fmt.Println("\nConsistent hashing moves ~1/(N+1) of keys; `hash % N` moves ~75%.")
}

func key(i int) string { return fmt.Sprintf("prefix-%d", i) }

func hashMod(s string, n int) uint64 {
	sum := md5.Sum([]byte(s))
	return binary.BigEndian.Uint64(sum[:8]) % uint64(n)
}

func printDist(dist map[string]int) {
	ns := make([]string, 0, len(dist))
	for n := range dist {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	for _, n := range ns {
		fmt.Printf("  %-8s %7d  (%.1f%%)\n", n, dist[n], pct(dist[n]))
	}
}

func pct(a int) float64 { return 100 * float64(a) / float64(keys) }
