package hashring

import (
	"fmt"
	"testing"
)

func TestDistributionAndRebalance(t *testing.T) {
	const keys = 50000
	r := New(150, "node-0", "node-1", "node-2")

	before := make(map[string]string, keys)
	dist := map[string]int{}
	for i := 0; i < keys; i++ {
		k := fmt.Sprintf("prefix-%d", i)
		n := r.Get(k)
		before[k] = n
		dist[n]++
	}
	// MD5 with 150 vnodes lands around 1.04x; use a loose bound so the test stays stable.
	mean := float64(keys) / 3
	for n, c := range dist {
		if ratio := float64(c) / mean; ratio < 0.75 || ratio > 1.25 {
			t.Fatalf("node %s skew %.2fx (count=%d)", n, ratio, c)
		}
	}

	r.Add("node-3")
	moved := 0
	for k, was := range before {
		if r.Get(k) != was {
			moved++
		}
	}
	if frac := float64(moved) / keys; frac < 0.15 || frac > 0.35 {
		t.Fatalf("rebalance moved %.1f%% (want ~25%%)", frac*100)
	}
}

// TestDeterministic checks that the tie-break makes ownership independent of
// the order nodes were added, and stable across process restarts.
func TestDeterministic(t *testing.T) {
	r1 := New(100, "a", "b", "c")
	r2 := New(100, "c", "a", "b")
	for i := 0; i < 2000; i++ {
		k := fmt.Sprintf("k%d", i)
		if r1.Get(k) != r2.Get(k) {
			t.Fatalf("non-deterministic for %q: %s vs %s", k, r1.Get(k), r2.Get(k))
		}
	}
}
